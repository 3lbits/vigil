package app

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/3lbits/vigil/internal/config"
)

var errConfigRequired = errors.New("config is required")

type Options struct {
	PolicySource string
	StaticFS     embed.FS
	Version      string
	StartTime    time.Time
}

func Run(ctx context.Context, cfg *config.Config, opts Options) error {
	if cfg == nil {
		return errConfigRequired
	}

	state, err := bootstrap(ctx, cfg, opts)
	if err != nil {
		return err
	}
	defer state.closeDB()

	mux, sm, err := buildMux(ctx, cfg, state, opts)
	if err != nil {
		return err
	}

	handler := withMiddleware(cfg, state, sm, mux)
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			slog.Error("server shutdown", "error", err)
		}
		state.obsShutdown()
	}()

	slog.Info("server starting", "addr", srv.Addr, "env", cfg.AppEnv)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}
