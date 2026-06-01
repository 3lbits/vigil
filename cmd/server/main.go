package main

import (
	"context"
	"embed"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/3lbits/vigil/internal/app"
	"github.com/3lbits/vigil/internal/config"
)

//go:embed policies/authz.rego
var policySource string

//go:embed public/css/output.css public/js/htmx.min.js public/js/alpine.min.js public/js/alpine-data.js
var staticFS embed.FS

// Version is overridden at build time: go build -ldflags "-X main.Version=v1.2.3"
var Version = "dev"

func main() {
	startTime := time.Now()

	_ = godotenv.Load()
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if err := app.Run(ctx, cfg, app.Options{
		PolicySource: policySource,
		StaticFS:     staticFS,
		Version:      Version,
		StartTime:    startTime,
	}); err != nil {
		stop()
		slog.Error("server startup failed", "error", err)
		os.Exit(1)
	}
	stop()
}
