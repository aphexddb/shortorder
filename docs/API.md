# shortorder HTTP API

Base URL defaults to `http://127.0.0.1:8080`. All request/response bodies are
JSON unless noted. On success print endpoints return `200`:

```json
{
  "status": "printed",
  "job": "shortorder-text",
  "bytes": 89,
  "printer": { "name": "EPSON TM-T20II", "model": "Volcora v-WRP2-A1W", "usb": "VID_04B8 PID_0E20" }
}
```

On failure they return `4xx` (bad request) or `503` (no printer / write failed):

```json
{ "status": "error", "error": "no supported printer detected (supported: [Volcora v-WRP2-A1W])" }
```

---

## `GET /healthz`

Liveness check. Returns `{"status":"ok","version":"..."}`.

## `GET /api/printers`

Lists supported models and the printers currently detected.

```json
{
  "supported": ["Volcora v-WRP2-A1W"],
  "detected": [
    { "name": "EPSON TM-T20II", "model": "Volcora v-WRP2-A1W", "usb": "VID_04B8 PID_0E20" }
  ]
}
```

## `POST /api/print/text`

Print formatted text.

| Field       | Type   | Default | Notes                                     |
|-------------|--------|---------|-------------------------------------------|
| `text`      | string | —       | **Required.** `\n` for line breaks.       |
| `align`     | string | `left`  | `left` \| `center` \| `right`             |
| `bold`      | bool   | `false` | Emphasized printing                       |
| `underline` | int    | `0`     | `0` off, `1` thin, `2` thick              |
| `width`     | int    | `1`     | Character width magnification, `1`–`8`    |
| `height`    | int    | `1`     | Character height magnification, `1`–`8`   |
| `feed`      | int    | `0`     | Extra blank lines after the text          |
| `cut`       | bool   | `true`  | Cut after printing                        |

```sh
curl -X POST http://127.0.0.1:8080/api/print/text \
  -H 'Content-Type: application/json' \
  -d '{"text":"ORDER #42\nLatte x2","align":"center","width":2,"height":2,"cut":true}'
```

## `POST /api/print/qr`

Render `data` as a QR code and print it as a raster bitmap.

| Field      | Type   | Default  | Notes                                            |
|------------|--------|----------|--------------------------------------------------|
| `data`     | string | —        | **Required.** Text/URL to encode.                |
| `scale`    | int    | `8`      | Module (pixel) size; ~6–10 prints cleanly        |
| `recovery` | string | `medium` | `low` \| `medium` \| `high` \| `highest`         |
| `align`    | string | `center` | `left` \| `center` \| `right`                    |
| `caption`  | string | —        | Optional text printed under the code             |
| `cut`      | bool   | `true`   | Cut after printing                               |

```sh
curl -X POST http://127.0.0.1:8080/api/print/qr \
  -H 'Content-Type: application/json' \
  -d '{"data":"https://example.com","scale":8,"caption":"scan me"}'
```

## `POST /api/print/barcode`

Render `data` as a barcode and print it as a raster bitmap. Supports 1D and 2D
symbologies; rendering to a raster (rather than the printer's native barcode
commands) keeps the output consistent across ESC/POS clones.

| Field     | Type   | Default   | Notes                                                                 |
|-----------|--------|-----------|-----------------------------------------------------------------------|
| `data`    | string | —         | **Required.** Content to encode. Numeric symbologies accept digits only. |
| `format`  | string | `code128` | See the table below.                                                  |
| `width`   | int    | auto      | Total width in dots. 1D: ~2 dots/module (`4` with `wide`); 2D: scales the whole symbol. Capped to the head width. |
| `height`  | int    | `80`      | Bar height in dots (1D only; ignored for 2D codes).                   |
| `wide`    | bool   | `false`   | Larger modules (1D ~4 dots/module, 2D ~10) for dense codes or finicky scanners. Ignored when `width` is set. |
| `hri`     | bool   | `false`   | Print the human-readable number under the code, grouped per symbology (EAN-8 `4+4`, EAN-13 `1+6+6`, UPC-A `1+5+5+1`). Ignored if `caption` is set. |
| `align`   | string | `center`  | `left` \| `center` \| `right`                                         |
| `caption` | string | —         | Optional text printed under the code; overrides `hri`.                |
| `cut`     | bool   | `true`    | Cut after printing                                                    |

### Supported formats

