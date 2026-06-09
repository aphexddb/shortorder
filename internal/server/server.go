// Package server exposes the thermal-printer service over a small HTTP/JSON API.
//
// The service is a long-running process; clients POST print jobs (text, QR
// codes, raster images, or raw ESC/POS) and the server renders them to ESC/POS
// and dispatches them to a detected, supported printer.
package server

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	qrcode "github.com/skip2/go-qrcode"

	"shortorder/internal/escpos"
	"shortorder/internal/printer"
)

//go:embed web/index.html
var indexHTML []byte

// Config configures the HTTP server and print defaults.
type Config struct {
	Addr        string // listen address, e.g. ":8080"
	PrinterName string // optional: force this spooler queue instead of auto-pick
	Width       int    // print head width in dots (80mm = 576)
	Version     string
}

// Server holds runtime state.
type Server struct {
	cfg Config
	log *slog.Logger
}

// New builds a Server.
func New(cfg Config, log *slog.Logger) *Server {
	if cfg.Width <= 0 {
		cfg.Width = 576
	}
	return &Server{cfg: cfg, log: log}
}

// Handler returns the configured HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /api/printers", s.handlePrinters)
	mux.HandleFunc("POST /api/print/text", s.handleText)
	mux.HandleFunc("POST /api/print/document", s.handleDocument)
	mux.HandleFunc("POST /api/print/svg", s.handleSVG)
	mux.HandleFunc("POST /api/print/sample/receipt", s.handleSampleReceipt)
	mux.HandleFunc("POST /api/print/sample/svg", s.handleSampleSVG)
	mux.HandleFunc("POST /api/print/qr", s.handleQR)
	mux.HandleFunc("POST /api/print/barcode", s.handleBarcode)
	mux.HandleFunc("POST /api/print/image", s.handleImage)
	mux.HandleFunc("POST /api/print/raw", s.handleRaw)
	mux.HandleFunc("POST /api/cut", s.handleCut)

	// Agent discovery: a machine-readable OpenAPI descriptor...
	mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)
	mux.HandleFunc("GET /.well-known/openapi.json", s.handleOpenAPI)

	// ...and an MCP server over the HTTP streamable transport. Stateless mode
	// keeps each request self-contained (no session bookkeeping), which suits a
	// simple tool server and any number of concurrent agents.
	mcpHTTP := mcpserver.NewStreamableHTTPServer(s.MCPServer(),
		mcpserver.WithStateLess(true),
		mcpserver.WithEndpointPath("/mcp"),
	)
	mux.Handle("/mcp", mcpHTTP)

	return s.withLogging(mux)
}

// ---- target selection ----------------------------------------------------

// target picks the printer to use: the configured override if set and present,
// otherwise the first detected supported printer.
func (s *Server) target() (printer.Info, error) {
	found, err := printer.Detect()
	if err != nil {
		return printer.Info{}, fmt.Errorf("detect printers: %w", err)
	}
	if len(found) == 0 {
		return printer.Info{}, fmt.Errorf("no supported printer detected (supported: %v)", printer.SupportedModels())
	}
	if s.cfg.PrinterName != "" {
		for _, p := range found {
			if p.Name == s.cfg.PrinterName {
				return p, nil
			}
		}
		return printer.Info{}, fmt.Errorf("configured printer %q not found among detected: %v", s.cfg.PrinterName, names(found))
	}
	return found[0], nil
}

// send dispatches an ESC/POS stream to the selected printer.
func (s *Server) send(jobName string, data []byte) (printer.Info, error) {
	t, err := s.target()
	if err != nil {
		return printer.Info{}, err
	}
	if err := printer.Print(t, data); err != nil {
		return t, fmt.Errorf("print to %q: %w", t.Name, err)
	}
	return t, nil
}

// ---- handlers ------------------------------------------------------------

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": s.cfg.Version,
	})
}

func (s *Server) handlePrinters(w http.ResponseWriter, r *http.Request) {
	found, err := printer.Detect()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"supported": printer.SupportedModels(),
		"detected":  found,
	})
}

// textSegment is one styled line of a text job. Every field is optional; the
// zero value prints plain, left-aligned text. Segments are how a single job
// mixes alignment, size, and emphasis line by line — e.g. a centered, enlarged,
// bold header above left-aligned body text — using the same ESC/POS primitives
// the flat fields use.
type textSegment struct {
	Text      string `json:"text"`
	Align     string `json:"align"`     // left|center|right
	Bold      bool   `json:"bold"`      //
	Underline byte   `json:"underline"` // 0,1,2
	Width     int    `json:"width"`     // char magnification 1..8
	Height    int    `json:"height"`    // char magnification 1..8
}

