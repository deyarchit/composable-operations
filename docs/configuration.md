# Configuration & Operations Guide

Everything needed to set up, configure, run, and extend Composable Operations. For what the project *is* and the principles it demonstrates, see the [README](../README.md).

---

## Prerequisites

| Tool | Purpose | Install |
|------|---------|---------|
| Go 1.21+ | Build the binaries | https://go.dev/dl |
| Temporal CLI | Local Temporal dev server (no Docker needed) | https://docs.temporal.io/cli#install |
| Ollama | Run LLMs locally (only if `LLM_PROVIDER=ollama`) | https://ollama.com |

---

## Setup

### 1. Configure environment

Copy the example env file. The defaults target a local Ollama and need no API key:

```bash
cp .env.example .env
```

### 2. Start Temporal and the server

```bash
make dev
```

This starts the Temporal dev server in the background, waits for it to be ready, then starts the combined API + worker process. Ctrl+C stops both. The Temporal web UI is available at http://localhost:8233 while the server runs.

### 3. Run a flow

In a second terminal:

```bash
# v1: human-in-the-loop (you approve or reject remediation)
go run ./cmd/flowctl run --flow incident-response-v1

# v2: fully autonomous (the LLM decides)
go run ./cmd/flowctl run --flow incident-response-v2
```

`flowctl` polls the run, renders per-step progress in place, and (v1 only) prompts you at the human gate. A captured example of both runs is in the [README](../README.md#sample-run).

---

## LLM providers

Select the backend with `LLM_PROVIDER` in `.env`.

### Ollama (default, local, no API key)

```
LLM_PROVIDER=ollama
OLLAMA_BASE_URL=http://localhost:11434
OLLAMA_MODEL=gemma3:4b
```

Start Ollama and pull a model before running:

```bash
ollama serve          # if not already running as a service
ollama pull gemma3:4b
```

### Claude (Anthropic API)

```
LLM_PROVIDER=claude
ANTHROPIC_API_KEY=sk-ant-...
CLAUDE_MODEL=claude-sonnet-4-6
```

No local model required. `CLAUDE_MODEL` defaults to `claude-sonnet-4-6` when omitted.

---

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_PROVIDER` | `ollama` | LLM backend: `ollama` or `claude` |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Ollama server URL |
| `OLLAMA_MODEL` | `gemma3:4b` | Model name for Ollama |
| `ANTHROPIC_API_KEY` | _(required for claude)_ | Anthropic API key |
| `CLAUDE_MODEL` | `claude-sonnet-4-6` | Model name for Claude |
| `TEMPORAL_ADDRESS` | `localhost:7233` | Temporal server address |
| `FLOWS_DIR` | `flows` | Directory containing YAML flow definitions |
| `LISTEN_ADDR` | `:8080` | HTTP API listen address |

---

## Flow definitions

A flow definition is a YAML file: a `name`, an optional `mock_data` sample file, and an ordered list of `steps`. Each step has an `id`, a `type` (an op type registered in the engine), and a `params` map specific to that op.

```yaml
name: incident-response
mock_data: incident-response.sample.json
steps:
  - id: check-metrics
    type: metrics.check
    params: {}
  - id: analyze-incident
    type: llm.analyze
    params:
      prompt_template: "Analyze this incident: {{.service}} ..."
  - id: human-review
    type: human.approval
    params:
      prompt: "Approve remediation?"
      display_fields: [service, analysis]
  # ...
```

### Bundled flows

| File | Description |
|------|-------------|
| `flows/incident-response-v1.yaml` | 5-step pipeline with a **human approval** gate |
| `flows/incident-response-v2.yaml` | Same pipeline with an **LLM decision** in place of the gate |
| `flows/incident-response.sample.json` | Trigger payload (PgBouncer pool-exhaustion scenario) |

### How sample data works

Each flow declares its sample input via the `mock_data` field. When `flowctl` starts a run it loads that JSON file and uses it as the initial input envelope. In demo mode `metrics.check` and `logs.check` are pass-through: they read fields already present in the envelope (`cpu_usage`, `logs`, etc.) and forward them unchanged. In production, replace those ops with implementations that call Prometheus, Loki, and similar systems.

---

## Built-in operations

| Type | Kind | Behavior |
|------|------|----------|
| `metrics.check` | deterministic | Reads/forwards service metric fields (pass-through in demo) |
| `logs.check` | deterministic | Reads/forwards recent log fields (pass-through in demo) |
| `llm.analyze` | LLM | Renders a prompt, calls the LLM, writes the parsed `analysis` object |
| `human.approval` | human gate | Suspends the run and emits a `decision` on human approve/reject |
| `llm.decision` | LLM | Emits the same `decision` shape from an LLM verdict (drop-in for the gate) |
| `remediate` | deterministic | Runs only if `decision.approved`; logs the scale action (demo mode) |

The `human.approval` and `llm.decision` ops emit the identical `decision` output (`{approved, comment, by}`), which is what makes swapping one for the other a definition-only change.

---

## Adding a new op type

1. Create a file in `internal/ops/` and implement `core.Operation`.
   - Activity ops implement `Execute(ctx, input, params)`.
   - Human gate ops implement the `core.HumanGate` interface.
2. Register it in `internal/ops/register.go` via `RegisterBuiltins`.
3. Reference the new `type` in any flow YAML.

No workflow code changes are needed. The engine dispatches by type name through the registry at load time, so a newly registered op is immediately usable by any flow.

---

## Development

```bash
make fmt        # format
make lint       # lint (must pass with zero warnings)
make build-all  # compile server and flowctl binaries
make test       # run unit + integration tests
make dev        # start Temporal + server (Ctrl+C stops both)
```
