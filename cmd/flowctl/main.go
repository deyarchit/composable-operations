package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"composable-operations/internal/core"
)

func main() {
	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	flowName := runCmd.String("flow", "", "Flow name (required)")
	inputFile := runCmd.String("input", "", "Path to input JSON file (optional; defaults to flows/<flow>.sample.json)")
	apiURL := runCmd.String("api", envOr("API_URL", "http://localhost:8080"), "API base URL")

	if len(os.Args) < 2 || os.Args[1] != "run" {
		_, _ = fmt.Fprintln(os.Stderr, "usage: flowctl run --flow <name> [--input file.json] [--api url]")
		os.Exit(1)
	}

	if parseErr := runCmd.Parse(os.Args[2:]); parseErr != nil {
		os.Exit(1)
	}
	if *flowName == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: --flow is required")
		os.Exit(1)
	}

	inputPath := *inputFile
	if inputPath == "" {
		inputPath = fmt.Sprintf("flows/%s.sample.json", *flowName)
	}

	inputData, readErr := os.ReadFile(inputPath) //nolint:gosec // path from flags, operator-controlled
	if readErr != nil {
		slog.Error("Failed to read input file", "path", inputPath, "error", readErr)
		os.Exit(1)
	}
	var inputJSON map[string]any
	if unmarshalErr := json.Unmarshal(inputData, &inputJSON); unmarshalErr != nil {
		slog.Error("Input file is not valid JSON", "error", unmarshalErr)
		os.Exit(1)
	}

	runID, startErr := startRun(*apiURL, *flowName, inputJSON)
	if startErr != nil {
		slog.Error("Failed to start run", "error", startErr)
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(os.Stdout, "Started run %s\n\n", runID)

	if pollErr := pollLoop(*apiURL, runID); pollErr != nil {
		slog.Error("Run failed", "error", pollErr)
		os.Exit(1)
	}
}

func startRun(apiURL, flowName string, input map[string]any) (string, error) {
	body, _ := json.Marshal(map[string]any{"input": input})
	resp, err := http.Post(apiURL+"/flows/"+flowName+"/runs", "application/json", bytes.NewReader(body)) //nolint:noctx // demo CLI; context wiring would add noise
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("start run: %s: %s", resp.Status, b)
	}
	var out struct {
		RunID string `json:"run_id"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&out); decodeErr != nil {
		return "", decodeErr
	}
	return out.RunID, nil
}

func pollLoop(apiURL, runID string) error {
	for {
		status, err := getStatus(apiURL, runID)
		if err != nil {
			return err
		}

		renderStatus(status)

		switch status.State {
		case core.RunCompleted:
			_, _ = fmt.Fprintln(os.Stdout, "\nCompleted.")
			return nil
		case core.RunFailed:
			return fmt.Errorf("run failed: %s", status.Error)
		case core.RunWaitingApproval:
			if status.ApprovalRequest != nil {
				if approvalErr := promptApproval(apiURL, runID, status.ApprovalRequest); approvalErr != nil {
					return approvalErr
				}
			}
		}

		time.Sleep(2 * time.Second)
	}
}

func getStatus(apiURL, runID string) (*core.RunStatus, error) {
	resp, err := http.Get(apiURL + "/runs/" + runID) //nolint:noctx,gosec // demo CLI
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get status: %s: %s", resp.Status, b)
	}
	var status core.RunStatus
	if decodeErr := json.NewDecoder(resp.Body).Decode(&status); decodeErr != nil {
		return nil, decodeErr
	}
	return &status, nil
}

func renderStatus(status *core.RunStatus) {
	_, _ = fmt.Fprintf(os.Stdout, "\rRun %s - %s", status.RunID[:8], status.State)
	for _, step := range status.Steps {
		icon := stepIcon(step.Status)
		_, _ = fmt.Fprintf(os.Stdout, "\n  %s %s", icon, step.StepID)
	}
	_, _ = fmt.Fprintln(os.Stdout)
}

func stepIcon(s core.StepStatus) string {
	switch s {
	case core.StepCompleted:
		return "[ok]"
	case core.StepFailed:
		return "[fail]"
	case core.StepRunning:
		return "[...]"
	default:
		return "[ ]"
	}
}

func promptApproval(apiURL, runID string, req *core.ApprovalRequest) error {
	_, _ = fmt.Fprintf(os.Stdout, "\n--- Approval required for step %q ---\n", req.StepID)
	_, _ = fmt.Fprintf(os.Stdout, "Prompt: %s\n", req.Prompt)
	_, _ = fmt.Fprintln(os.Stdout, "Payload:")
	for k, v := range req.Payload {
		_, _ = fmt.Fprintf(os.Stdout, "  %s: %v\n", k, v)
	}

	_, _ = fmt.Fprint(os.Stdout, "\nApprove? (yes/no): ")
	var answer string
	if _, scanErr := fmt.Scanln(&answer); scanErr != nil {
		return fmt.Errorf("read approval: %w", scanErr)
	}
	approved := strings.ToLower(strings.TrimSpace(answer)) == "yes"

	_, _ = fmt.Fprint(os.Stdout, "Comment (optional): ")
	var comment string
	_, _ = fmt.Scanln(&comment)

	body, _ := json.Marshal(map[string]any{
		"step_id":  req.StepID,
		"approved": approved,
		"comment":  comment,
	})
	resp, err := http.Post(apiURL+"/runs/"+runID+"/approvals", "application/json", bytes.NewReader(body)) //nolint:noctx // demo CLI
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("submit approval: %s: %s", resp.Status, b)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