// textRequest is the body for POST /api/print/text. There are two ways to
// specify content, simplest first:
//
//   - text plus the flat style fields: one style applied to the whole block.
//   - lines: an ordered list of independently styled segments. When lines is
//     non-empty it takes precedence and the flat text/style fields are ignored.
//
// feed and cut always apply to the job as a whole, after the content.
type textRequest struct {
	Text      string        `json:"text"`
	Align     string        `json:"align"`     // left|center|right
	Bold      bool          `json:"bold"`      //
	Underline byte          `json:"underline"` // 0,1,2
	Width     int           `json:"width"`     // char magnification 1..8
	Height    int           `json:"height"`    // char magnification 1..8
	Lines     []textSegment `json:"lines"`     // optional per-line styling; overrides text when set
	Feed      int           `json:"feed"`      // extra line feeds after text
	Cut       *bool         `json:"cut"`       // cut after printing (default true)
}

func (s *Server) handleText(w http.ResponseWriter, r *http.Request) {
	var req textRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Text == "" && len(req.Lines) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("text or lines is required"))
		return
	}
	s.dispatch(w, "shortorder-text", buildText(req))
}

// buildText renders a text job to ESC/POS. Pure (no I/O) so both the HTTP
// handler and the MCP tool share exactly one code path.
//
// When req.Lines is set, each segment is rendered with its own style; otherwise
// the flat text/style fields render as a single segment. Either way feed and
// cut apply once, at the end.
func buildText(req textRequest) []byte {
	b := escpos.New()
	if len(req.Lines) > 0 {
		for _, seg := range req.Lines {
			writeSegment(b, seg)
		}
	} else {
		writeSegment(b, textSegment{
			Text:      req.Text,
			Align:     req.Align,
			Bold:      req.Bold,
			Underline: req.Underline,
			Width:     req.Width,
			Height:    req.Height,
		})
	}
	if req.Feed > 0 {
		b.Feed(req.Feed)
	}
	if cutOrDefault(req.Cut) {
		b.Cut()
	}
	return b.Bytes()
}

// writeSegment emits one styled line, then resets the styles it set so segments
// don't bleed into one another (or into the feed/cut that follow). Each segment
// sets its own alignment, so alignment needn't be reset between segments.
func writeSegment(b *escpos.Builder, seg textSegment) {
	b.Align(parseAlign(seg.Align))
	if seg.Width > 0 || seg.Height > 0 {
		b.Size(orOne(seg.Width), orOne(seg.Height))
	}
	if seg.Bold {
		b.Bold(true)
	}
	if seg.Underline > 0 {
		b.Underline(seg.Underline)
	}
	b.Line(seg.Text)
	b.Bold(false).Underline(0).Size(1, 1)
}

// qrRequest is the body for POST /api/print/qr.
type qrRequest struct {
	Data     string `json:"data"`
	Scale    int    `json:"scale"`    // module pixel size (default 8)
	Recovery string `json:"recovery"` // low|medium|high|highest
	Align    string `json:"align"`
	Caption  string `json:"caption"` // optional text under the code
	Cut      *bool  `json:"cut"`
}

func (s *Server) handleQR(w http.ResponseWriter, r *http.Request) {
	var req qrRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Data == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("data is required"))
		return
	}
	data, err := buildQR(req, s.cfg.Width)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.dispatch(w, "shortorder-qr", data)
}

// buildQR renders a QR job to ESC/POS at the given head width.
func buildQR(req qrRequest, width int) ([]byte, error) {
	b := escpos.New()
	if err := writeQR(b, req, width); err != nil {
		return nil, err
	}
	if cutOrDefault(req.Cut) {
		b.Cut()
	}
	return b.Bytes(), nil
}

// writeQR renders a QR code into an existing builder (no init, no cut) so both
// the standalone QR job and the document model emit identical bytes.
func writeQR(b *escpos.Builder, req qrRequest, width int) error {
	img, err := escpos.QRImage(req.Data, req.Scale, parseRecovery(req.Recovery))
	if err != nil {
		return fmt.Errorf("render qr: %w", err)
	}
	img = escpos.FitWidth(img, width)
	b.Align(parseAlignDefault(req.Align, escpos.AlignCenter))
	b.Image(img)
	if req.Caption != "" {
		b.Feed(1).Line(req.Caption)
	}
	b.Align(escpos.AlignLeft)
	return nil
}

