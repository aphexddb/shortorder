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

Print formatted text. Provide the content one of two ways: `text` with the flat
style fields (one style for the whole block), or `lines` for per-line styling.
At least one of `text` or `lines` is required.

| Field       | Type   | Default | Notes                                                                 |
|-------------|--------|---------|-----------------------------------------------------------------------|
| `text`      | string | —       | `\n` for line breaks. Required unless `lines` is given; ignored when `lines` is given. |
| `align`     | string | `left`  | `left` \| `center` \| `right`                                         |
| `bold`      | bool   | `false` | Emphasized printing                                                   |
| `underline` | int    | `0`     | `0` off, `1` thin, `2` thick                                          |
| `width`     | int    | `1`     | Character width magnification, `1`–`8`                                |
| `height`    | int    | `1`     | Character height magnification, `1`–`8`                               |
| `lines`     | array  | —       | Optional per-line styling; see below. Overrides `text` when present. |
| `feed`      | int    | `0`     | Extra blank lines after the text                                      |
| `cut`       | bool   | `true`  | Cut after printing                                                   |

```sh
curl -X POST http://127.0.0.1:8080/api/print/text \
  -H 'Content-Type: application/json' \
  -d '{"text":"ORDER #42\nLatte x2","align":"center","width":2,"height":2,"cut":true}'
```

### Per-line styling (`lines`)

For receipts that mix styles, send `lines`: an ordered array of styled segments,
each printed on its own line. When `lines` is present the top-level `text` and
style fields are ignored; `feed` and `cut` still apply to the whole job. Each
segment takes the same style fields as the flat form (all optional):

| Field       | Type   | Default | Notes                                   |
|-------------|--------|---------|-----------------------------------------|
| `text`      | string | —       | **Required.** `\n` for line breaks.     |
| `align`     | string | `left`  | `left` \| `center` \| `right`           |
| `bold`      | bool   | `false` | Emphasized printing                     |
| `underline` | int    | `0`     | `0` off, `1` thin, `2` thick            |
| `width`     | int    | `1`     | Character width magnification, `1`–`8`  |
| `height`    | int    | `1`     | Character height magnification, `1`–`8` |

```sh
curl -X POST http://127.0.0.1:8080/api/print/text \
  -H 'Content-Type: application/json' \
  -d '{
    "lines": [
      {"text":"ORDER #42","align":"center","bold":true,"width":2,"height":2},
      {"text":"Latte x2","align":"left"},
      {"text":"Thank you!","align":"center","underline":1}
    ],
    "feed": 1,
    "cut": true
  }'
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
`print_text`, `print_qr`, `print_image`, and `cut`, mirroring the REST API.
`print_text` takes `text` with the flat style fields for uniform styling, or an
optional `lines` array (each item a styled segment) to mix alignment, sizes, and
emphasis line by line. Example (JSON-RPC):

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
