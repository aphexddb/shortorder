package server

import (
	"net/http"
)

// handleOpenAPI serves a machine-readable OpenAPI 3.1 description of the API so
// function-calling agents and tool loaders can import it automatically. It is
// available at /openapi.json and /.well-known/openapi.json.
func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	base := scheme + "://" + r.Host

	writeJSON(w, http.StatusOK, s.openAPISpec(base))
}

func (s *Server) openAPISpec(serverURL string) map[string]any {
	align := map[string]any{
		"type": "string", "enum": []string{"left", "center", "right"},
	}
	cut := map[string]any{
		"type": "boolean", "default": true, "description": "Cut the paper after printing.",
	}
	printedResponse := map[string]any{
		"description": "Printed",
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": ref("PrintResult"),
			},
		},
	}
	errorResponses := map[string]any{
		"400": jsonError("Invalid request"),
		"503": jsonError("No printer connected or the write failed"),
	}

	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "shortorder",
			"version":     s.cfg.Version,
			"description": "HTTP API for printing text, QR codes, barcodes, and images to a USB thermal receipt printer and cutting the receipt. An MCP server with the same capabilities is available at /mcp.",
			"license":     map[string]any{"name": "Apache-2.0"},
		},
		"servers": []any{map[string]any{"url": serverURL}},
		"paths": map[string]any{
			"/healthz": map[string]any{
				"get": op("health", "Liveness and version", nil, map[string]any{
					"200": map[string]any{"description": "OK"},
				}),
			},
			"/api/printers": map[string]any{
				"get": op("listPrinters", "List supported models and detected devices", nil, map[string]any{
					"200": map[string]any{"description": "Printer inventory"},
				}),
			},
			"/api/print/text": map[string]any{
				"post": op("printText", "Print formatted text", ref("TextRequest"), merge(map[string]any{"200": printedResponse}, errorResponses)),
			},
			"/api/print/qr": map[string]any{
				"post": op("printQR", "Render and print a QR code", ref("QRRequest"), merge(map[string]any{"200": printedResponse}, errorResponses)),
			},
			"/api/print/barcode": map[string]any{
				"post": op("printBarcode", "Render and print a 1D or 2D barcode", ref("BarcodeRequest"), merge(map[string]any{"200": printedResponse}, errorResponses)),
			},
			"/api/print/image": map[string]any{
				"post": op("printImage", "Print a base64 PNG/JPEG/GIF as a dithered raster", ref("ImageRequest"), merge(map[string]any{"200": printedResponse}, errorResponses)),
			},
			"/api/print/raw": map[string]any{
				"post": op("printRaw", "Send a raw ESC/POS byte stream", ref("RawRequest"), merge(map[string]any{"200": printedResponse}, errorResponses)),
			},
			"/api/cut": map[string]any{
				"post": op("cut", "Feed and cut", nil, merge(map[string]any{"200": printedResponse}, errorResponses)),
			},
		},
		"components": map[string]any{
			"schemas": map[string]any{
				"TextRequest": object(map[string]any{
					"text":      map[string]any{"type": "string", "description": `Text to print; \n for line breaks. Required unless "lines" is given; ignored when "lines" is given.`},
					"align":     align,
					"bold":      map[string]any{"type": "boolean"},
					"underline": map[string]any{"type": "integer", "enum": []int{0, 1, 2}},
					"width":     map[string]any{"type": "integer", "minimum": 1, "maximum": 8},
					"height":    map[string]any{"type": "integer", "minimum": 1, "maximum": 8},
					"lines": map[string]any{
						"type":        "array",
						"items":       ref("TextSegment"),
						"description": `Optional per-line styling. When present, each item prints as its own styled line and the top-level text/align/bold/underline/width/height are ignored.`,
					},
					"feed": map[string]any{"type": "integer", "minimum": 0},
					"cut":  cut,
				}, nil),
				"TextSegment": object(map[string]any{
					"text":      map[string]any{"type": "string", "description": `Text for this line; \n for line breaks.`},
					"align":     align,
					"bold":      map[string]any{"type": "boolean"},
					"underline": map[string]any{"type": "integer", "enum": []int{0, 1, 2}},
					"width":     map[string]any{"type": "integer", "minimum": 1, "maximum": 8},
					"height":    map[string]any{"type": "integer", "minimum": 1, "maximum": 8},
				}, []string{"text"}),
				"QRRequest": object(map[string]any{
					"data":     map[string]any{"type": "string", "description": "Text or URL to encode."},
					"scale":    map[string]any{"type": "integer", "default": 8},
					"recovery": map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "highest"}},
					"align":    align,
					"caption":  map[string]any{"type": "string"},
					"cut":      cut,
				}, []string{"data"}),
				"BarcodeRequest": object(map[string]any{
					"data":    map[string]any{"type": "string", "description": "Content to encode; numeric symbologies accept digits only."},
					"format":  map[string]any{"type": "string", "enum": []string{"code128", "gs1-128", "code39", "code93", "ean13", "ean8", "upca", "itf", "itf14", "standard2of5", "codabar", "datamatrix", "pdf417"}, "default": "code128"},
					"width":   map[string]any{"type": "integer", "description": "Total width in dots (1D: ~2 dots/module; 2D: scales the whole symbol). Capped to head width."},
					"height":  map[string]any{"type": "integer", "default": 80, "description": "Bar height in dots for 1D codes; ignored for 2D codes."},
					"wide":    map[string]any{"type": "boolean", "default": false, "description": "Larger modules (1D ~4 dots/module, 2D ~10) for dense codes or finicky scanners; ignored when width is set."},
					"hri":     map[string]any{"type": "boolean", "default": false, "description": "Print the human-readable number under the code, grouped per symbology (EAN/UPC). Ignored if caption is set."},
					"align":   align,
					"caption": map[string]any{"type": "string", "description": "Optional text printed under the barcode; overrides hri."},
					"cut":     cut,
				}, []string{"data"}),
				"ImageRequest": object(map[string]any{
					"image_base64": map[string]any{"type": "string", "contentEncoding": "base64", "description": "Base64 PNG/JPEG/GIF."},
					"align":        align,
					"cut":          cut,
				}, []string{"image_base64"}),
				"RawRequest": object(map[string]any{
					"bytes": map[string]any{"type": "string", "contentEncoding": "base64", "description": "Base64 ESC/POS byte stream."},
				}, []string{"bytes"}),
				"PrintResult": object(map[string]any{
					"status":  map[string]any{"type": "string"},
					"job":     map[string]any{"type": "string"},
					"bytes":   map[string]any{"type": "integer"},
					"printer": ref("Printer"),
				}, nil),
				"Printer": object(map[string]any{
					"name":  map[string]any{"type": "string"},
					"model": map[string]any{"type": "string"},
					"usb":   map[string]any{"type": "string"},
					"path":  map[string]any{"type": "string"},
				}, nil),
			},
		},
	}
}

// ---- tiny OpenAPI builders ----------------------------------------------

func op(id, summary string, bodySchema map[string]any, responses map[string]any) map[string]any {
	o := map[string]any{
		"operationId": id,
		"summary":     summary,
		"responses":   responses,
	}
	if bodySchema != nil {
		o["requestBody"] = map[string]any{
			"required": true,
			"content": map[string]any{
				"application/json": map[string]any{"schema": bodySchema},
			},
		}
	}
	return o
}

func object(props map[string]any, required []string) map[string]any {
	o := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

func ref(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func jsonError(desc string) map[string]any {
	return map[string]any{
		"description": desc,
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": object(map[string]any{
					"status": map[string]any{"type": "string"},
					"error":  map[string]any{"type": "string"},
				}, nil),
			},
		},
	}
}

func merge(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