// barcodeRequest is the body for POST /api/print/barcode.
type barcodeRequest struct {
	Data    string `json:"data"`
	Format  string `json:"format"` // code128|code39|code93|ean13|ean8|upca|itf|codabar
	Width   int    `json:"width"`  // total barcode width in dots (default: ~2 dots/module)
	Height  int    `json:"height"` // bar height in dots (default 80)
	Wide    bool   `json:"wide"`   // wider bars (~4 dots/module) for dense codes / finicky scanners
	HRI     bool   `json:"hri"`    // print the human-readable number under the code (EAN/UPC grouped)
	Align   string `json:"align"`
	Caption string `json:"caption"` // optional text under the code (overrides hri when set)
	Cut     *bool  `json:"cut"`
}

func (s *Server) handleBarcode(w http.ResponseWriter, r *http.Request) {
	var req barcodeRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Data == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("data is required"))
		return
	}
	data, err := buildBarcode(req, s.cfg.Width)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.dispatch(w, "shortorder-barcode", data)
}

// buildBarcode renders a barcode job to ESC/POS at the given head width. Pure
// (no I/O) so the HTTP handler and the MCP tool share one code path.
func buildBarcode(req barcodeRequest, width int) ([]byte, error) {
	b := escpos.New()
	if err := writeBarcode(b, req, width); err != nil {
		return nil, err
	}
	if cutOrDefault(req.Cut) {
		b.Cut()
	}
	return b.Bytes(), nil
}

// writeBarcode renders a barcode into an existing builder (no init, no cut),
// shared by the standalone barcode job and the document model.
func writeBarcode(b *escpos.Builder, req barcodeRequest, width int) error {
	img, err := escpos.BarcodeImage(req.Data, req.Format, req.Width, req.Height, req.Wide)
	if err != nil {
		return fmt.Errorf("render barcode: %w", err)
	}
	img = escpos.FitWidth(img, width)

	// An explicit caption wins; otherwise hri prints the grouped digits.
	caption := req.Caption
	if caption == "" && req.HRI {
		caption = escpos.BarcodeHRI(req.Format, req.Data)
	}

	b.Align(parseAlignDefault(req.Align, escpos.AlignCenter))
	b.Image(img)
	if caption != "" {
		b.Feed(1).Line(caption)
	}
	b.Align(escpos.AlignLeft)
	return nil
}

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	img, cut, align, err := decodeImageRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.dispatch(w, "shortorder-image", buildImageRaster(img, s.cfg.Width, align, cut))
}

// buildImageRaster renders an already-decoded image to ESC/POS, scaled to fit
// the head width.
func buildImageRaster(img image.Image, width int, align escpos.Align, cut bool) []byte {
	b := escpos.New()
	writeImage(b, img, width, align)
	if cut {
		b.Cut()
	}
	return b.Bytes()
}

// writeImage scales img to the head width and renders it into an existing
// builder (no init, no cut), shared by the image job and the document model.
func writeImage(b *escpos.Builder, img image.Image, width int, align escpos.Align) {
	img = escpos.FitWidth(img, width)
	b.Align(align)
	b.Image(img)
	b.Align(escpos.AlignLeft)
}

// rawRequest carries a base64-encoded ESC/POS stream.
type rawRequest struct {
	Bytes string `json:"bytes"` // base64
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	var data []byte
	if hasJSON(ct) {
		var req rawRequest
		if !decode(w, r, &req) {
			return
		}
		d, err := base64.StdEncoding.DecodeString(req.Bytes)
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("decode base64: %w", err))
			return
		}
		data = d
	} else {
		d, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("read body: %w", err))
			return
		}
		data = d
	}
	if len(data) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("empty payload"))
		return
	}
	s.dispatch(w, "shortorder-raw", data)
}

func (s *Server) handleCut(w http.ResponseWriter, r *http.Request) {
	b := escpos.New().Cut()
	s.dispatch(w, "shortorder-cut", b.Bytes())
}

