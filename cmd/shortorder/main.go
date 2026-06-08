// Command shortorder runs a long-lived HTTP service that prints text, QR codes,
// and images to a supported USB thermal receipt printer (the Volcora v-WRP2-A1W)
// and cuts the receipt, all driven over a small JSON API.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

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
	var (
		addr        = flag.String("addr", envOr("SHORTORDER_ADDR", defaultAddr()), "HTTP listen address")
		printerName = flag.String("printer", os.Getenv("SHORTORDER_PRINTER"), "force a specific printer queue name (default: first detected supported printer)")
		width       = flag.Int("width", envInt("SHORTORDER_WIDTH", 576), "print head width in dots (80mm=576, 58mm=384)")
		debug       = flag.Bool("debug", false, "verbose request logging")
		showVersion = flag.Bool("version", false, "print version and exit")
		list        = flag.Bool("list", false, "list detected supported printers and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("shortorder %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown error", "err", err)
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
