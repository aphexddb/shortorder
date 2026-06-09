# 🧾 shortorder

**A tiny service that prints to a USB thermal receipt printer.** 

shortorder is an AI-enabled thermal receipt printer. It runs as a small HTTP service that lets AI agents (and ordinary scripts) print to a USB thermal printer. Send it a request and it prints text, full receipt layouts, QR codes, barcodes, images, or arbitrary SVG, then cuts the receipt.

It ships an MCP server, so an LLM agent can discover the printer and use it as a tool without writing an integration. There's also a plain REST API for everything else. It's a single Go binary with no dependencies, no printer drivers, and no cloud, and it runs on Windows, Linux, and the Raspberry Pi.

```sh
curl -X POST http://localhost/api/print/text \
  -H 'Content-Type: application/json' \
  -d '{"text":"Hello, world!","cut":true}'
```

<p>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white">
  <img alt="License: Apache 2.0" src="https://img.shields.io/badge/License-Apache_2.0-green">
  <img alt="Platforms" src="https://img.shields.io/badge/platforms-Windows%20%7C%20Linux%20%7C%20Raspberry%20Pi-blue">
  <img alt="MCP" src="https://img.shields.io/badge/MCP-ready-7c3aed">
  <img alt="Protocol" src="https://img.shields.io/badge/protocol-ESC%2FPOS-orange">
</p>


## What it is

A REST and MCP interface to an Epson-compatible ESC/POS USB receipt printer — the
common identity shared by most inexpensive 80mm/58mm receipt printers (Epson
TM-series, Volcora, and compatible clones). You can drive it from an LLM agent, a shell script, a smart-home automation, or any language that can make an HTTP request.

A few things people use it for:

- Printing order tickets for a kitchen, café, or market stall.
- Letting an AI assistant print reminders, notes, or labels on request.
- Printing QR codes for inventory, packages, or Wi-Fi guest access.
- Scheduled output, like a morning weather slip, a daily summary, or jokes and memes.

## Use it from an AI agent

This is the part that makes it AI-enabled. There are three ways an agent can find and use the printer.

