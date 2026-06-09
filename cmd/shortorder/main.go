// Command shortorder runs a long-lived HTTP service that prints text, QR codes,
// and images to a supported Epson-compatible ESC/POS USB thermal receipt printer
// and cuts the receipt, all driven over a small JSON API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/grandcat/zeroconf"

	"shortorder/internal/printer"
	"shortorder/internal/server"
)

// Set via -ldflags at build time (see .goreleaser.yaml).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Subcommand: `shortorder mcp` runs the MCP server over stdio (for agents
	// that launch tools as a subprocess). The protocol owns stdout, so logs go
	// to stderr only.
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		runStdioMCP()
		return
	}

	var (
		addr        = flag.String("addr", envOr("SHORTORDER_ADDR", defaultAddr()), "HTTP listen address")
		port        = flag.String("port", "", "HTTP listen port (overrides the port in -addr; also reads $PORT)")
		printerName = flag.String("printer", os.Getenv("SHORTORDER_PRINTER"), "force a specific printer queue name (default: first detected supported printer)")
		width       = flag.Int("width", envInt("SHORTORDER_WIDTH", 576), "print head width in dots (80mm=576, 58mm=384)")
		debug       = flag.Bool("debug", false, "verbose request logging")
		showVersion = flag.Bool("version", false, "print version and exit")
		showHelp    = flag.Bool("help", false, "show usage (including -port) and exit")
		list        = flag.Bool("list", false, "list detected supported printers and exit")
	)
	flag.Parse()

	// Accept both the -help flag and a bare `help` subcommand (mirroring `mcp`),
	// so `shortorder help` prints usage rather than starting the server.
	if *showHelp || (len(os.Args) > 1 && os.Args[1] == "help") {
		flag.Usage()
		return
	}

	if *showVersion {
		fmt.Printf("shortorder %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	// Make this the default so lower-level packages (e.g. the SVG font seeder in
	// internal/escpos) log through the same handler.
	slog.SetDefault(log)

	// Resolve the listen port and note where it came from. Precedence:
	// the -port flag wins over $PORT, which wins over the port already implied
	// by -addr (itself from -addr / $SHORTORDER_ADDR / the platform default).
	// Whatever wins is folded back into *addr, the single value the server and
	// mDNS advertisement read downstream.
	host, addrPort, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Error("invalid -addr", "addr", *addr, "err", err)
		os.Exit(1)
	}
	portValue, portSource := addrPort, "-addr"
	if env := os.Getenv("PORT"); env != "" {
		portValue, portSource = env, "PORT env"
	}
	if flagSet("port") {
		portValue, portSource = *port, "-port flag"
	}
	*addr = net.JoinHostPort(host, portValue)
	log.Info("listen port configured", "port", portValue, "source", portSource)

	if *list {
		listPrinters(log)
		return
	}

	srv := server.New(server.Config{
		Addr:        *addr,
		PrinterName: *printerName,
		Width:       *width,
		Version:     version,
	}, log)

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Report what we found at startup so the operator sees it immediately.
	if found, err := printer.Detect(); err != nil {
		log.Warn("printer detection failed at startup", "err", err)
	} else if len(found) == 0 {
		log.Warn("no supported printer detected at startup", "supported", printer.SupportedModels())
	} else {
		for _, p := range found {
			log.Info("printer ready", "name", p.Name, "model", p.Model, "usb", p.USB)
		}
	}

	// Advertise over mDNS / DNS-SD so agents on the LAN can discover the service
	// without knowing its IP (best-effort — never fatal). Held in an outer-scoped
	// var so it can be torn down explicitly (and with a time bound) at shutdown,
	// rather than via defer — see the shutdown sequence below.
	var mdns *zeroconf.Server
	if m, err := advertiseMDNS(*addr, version); err != nil {
		log.Warn("mDNS advertisement unavailable", "err", err)
	} else if m != nil {
		mdns = m
		log.Info("advertising over mDNS", "service", mdnsService, "instance", mdnsInstance)
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("shortorder listening", "addr", *addr, "version", version)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")

	// Stop advertising first. grandcat/zeroconf v1.0.0's Shutdown waits on its
	// receive goroutines, which sit in a blocking ReadFrom and don't always
	// unblock when the socket closes (notably on Windows) — so it can hang
	// indefinitely. Bound it: send the goodbye if it's quick, give up otherwise.
	if mdns != nil {
		done := make(chan struct{})
		go func() { mdns.Shutdown(); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			log.Warn("mDNS shutdown timed out; continuing")
		}
	}

	// Drain HTTP gracefully: stop accepting, let in-flight requests finish. A
	// streaming connection that never goes idle (e.g. an MCP client holding the
	// /mcp SSE stream open) would otherwise block until the deadline, so on
	// timeout force every remaining connection closed rather than hang the quit.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Warn("graceful shutdown timed out; forcing connections closed", "err", err)
		if cerr := httpServer.Close(); cerr != nil {
			log.Error("forced close failed", "err", cerr)
		}
	}
}

func listPrinters(log *slog.Logger) {
	found, err := printer.Detect()
	if err != nil {
		log.Error("detection failed", "err", err)
		os.Exit(1)
	}
	if len(found) == 0 {
		fmt.Printf("No supported printer detected.\nSupported models: %v\n", printer.SupportedModels())
		return
	}
	fmt.Println("Detected supported printers:")
	for _, p := range found {
		fmt.Printf("  - %-28s model=%-22s usb=%s\n", p.Name, p.Model, p.USB)
	}
}

const (
	mdnsService  = "_shortorder._tcp"
	mdnsInstance = "shortorder"
)

// runStdioMCP serves the MCP tool server over stdio and blocks until stdin
// closes. Used by `shortorder mcp`.
func runStdioMCP() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)
	srv := server.New(server.Config{
		PrinterName: os.Getenv("SHORTORDER_PRINTER"),
		Width:       envInt("SHORTORDER_WIDTH", 576),
		Version:     version,
	}, log)
	if err := srv.ServeStdioMCP(); err != nil {
		log.Error("mcp stdio server error", "err", err)
		os.Exit(1)
	}
}

// advertiseMDNS registers the service over multicast DNS as _shortorder._tcp,
// with TXT metadata pointing agents at the web UI, JSON API, MCP endpoint, and
// OpenAPI descriptor.
func advertiseMDNS(addr, version string) (*zeroconf.Server, error) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("parse listen address %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("parse port %q: %w", portStr, err)
	}
	txt := []string{
		"version=" + version,
		"path=/",
		"api=/api",
		"mcp=/mcp",
		"openapi=/openapi.json",
	}
	return zeroconf.Register(mdnsInstance, mdnsService, "local.", port, txt, nil)
}

// defaultAddr is :80 everywhere except Windows, where binding 80 commonly
// collides with IIS / http.sys and needs elevation — so dev there defaults to
// :8080. On Linux (e.g. a Raspberry Pi appliance) the bundled service runs as
// root and serves on :80.
func defaultAddr() string {
	if runtime.GOOS == "windows" {
		return ":8080"
	}
	return ":80"
}

// flagSet reports whether the named flag was explicitly passed on the command
// line (as opposed to carrying its default), so an empty -port "" can be told
// apart from -port being absent.
func flagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}
