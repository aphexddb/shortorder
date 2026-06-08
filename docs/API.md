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