// dispatch sends bytes to the printer and writes a uniform JSON response. The
// outcome (printed or the failure reason) is attached to the request's log line
// via the access logger's note.
func (s *Server) dispatch(w http.ResponseWriter, jobName string, data []byte) {
	t, err := s.send(jobName, data)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	setNote(w, fmt.Sprintf("printed %s on %q (%d bytes)", jobName, t.Name, len(data)))
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "printed",
		"job":     jobName,
		"bytes":   len(data),
		"printer": t,
	})
}

// ---- request logging -----------------------------------------------------

// statusRecorder wraps a ResponseWriter to capture the status code, response
// size, and an optional human note set by handlers (the print outcome or an
// error reason) so the access logger can emit one complete line per request.
type statusRecorder struct {
	http.ResponseWriter
	status int
	size   int
	note   string
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.size += n
	return n, err
}

// Flush forwards to the underlying ResponseWriter when it supports streaming
// (Server-Sent Events), as the MCP streamable-HTTP transport may use.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// setNote attaches a short outcome string to the current request's log line.
func setNote(w http.ResponseWriter, note string) {
	if sr, ok := w.(*statusRecorder); ok {
		sr.note = note
	}
}

// withLogging logs every request: method, path, status, response size, duration,
// client, and the handler's outcome note. Level follows status: 5xx -> error,
// 4xx -> warn, else info — so invalid commands and failures stand out.
func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.size,
			"dur", time.Since(start).Round(time.Microsecond).String(),
			"remote", clientIP(r),
		}
		if rec.note != "" {
			attrs = append(attrs, "info", rec.note)
		}
		switch {
		case rec.status >= 500:
			s.log.Error("request", attrs...)
		case rec.status >= 400:
			s.log.Warn("request", attrs...)
		default:
			s.log.Info("request", attrs...)
		}
	})
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ---- helpers -------------------------------------------------------------

func decodeImageRequest(r *http.Request) (img image.Image, cut bool, align escpos.Align, err error) {
	cut = r.URL.Query().Get("cut") != "false"
	align = parseAlignDefault(r.URL.Query().Get("align"), escpos.AlignCenter)

	ct := r.Header.Get("Content-Type")
	var raw []byte
	if hasJSON(ct) {
		var body struct {
			ImageBase64 string `json:"image_base64"`
			Cut         *bool  `json:"cut"`
			Align       string `json:"align"`
		}
		if e := json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(&body); e != nil {
			return nil, false, 0, fmt.Errorf("decode json: %w", e)
		}
		raw, err = base64.StdEncoding.DecodeString(body.ImageBase64)
		if err != nil {
			return nil, false, 0, fmt.Errorf("decode image_base64: %w", err)
		}
		if body.Cut != nil {
			cut = *body.Cut
		}
		if body.Align != "" {
			align = parseAlignDefault(body.Align, escpos.AlignCenter)
		}
	} else {
		raw, err = io.ReadAll(io.LimitReader(r.Body, 32<<20))
		if err != nil {
			return nil, false, 0, fmt.Errorf("read body: %w", err)
		}
	}
	if len(raw) == 0 {
		return nil, false, 0, fmt.Errorf("no image data")
	}
	im, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, false, 0, fmt.Errorf("decode image (png/jpeg/gif): %w", err)
	}
	return im, cut, align, nil
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid json body: %w", err))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	setNote(w, err.Error())
	writeJSON(w, code, map[string]any{"status": "error", "error": err.Error()})
}

func hasJSON(contentType string) bool {
	return bytes.Contains([]byte(contentType), []byte("application/json"))
}

func parseAlign(s string) escpos.Align {
	return parseAlignDefault(s, escpos.AlignLeft)
}

func parseAlignDefault(s string, def escpos.Align) escpos.Align {
	switch s {
	case "center", "centre":
		return escpos.AlignCenter
	case "right":
		return escpos.AlignRight
	case "left":
		return escpos.AlignLeft
	default:
		return def
	}
}

func parseRecovery(s string) qrcode.RecoveryLevel {
	switch s {
	case "low":
		return qrcode.Low
	case "high":
		return qrcode.High
	case "highest":
		return qrcode.Highest
	default:
		return qrcode.Medium
	}
}

func cutOrDefault(p *bool) bool {
	if p == nil {
		return true
	}
	return *p
}

func orOne(v int) int {
	if v <= 0 {
		return 1
	}
	return v
}

func names(infos []printer.Info) []string {
	out := make([]string, len(infos))
	for i, p := range infos {
		out[i] = p.Name
	}
	return out
}
