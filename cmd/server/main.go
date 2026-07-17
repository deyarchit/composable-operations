package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"composable-operations/internal/api"
	"composable-operations/internal/engine"
	"composable-operations/internal/llm"
	"composable-operations/internal/loader"
	"composable-operations/internal/ops"
	"composable-operations/internal/registry"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load() // load .env if present; silently ignored when absent

	temporalAddr := envOr("TEMPORAL_ADDRESS", "localhost:7233")
	flowsDir := envOr("FLOWS_DIR", "flows")
	listenAddr := envOr("LISTEN_ADDR", ":8080")

	temporalClient, err := client.Dial(client.Options{HostPort: temporalAddr})
	if err != nil {
		return err
	}
	defer temporalClient.Close()

	llmClient, err := llm.NewChatModel(context.Background())
	if err != nil {
		return err
	}

	reg := registry.New()
	if regErr := ops.RegisterBuiltins(reg, llmClient); regErr != nil {
		return regErr
	}

	w := worker.New(temporalClient, engine.TaskQueue, worker.Options{})
	engine.Register(w, &engine.Workflows{Registry: reg}, &engine.Activities{Registry: reg})
	if startErr := w.Start(); startErr != nil {
		return startErr
	}
	defer w.Stop()

	ldr := loader.New(flowsDir, reg)
	h := api.NewHandler(temporalClient, ldr)

	e := echo.New()
	e.HideBanner = true
	api.RegisterRoutes(e, h)

	go func() {
		slog.Info("HTTP server listening", "addr", listenAddr)
		if serverErr := e.Start(listenAddr); serverErr != nil {
			slog.Info("Server stopped", "reason", serverErr)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down")
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
