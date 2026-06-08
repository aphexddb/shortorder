# 🧾 shortorder

**A tiny HTTP service that prints to a USB thermal receipt printer.** Send it an
HTTP request and it prints — plain text, QR codes, or any image — then cuts the
receipt. One static Go binary, no dependencies, no print drivers, no cloud.

```sh
curl -X POST http://localhost/api/print/text \
  -H 'Content-Type: application/json' \
  -d '{"text":"Hello, world!","cut":true}'
```

<p>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white">
  <img alt="License: Apache 2.0" src="https://img.shields.io/badge/License-Apache_2.0-green">
  <img alt="Platforms" src="https://img.shields.io/badge/platforms-Windows%20%7C%20Linux%20%7C%20Raspberry%20Pi-blue">
  <img alt="Protocol" src="https://img.shields.io/badge/protocol-ESC%2FPOS-orange">
</p>

> **What is shortorder?** shortorder is an open-source REST API for thermal
> receipt printers, written in Go. It turns an ESC/POS USB printer (like the
> Volcora v-WRP2-A1W or an Epson TM-series) into a network service you can drive
> from any language, script, smart-home automation, or AI agent. It runs great on
> a Raspberry Pi as a `systemd` appliance, or on a Windows/Mac desktop.

---

## ✨ Features

- 🖨️ **Print text, QR codes, and images** over a dead-simple JSON API.
- ✂️ **Auto-cuts** the receipt (partial or full cut).
- 🔌 **Plug-and-play USB detection** — finds the printer, no driver install or
  print queue setup required.
- 🧱 **Single static binary** — pure Go, `CGO_ENABLED=0`, nothing to install
  alongside it.
- 🍓 **Raspberry Pi ready** — ships as a `.deb`/`.rpm`/`.apk` that installs a
  `systemd` service and turns the Pi into a `shortorder.local` print appliance.
- 🌐 **Built-in web UI** — see connected devices, run test prints, and read the
  API docs in your browser.
- 📋 **Structured request logging** — every call is logged with its outcome.
- 🎛️ **ESC/POS under the hood** — works with the huge family of Epson-compatible
  receipt printers; add new models with a one-line allowlist entry.

## 💡 What can you build with it?

- A **home automation printer** — print reminders, shopping lists, or the day's
  calendar from Home Assistant / Node-RED / a cron job.
- An **order ticket printer** for a café, pop-up, market stall, or home kitchen.
- A **label / QR printer** for inventory, packages, or Wi-Fi guest passwords.
- An **AI agent's hands in the real world** — let an LLM or chatbot physically
  print notes, tickets, or art via a single HTTP POST.
- A **fun gadget** — print memes, daily weather, tarot cards, or "word of the day"
  receipts on a schedule.

## 🖨️ Supported hardware

| Printer | Interface | Status |
|---------|-----------|--------|
| **Volcora v-WRP2-A1W** | USB (ESC/POS) | ✅ Supported |
| Epson TM-series & compatible clones | USB (ESC/POS) | ✅ Works (same USB identity) |

Most inexpensive 80mm/58mm USB receipt printers are **ESC/POS** and
Epson-compatible. To add one, drop a `Model` (USB vendor/product ID and/or a
name substring) into the allowlist in
[`internal/printer/printer.go`](internal/printer/printer.go) — the print path is
already universal.

## 🚀 Quick start

```sh
# Build a single static binary into ./bin
make build        # or: CGO_ENABLED=0 go build -o bin/shortorder ./cmd/shortorder

# See what's detected
./bin/shortorder -list

# Run the service
./bin/shortorder
```

Now open the **web UI** in a browser:

- Linux / macOS → <http://localhost/>
- Windows → <http://localhost:8080/>

…or print from anything that speaks HTTP:

```sh
# Plain text, centered & bold, then cut
curl -X POST http://localhost/api/print/text \
  -H 'Content-Type: application/json' \
  -d '{"text":"ORDER #42","align":"center","bold":true,"cut":true}'

# A scannable QR code with a caption
curl -X POST http://localhost/api/print/qr \
  -H 'Content-Type: application/json' \
  -d '{"data":"https://example.com","caption":"scan me"}'

# Any PNG / JPEG / GIF, scaled to fit and dithered
curl -X POST 'http://localhost/api/print/image?align=center' \
  --data-binary @logo.png -H 'Content-Type: application/octet-stream'
```

## ⚙️ Configuration

| Flag        | Env                  | Default                       | Description                               |
|-------------|----------------------|-------------------------------|-------------------------------------------|
| `-addr`     | `SHORTORDER_ADDR`    | `:80` (`:8080` on Windows)    | HTTP listen address                       |
| `-printer`  | `SHORTORDER_PRINTER` | _(auto)_                      | Force a specific detected printer by name |
| `-width`    | `SHORTORDER_WIDTH`   | `576`                         | Print head width in dots (80mm=576, 58mm=384) |
| `-debug`    |                      | `false`                       | Verbose request logging                   |
| `-list`     |                      |                               | List detected printers and exit           |
| `-version`  |                      |                               | Print version and exit                    |

