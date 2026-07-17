package main

import (
	"context"
	"log/slog"
	"os"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"composable-operations/internal/engine"
	"composable-operations/internal/llm"
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
	temporalAddr := envOr("TEMPORAL_ADDRESS", "localhost:7233")

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

	slog.Info("Worker starting", "task_queue", engine.TaskQueue)
	return w.Run(worker.InterruptCh())
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