| `format`                                    | Symbology                       | Notes                                                       |
|---------------------------------------------|---------------------------------|-------------------------------------------------------------|
| `code128`                                   | Code 128                        | Default. Full ASCII; the most general-purpose 1D code.      |
| `gs1-128`                                   | GS1-128 (UCC/EAN-128)           | Code 128 with a leading FNC1 flagging GS1 element strings.  |
| `code39`                                    | Code 39                         | Uppercase `A–Z`, digits, and `- . $ / + % space`.           |
| `code93`                                    | Code 93                         | Same character set as Code 39, denser bars.                 |
| `ean13`                                     | EAN-13                          | 12 digits (check computed) or 13 digits.                    |
| `ean8`                                      | EAN-8                           | 7 digits (check computed) or 8 digits.                      |
| `upca`                                      | UPC-A                           | 11 digits (check computed) or 12 digits.                    |
| `itf`                                       | Interleaved 2 of 5              | Digits only, even count; no check digit added.              |
| `itf14`                                     | ITF-14 (GTIN-14)               | Exactly 14 digits. Bearer bar not drawn.                    |
| `standard2of5`                              | Standard (non-interleaved) 2 of 5 | Digits only.                                              |
| `codabar`                                   | Codabar                         | Start/stop chars `A`–`D`; digits and `- $ : / . +`.         |
| `datamatrix`                                | Data Matrix (2D)                | Any text; scales as a square.                               |
| `pdf417`                                    | PDF417 (2D)                     | Any text; stacked, wide aspect.                             |

```sh
# Code 128 with the value printed beneath it
curl -X POST http://127.0.0.1:8080/api/print/barcode \
  -H 'Content-Type: application/json' \
  -d '{"data":"WO-2026-0608-42","format":"code128","caption":"WO-2026-0608-42"}'

# EAN-13 with the grouped human-readable number underneath
curl -X POST http://127.0.0.1:8080/api/print/barcode \
  -H 'Content-Type: application/json' \
  -d '{"data":"5901234123457","format":"ean13","hri":true}'

# A Data Matrix 2D code, enlarged
curl -X POST http://127.0.0.1:8080/api/print/barcode \
  -H 'Content-Type: application/json' \
  -d '{"data":"ASSET-MCH-00471","format":"datamatrix","wide":true}'
```

## `POST /api/print/image`

Print an arbitrary image as a 1-bit raster (Floyd–Steinberg dithered). Accepts
**PNG, JPEG, or GIF**. Images wider than the head are scaled down to fit; the
default fit width is 576 dots (`-width`).

Two ways to send the image:

- **Raw body** (`Content-Type: application/octet-stream` or an image type) — the
  body is the image file. `cut` and `align` come from the query string:
  `?cut=true&align=center`.
- **JSON body** (`Content-Type: application/json`):

  | Field          | Type   | Default  | Notes                          |
  |----------------|--------|----------|--------------------------------|
  | `image_base64` | string | —        | **Required.** Base64 image.    |
  | `align`        | string | `center` | `left` \| `center` \| `right`  |
  | `cut`          | bool   | `true`   | Cut after printing             |

```sh
curl -X POST 'http://127.0.0.1:8080/api/print/image?align=center&cut=true' \
  --data-binary @logo.png -H 'Content-Type: application/octet-stream'
```

## `POST /api/print/raw`

Send a raw ESC/POS byte stream untouched — for advanced commands the higher-level
endpoints don't cover.

- **Raw body** (`application/octet-stream`): the body bytes are sent verbatim.
- **JSON body**: `{ "bytes": "<base64 ESC/POS>" }`.

```sh
# Initialize + "Hi" + feed 3 + partial cut, as raw bytes
printf '\x1b@Hi\n\x1bd\x03\x1dV\x42\x00' | \
  curl -X POST http://127.0.0.1:8080/api/print/raw \
  --data-binary @- -H 'Content-Type: application/octet-stream'
```

## `POST /api/cut`

Feed a few lines clear of the head and perform a partial cut. No body.

```sh
curl -X POST http://127.0.0.1:8080/api/cut
```

---

## Agent discovery

### `GET /openapi.json` · `GET /.well-known/openapi.json`

Returns an OpenAPI 3.1 description of this API for function-calling agents and
tool loaders. The `servers[0].url` is filled in from the request host.

### `POST /mcp`

A [Model Context Protocol](https://modelcontextprotocol.io) server over the HTTP
streamable transport (stateless). Exposes the tools `list_printers`,
`print_text`, `print_qr`, `print_barcode`, `print_image`, and `cut`. Example (JSON-RPC):

```sh
curl -X POST http://127.0.0.1:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
```

The same MCP server runs over **stdio** via `shortorder mcp`, for agents that
launch tools as a subprocess:

```jsonc
{ "mcpServers": { "shortorder": { "command": "shortorder", "args": ["mcp"] } } }
```

### mDNS / DNS-SD

When running, the service advertises `_shortorder._tcp` on the local network with
TXT records `version`, `path=/`, `api=/api`, `mcp=/mcp`, `openapi=/openapi.json`.
Browse with `dns-sd -B _shortorder._tcp` or `avahi-browse _shortorder._tcp`.