**MCP server.** shortorder exposes the printer as [Model Context Protocol](https://modelcontextprotocol.io) tools: `list_printers`, `print_text`, `print_document`, `print_qr`, `print_barcode`, `print_svg`, `print_image`, and `cut`. An MCP-aware agent gets the tool schemas automatically. Two transports are supported.

`print_document` is the one to reach for to lay out a whole receipt — header, an itemized table with prices flush-right, rules, totals, a barcode/QR, footer — in a single job using the printer's crisp native text. `print_svg` is the escape hatch for any layout the character grid can't express (logos, free positioning, shapes), rendered to a raster with fonts bundled in the binary.

stdio, for agents that launch a tool as a subprocess:

```jsonc
// e.g. claude_desktop_config.json, or any MCP client
{ "mcpServers": { "shortorder": { "command": "shortorder", "args": ["mcp"] } } }
```

HTTP (streamable transport) at `POST /mcp`, live whenever the service is running.

**OpenAPI.** A 3.1 descriptor is served at `/openapi.json` (and `/.well-known/openapi.json`) for function-calling agents and tool loaders that import an OpenAPI spec.

**mDNS.** When running, the service advertises `_shortorder._tcp` on the local network with TXT records (version, path, api, mcp, openapi), so an agent can find the box without being told its IP. Browse it with `dns-sd -B _shortorder._tcp` or `avahi-browse _shortorder._tcp`.

## Supported hardware

| Printer | Interface | Status |
|---------|-----------|--------|
| Epson TM-series & Epson-compatible ESC/POS | USB (ESC/POS) | Supported |
| Volcora v-WRP2-A1W and similar clones | USB (ESC/POS) | Tested (same Epson USB identity) |

Most inexpensive 80mm and 58mm USB receipt printers are ESC/POS and Epson-compatible. To add one, put a **Model** (USB vendor/product ID and/or a name substring) in the allowlist in
[`internal/printer/printer.go`](internal/printer/printer.go). The print path is already shared across models.

## Install

Prebuilt binaries and packages for each release are on the
[Releases page](https://github.com/aphexddb/shortorder/releases/latest). Download
the file for your platform, or use the commands below. The commands use the
[GitHub CLI](https://cli.github.com) (`gh`), which resolves the latest version
and its filenames for you; if you do not have `gh`, just download the file from
the releases page instead.

### Linux and Raspberry Pi (package)

On a Raspberry Pi or any systemd Linux box this is the easiest option. The
package installs a service that starts on boot and restarts on failure. Use the
arm64 build for a Raspberry Pi and amd64 for a typical x86 machine.

```sh
# Debian, Ubuntu, Raspberry Pi OS
gh release download -R aphexddb/shortorder -p '*_linux_arm64.deb'
sudo dpkg -i shortorder_*_linux_arm64.deb
```

`.rpm` (Fedora, RHEL) and `.apk` (Alpine) packages are published too. The service
serves the web UI and API on port 80. See
[Deploy on a Raspberry Pi](#deploy-on-a-raspberry-pi-debian-package) for managing
it.

### Linux or macOS (tarball)

```sh
gh release download -R aphexddb/shortorder -p '*_linux_amd64.tar.gz'
tar -xzf shortorder_*_linux_amd64.tar.gz
./shortorder
```

On macOS use the `darwin_arm64` (Apple Silicon) or `darwin_amd64` (Intel)
tarball.

### Windows

Download `shortorder_<version>_windows_amd64.zip` from the
[Releases page](https://github.com/aphexddb/shortorder/releases/latest), unzip
it, and run `shortorder.exe`. It listens on <http://localhost:8080/>.

### Verify a download (optional)

Every release ships a `checksums.txt`:

```sh
gh release download -R aphexddb/shortorder -p 'checksums.txt'
sha256sum -c checksums.txt --ignore-missing
```

## Using it

Once it's running, open the web UI in a browser. It lists connected devices, runs
test prints, and documents the API.

- Linux and macOS: <http://localhost/>
- Windows: <http://localhost:8080/>

Or print from anything that speaks HTTP:

```sh
# Plain text, centered and bold, then cut
curl -X POST http://localhost/api/print/text \
  -H 'Content-Type: application/json' \
  -d '{"text":"ORDER #42","align":"center","bold":true,"cut":true}'

# A whole receipt in one job: header, itemized table, total, then cut
curl -X POST http://localhost/api/print/document \
  -H 'Content-Type: application/json' \
  -d '{
    "elements": [
      {"type":"text","text":"SHORT ORDER","align":"center","bold":true,"width":2,"height":2},
      {"type":"rule"},
      {"type":"table",
       "columns":[{"width":3},{"width":0},{"width":8,"align":"right"}],
       "rows":[["2","Coffee","$6.00"],["1","Muffin","$3.25"]]},
      {"type":"rule","char":"="},
      {"type":"row","left":"TOTAL","right":"$9.25","bold":true}
    ],
    "cut": true
  }'

# Arbitrary layout via SVG (logos, shapes, free positioning)
curl -X POST http://localhost/api/print/svg \
  -H 'Content-Type: application/json' \
  -d '{"svg":"<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"384\" height=\"80\" viewBox=\"0 0 384 80\"><rect width=\"384\" height=\"80\" fill=\"white\"/><text x=\"192\" y=\"54\" font-family=\"sans-serif\" font-size=\"40\" text-anchor=\"middle\">SHORT ORDER</text></svg>"}'

# A QR code with a caption
curl -X POST http://localhost/api/print/qr \
  -H 'Content-Type: application/json' \
  -d '{"data":"https://example.com","caption":"scan me"}'

# A CODE128 barcode with the value printed beneath it
# (add "wide":true for ~4 dots/module — easier on dense codes and finicky scanners)
curl -X POST http://localhost/api/print/barcode \
  -H 'Content-Type: application/json' \
  -d '{"data":"SHORTORDER42","format":"code128","caption":"SHORTORDER42"}'

# Any PNG, JPEG, or GIF, scaled to fit and dithered
curl -X POST 'http://localhost/api/print/image?align=center' \
  --data-binary @logo.png -H 'Content-Type: application/octet-stream'

# Built-in samples (no body): a full receipt and an SVG showcase
curl -X POST http://localhost/api/print/sample/receipt
curl -X POST http://localhost/api/print/sample/svg
```

## Build from source

Requires Go 1.26 or newer.

```sh
# Build a single static binary into ./bin
make build        # or: CGO_ENABLED=0 go build -o bin/shortorder ./cmd/shortorder

# See what's detected
./bin/shortorder -list

# Run the service
./bin/shortorder
```

## Configuration

| Flag        | Env                  | Default                       | Description                               |
|-------------|----------------------|-------------------------------|-------------------------------------------|
| `-addr`     | `SHORTORDER_ADDR`    | `:80` (`:8080` on Windows)    | HTTP listen address                       |
| `-printer`  | `SHORTORDER_PRINTER` | auto                          | Force a specific detected printer by name |
| `-width`    | `SHORTORDER_WIDTH`   | `576`                         | Print head width in dots (80mm=576, 58mm=384) |
| `-debug`    |                      | `false`                       | Verbose request logging                   |
| `-list`     |                      |                               | List detected printers and exit           |
| `-version`  |                      |                               | Print version and exit                    |

Port 80 is the default on Linux and macOS (the Raspberry Pi service runs as root
and binds it directly). On Windows it defaults to 8080 to avoid the common
`http.sys`/IIS collision and the need for elevation.

## HTTP API

| Method and path         | Purpose                                       |
|-------------------------|-----------------------------------------------|
| `GET  /`                | Web UI: devices, test prints, API docs        |
| `GET  /healthz`         | Liveness and version                          |
| `GET  /api/printers`    | List supported models and detected devices    |
| `POST /api/print/text`  | Print formatted text                          |
| `POST /api/print/document` | Lay out a whole receipt (text, rows, tables, rules, codes) in one job |
| `POST /api/print/qr`    | Render and print a QR code                     |
| `POST /api/print/barcode` | Render and print a 1D/2D barcode (incl. DataMatrix, PDF417) |
| `POST /api/print/svg`   | Render arbitrary SVG markup to a raster and print it |
| `POST /api/print/image` | Print a PNG/JPEG/GIF as a dithered raster      |
| `POST /api/print/sample/receipt` | Print the built-in sample receipt (no body) |
| `POST /api/print/sample/svg` | Print the built-in SVG showcase (no body) |
| `POST /api/print/raw`   | Send a raw ESC/POS byte stream                 |
| `POST /api/cut`         | Feed and cut                                   |
| `GET  /openapi.json`    | OpenAPI 3.1 descriptor (also `/.well-known/openapi.json`) |
| `POST /mcp`             | MCP server (HTTP streamable transport)         |

Full request and response reference: [docs/API.md](docs/API.md).

For crisp receipt text, use `/api/print/text` (the printer's native font). Image
mode rasterizes at the head's dot density, so keep any text inside an image
generously sized or it can print faint.

## Deploy on a Raspberry Pi (Debian package)

shortorder publishes `.deb`, `.rpm`, and `.apk` packages. On a Raspberry Pi
(arm64), install the `.deb` to get a service that starts on boot:

```sh
sudo dpkg -i shortorder_*_linux_arm64.deb
```

The package:

- installs the binary to `/usr/bin/shortorder`,
- installs and enables a `systemd` service (`shortorder.service`) that starts on
  boot and restarts on failure,
- serves the UI and API on port 80,
- sets the Pi's hostname to `shortorder`, so it's reachable at
  <http://shortorder.local/> over mDNS.

```sh
sudo systemctl status shortorder      # check it's running
journalctl -u shortorder -f           # follow the logs
echo 'SHORTORDER_ADDR=:8080' | sudo tee /etc/default/shortorder   # override settings
```

Plug a supported USB printer into the Pi and it's detected automatically.

## How it works

Receipt printers speak ESC/POS. A print job is a byte stream of printable text
plus control sequences for formatting, raster bitmaps, and the cutter.
shortorder renders each API request to ESC/POS and writes it straight to the
printer's USB device, with no spooler, print queue, or driver install:

- On Windows, it finds the printer via the USB printer device interface
  (`GUID_DEVINTERFACE_USBPRINT`) and writes with `CreateFile`/`WriteFile`.
- On Linux and the Raspberry Pi, it finds the printer in `sysfs`, matches its USB
  VID/PID against the allowlist, and writes to its `usblp` node (`/dev/usb/lp0`).

Text and full receipt layouts (`/api/print/document`) are composed from the
printer's native font over a fixed character grid — rows, tables, and rules laid
out in code — so they print crisp and small. QR codes, barcodes, images, and SVG
(`/api/print/svg`) are rendered to a 1-bit raster (Floyd-Steinberg dithered for
photos and grayscale) and sent with the `GS v 0` raster command, so output is
the same across printers regardless of their native graphics support. SVG is
rendered by a pure-Go engine with fonts bundled into the binary (Roboto, Gelasio,
Go Mono), so text renders identically on every host with no system fonts
installed.

## Logging

Every request is logged as one structured line with method, path, status, size, duration, client, and outcome. The level follows the result (`INFO` for 2xx and 3xx, `WARN` for 4xx, `ERROR` for 5xx), so invalid commands and print failures stand out.

## Cutting a release (maintainers)

[GoReleaser](https://goreleaser.com) builds the full matrix (linux, windows, and darwin, for amd64 and arm64) plus the Linux packages:

```sh
make check          # validate .goreleaser.yaml
make snapshot       # full matrix + .deb/.rpm/.apk into ./dist (no publish)
make tag V=v0.1.0   # tag + push, CI cuts a GitHub release
```

## FAQ

**Does it need printer drivers?** No. shortorder talks to the printer's raw USB endpoint directly. There's no driver, print queue, or spooler to configure.

**What printers are supported?** Epson-compatible ESC/POS USB receipt printers — Epson TM-series and the many clones that share Epson's USB identity (tested with a Volcora v-WRP2-A1W). Most budget 80mm/58mm receipt printers qualify. Adding a model is a one-line allowlist change.

**Which platforms?** Printing works on Windows and Linux, including the Raspberry Pi. The binary also builds and runs on macOS, where the print path is a stub pending a CUPS backend.

**Can an AI agent or LLM use this?** Yes. It ships an MCP server (stdio and HTTP) so MCP-aware agents discover the print tools, an OpenAPI spec for function-calling agents, and mDNS so agents find it on the LAN. You can also just POST the REST API. See [Use it from an AI agent](#use-it-from-an-ai-agent).

**Does it phone home or need the internet?** No. It's a fully local service.

**What's the wire protocol to the printer?** ESC/POS over USB: the `usbprint` device interface on Windows, the `usblp` character device on Linux.

**What barcodes are supported?** Code 128, GS1-128, Code 39, Code 93, EAN-13, EAN-8, UPC-A, Interleaved 2 of 5, ITF-14, Standard 2 of 5, Codabar, Data Matrix, PDF417

## License

[Apache 2.0](LICENSE). Contributions and new printer models welcome.

The SVG renderer bundles the Roboto and Gelasio fonts, both under the
[SIL Open Font License](https://openfontlicense.org) (license texts in
[`internal/escpos/fonts/`](internal/escpos/fonts/)).
