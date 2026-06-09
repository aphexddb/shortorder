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

## `POST /api/print/document`

Lay out and print a **whole receipt as one job** — an ordered list of layout
elements rendered top to bottom with a single cut at the end. Unlike the other
print endpoints (each of which is its own job that cuts), a document composes a
header, an itemized table with prices flush-right, rules, totals, a barcode/QR,
and a footer into one receipt, using the printer's crisp native monospace text.

Layout happens over a fixed character grid. The line width in characters comes
from `columns`, or is derived from the head width when omitted (80mm = 48,
58mm = 32). Column-aware elements (`row`, `columns`, `table`, `rule`) are
accurate at the default text size; use enlarged text only for centered headers.

| Field      | Type   | Default | Notes                                                       |
|------------|--------|---------|-------------------------------------------------------------|
| `columns`  | int    | derived | Line width in characters. `0`/omitted derives from the head width. |
| `elements` | array  | —       | **Required.** Ordered layout elements (see below).          |
| `feed`     | int    | `0`     | Extra blank lines after the document.                       |
| `cut`      | bool   | `true`  | Cut once, after the whole document.                         |

### Elements

Each element is an object with a `type` and the fields for that type:

| `type`    | Fields                                                  | Renders                                              |
|-----------|---------------------------------------------------------|------------------------------------------------------|
| `text`    | `text`, `align`, `bold`, `underline`, `width`, `height` | A word-wrapped paragraph. Wrapped to the line width at default size; `width`/`height` (1–8) enlarge it (and disable wrapping). |
| `row`     | `left`, `right`, `bold`, `underline`                    | A label left, a value flush right (`Cheeseburger      $8.50`). Left text wraps; the value stays on the last line. |
| `columns` | `cells`, `columns`, `gap`, `bold`, `underline`          | One row of N cells. Each cell wraps within its column. |
| `table`   | `rows`, `columns`, `gap`, `bold`, `underline`           | Many rows sharing one set of column definitions.     |
| `rule`    | `char`                                                  | A horizontal rule across the line (default `-`).     |
| `feed`    | `lines`                                                 | Blank vertical space (default 1 line).               |
| `qr`      | `data`, `scale`, `recovery`, `align`, `caption`         | A QR code (same rendering as `/api/print/qr`).       |
| `barcode` | `data`, `format`, `wide`, `hri`, `align`, `caption`     | A 1D/2D barcode (same rendering as `/api/print/barcode`). |
| `image`   | `image_base64`, `align`                                 | A base64 PNG/JPEG/GIF raster, scaled to fit the head. |

A column definition (`columns[]`) is `{ "width": <cells>, "align": "left|center|right" }`.
A `width` of `0` (or omitted) is **auto**: the remaining line width is split
evenly among the auto columns. `gap` is the number of blank cells between
columns (default `1`).

```sh
curl -X POST http://127.0.0.1:8080/api/print/document \
  -H 'Content-Type: application/json' \
  -d '{
    "elements": [
      {"type":"text","text":"SHORT ORDER CAFE","align":"center","bold":true,"width":2,"height":2},
      {"type":"text","text":"123 Main St","align":"center"},
      {"type":"rule"},
      {"type":"table",
       "columns":[{"width":3},{"width":0},{"width":8,"align":"right"}],
       "rows":[["2","Coffee","$6.00"],["1","Blueberry Muffin","$3.25"]]},
      {"type":"rule","char":"="},
      {"type":"row","left":"TOTAL","right":"$9.25","bold":true},
      {"type":"feed","lines":1},
      {"type":"barcode","format":"code128","data":"ORD-1042","hri":true},
      {"type":"text","text":"Thank you!","align":"center"}
    ],
    "cut": true
  }'
```

## `POST /api/print/svg`

Render arbitrary **SVG markup** to a raster and print it — the universal layout
escape hatch. Where `/api/print/text` and `/api/print/document` lay out receipts
in the printer's native character grid, this prints *anything you can draw*:
logos, free positioning, rotation, shapes, rules, gradients, embedded images.
Printed as a 1-bit dithered raster.

**Fonts.** For determinism the host's fonts are **not** used — text renders in a
bundled font (the BSD-licensed Go fonts) so the same markup prints identically on
every host, including a bare appliance with no fonts installed. Consequences:
every `font-family` (serif, sans-serif, cursive, a specific name, …) maps to that
one typeface, and `font-weight`/`font-style` are **not** differentiated (no real
bold or italic). Map `font-family` to a generic (`sans-serif`, `monospace`); for
crisp, genuinely bold or styled receipt text use `/api/print/text` or
`/api/print/document` instead. (An SVG that requests a single specific font with
no generic fallback returns a clean error rather than rendering.)

Prefer native text (`/api/print/text`, `/api/print/document`) when the grid is
enough: it is crisper, selectable, supports real bold, and is a fraction of the
bytes. Reach for SVG when the layout can't be expressed in fixed-width rows.

The root `<svg>` must declare a `width` and `height` (or a `viewBox`) so it has
an intrinsic size; it is scaled, preserving aspect ratio, to the target width.

| Field   | Type   | Default  | Notes                                                                 |
|---------|--------|----------|-----------------------------------------------------------------------|
| `svg`   | string | —        | **Required.** SVG markup.                                             |
| `width` | int    | head     | Target raster width in dots. Defaults to and is capped at the head width (`-width`, 576 for 80mm). A smaller value prints narrower and is positioned by `align`. |
| `align` | string | `center` | `left` \| `center` \| `right` (used when narrower than the head).      |
| `cut`   | bool   | `true`   | Cut after printing.                                                   |

```sh
curl -X POST http://127.0.0.1:8080/api/print/svg \
  -H 'Content-Type: application/json' \
  -d '{
    "svg": "<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"384\" height=\"120\" viewBox=\"0 0 384 120\"><rect width=\"384\" height=\"120\" fill=\"white\"/><text x=\"192\" y=\"70\" font-family=\"serif\" font-size=\"48\" font-weight=\"bold\" text-anchor=\"middle\">SHORT ORDER</text></svg>",
    "cut": true
  }'
```

A tall/narrow SVG that would rasterize into meters of paper is rejected (the
rendered height is capped); reduce its height-to-width ratio or `width`.

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
`print_text`, `print_document`, `print_svg`, `print_qr`, `print_barcode`,
`print_image`, and `cut`, mirroring the REST API. `print_text` takes `text` with the flat style fields for uniform
styling, or an optional `lines` array (each item a styled segment) to mix
alignment, sizes, and emphasis line by line. Example (JSON-RPC):

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