> Port `80` is the default on Linux/macOS (the Raspberry Pi service runs as root
> and binds it directly). On Windows it defaults to `8080` to avoid the common
> `http.sys`/IIS collision and the need for elevation.

## 🌐 HTTP API

| Method & path           | Purpose                                       |
|-------------------------|-----------------------------------------------|
| `GET  /`                | Web UI: devices, test prints, API docs        |
| `GET  /healthz`         | Liveness + version                            |
| `GET  /api/printers`    | List supported models and detected devices    |
| `POST /api/print/text`  | Print formatted text                          |
| `POST /api/print/qr`    | Render and print a QR code                     |
| `POST /api/print/image` | Print a PNG/JPEG/GIF as a dithered raster      |
| `POST /api/print/raw`   | Send a raw ESC/POS byte stream                 |
| `POST /api/cut`         | Feed and cut                                   |

📖 **Full request/response reference:** [docs/API.md](docs/API.md).

> 💡 For crisp receipt text use `/api/print/text` (the printer's native font).
> Image mode rasterizes at the head's dot density, so keep any text inside an
> image generously sized or it can print faint.

## 🍓 Deploy on a Raspberry Pi (Debian package)

shortorder publishes `.deb`, `.rpm`, and `.apk` packages. On a Raspberry Pi
(arm64), install the `.deb` and you get a self-starting print appliance:

```sh
sudo dpkg -i shortorder_*_linux_arm64.deb
```

The package:

- installs the binary to `/usr/bin/shortorder`,
- installs and **enables a `systemd` service** (`shortorder.service`) that starts
  on boot and restarts on failure,
- serves the UI + API on **port 80**,
- **sets the Pi's hostname to `shortorder`**, so it's reachable at
  **<http://shortorder.local/>** over mDNS.

```sh
sudo systemctl status shortorder      # check it's running
journalctl -u shortorder -f           # follow the logs
echo 'SHORTORDER_ADDR=:8080' | sudo tee /etc/default/shortorder   # override settings
```

Plug a supported USB printer into the Pi and it's detected automatically.

## 🛠️ How it works

Receipt printers speak **ESC/POS**: a print job is just a byte stream of
printable text plus control sequences for formatting, raster bitmaps, and the
cutter. shortorder renders each API request to ESC/POS and writes it **straight
to the printer's USB device** — no spooler, print queue, or driver install:

- **Windows** — finds the printer via the USB printer device interface
  (`GUID_DEVINTERFACE_USBPRINT`) and writes with `CreateFile`/`WriteFile`.
- **Linux / Raspberry Pi** — finds the printer in `sysfs`, matches its USB
  VID/PID against the allowlist, and writes to its `usblp` node (`/dev/usb/lp0`).

QR codes and images are rendered to a 1-bit raster (Floyd–Steinberg dithered for
photos and grayscale) and sent with the `GS v 0` raster command, so output is
identical across printers regardless of their native graphics support.

## 📋 Logging

Every request is logged as a single structured line — method, path, status,
size, duration, client, and outcome — with the level keyed to the result
(`INFO` for 2xx/3xx, `WARN` for 4xx, `ERROR` for 5xx) so invalid commands and
print failures stand out:

```
level=INFO  msg=request method=POST path=/api/print/text status=200 ... info="printed shortorder-text on \"EPSON TM-T20II\" (44 bytes)"
level=WARN  msg=request method=POST path=/api/print/text status=400 ... info="text is required"
```

## 📦 Building releases

[GoReleaser](https://goreleaser.com) builds the full
`linux / windows / darwin × amd64 / arm64` matrix plus the Linux packages:

```sh
make check          # validate .goreleaser.yaml
make snapshot       # full matrix + .deb/.rpm/.apk into ./dist (no publish)
make tag V=v0.1.0   # tag + push -> CI cuts a GitHub release
```

## ❓ FAQ

**Does it need printer drivers?** No. shortorder talks to the printer's raw USB
endpoint directly. There's no driver, print queue, or spooler to configure.

**What printers are supported?** The Volcora v-WRP2-A1W and Epson-compatible
ESC/POS USB receipt printers. Adding a model is a one-line allowlist change.

**Which platforms?** Printing works on **Windows** and **Linux** (including
Raspberry Pi). The binary also builds and runs on macOS, where the print path is
a stub pending a CUPS backend.

**Can an AI agent or LLM use this?** Yes — that's a primary use case. It's a
plain REST API, so any agent that can make an HTTP POST can print text, QR codes,
or images in the physical world.

**Does it phone home or need the internet?** No. It's a fully local service.

**What's the wire protocol to the printer?** ESC/POS over USB (the `usbprint`
device interface on Windows, the `usblp` character device on Linux).

## 📄 License

[Apache 2.0](LICENSE). Contributions and new printer models welcome.
