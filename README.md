# Composable Operations

A workflow engine where business processes are expressed as chains of operations defined in YAML. Backed by Temporal for durable execution, human-in-the-loop approval gates, and retries.

The demo scenario is an **incident-response pipeline**: check service metrics, check recent logs, ask an LLM to identify the root cause, then either require a human to approve remediation (v1) or let the LLM decide autonomously (v2). Swapping from v1 to v2 is a single YAML edit — no code change, no redeploy.

---

## Prerequisites

| Tool | Purpose | Install |
|------|---------|---------|
| Go 1.21+ | Build the binaries | https://go.dev/dl |
| Temporal CLI | Local Temporal server (no Docker needed) | https://docs.temporal.io/cli#install |
| Ollama | Run LLMs locally (only if `LLM_PROVIDER=ollama`) | https://ollama.com |

---

## Quickstart

### 1. Configure environment

Copy the example env file and fill in the values for your chosen LLM provider:

```bash
cp .env.example .env
```

### 2. Start Temporal and the server

```bash
make dev
```

This starts the Temporal dev server in the background, waits for it to be ready, then starts the API + worker process. Ctrl+C stops both.

Temporal web UI is available at http://localhost:8233 while the server is running.

### 3. Run a flow

Open a second terminal.

**v1: Human-in-the-loop** (you will be prompted to approve or reject remediation):

```bash
go run ./cmd/flowctl run --flow incident-response-v1
```

**v2: Fully autonomous** (LLM decides whether to remediate):

```bash
go run ./cmd/flowctl run --flow incident-response-v2
```

The CLI polls the run status, renders per-step progress in place, and for v1 prompts you to approve or reject when the flow reaches the human gate.

---

## LLM providers

Set `LLM_PROVIDER` in `.env` to select the backend.

### Ollama (default, local, no API key)

```
LLM_PROVIDER=ollama
OLLAMA_BASE_URL=http://localhost:11434
OLLAMA_MODEL=llama3.2
```

Start Ollama and pull a model before running:

```bash
ollama serve          # if not already running as a service
ollama pull llama3.2
```

### Claude (Anthropic API)

```
LLM_PROVIDER=claude
ANTHROPIC_API_KEY=sk-ant-...
CLAUDE_MODEL=claude-sonnet-4-6
```

No local model required. `CLAUDE_MODEL` defaults to `claude-sonnet-4-6` if omitted.

---

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_PROVIDER` | `ollama` | LLM backend: `ollama` or `claude` |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Ollama server URL |
| `OLLAMA_MODEL` | `llama3.2` | Model name for Ollama |
| `ANTHROPIC_API_KEY` | _(required for claude)_ | Anthropic API key |
| `CLAUDE_MODEL` | `claude-sonnet-4-6` | Model name for Claude |
| `TEMPORAL_ADDRESS` | `localhost:7233` | Temporal server address |
| `FLOWS_DIR` | `flows` | Directory containing YAML flow definitions |
| `LISTEN_ADDR` | `:8080` | HTTP API listen address |

---

## Flow definitions

| File | Description |
|------|-------------|
| `flows/incident-response-v1.yaml` | 5-step HITL pipeline: metrics check → logs check → LLM analysis → human approval → remediate |
| `flows/incident-response-v2.yaml` | Same pipeline with an LLM decision step replacing the human gate |
| `flows/incident-response.sample.json` | Sample trigger payload (PgBouncer pool-exhaustion scenario) |

### How sample data works

Each flow YAML declares which sample file to use via the `mock_data` field:

```yaml
name: incident-response
mock_data: incident-response.sample.json
steps:
  ...
```

When `flowctl` starts a run it loads the named JSON file and uses it as the initial input envelope. Ops like `metrics.check` and `logs.check` are pass-through in demo mode: they read fields already present in the envelope (cpu_usage, logs, etc.) and forward them unchanged. In production, replace those ops with implementations that call Prometheus, Loki, etc.

---

## Adding a new op type

1. Create a file in `internal/ops/` and implement `core.Operation`.
   - Activity ops implement `Execute(ctx, input, params)`.
   - Human gate ops implement the `core.HumanGate` interface.
2. Register it in `internal/ops/register.go` via `RegisterBuiltins`.
3. Reference the new `type` in any flow YAML.

No workflow code changes needed. The engine dispatches by type name through the registry at load time.

---

## Development

```bash
make fmt        # format
make lint       # lint (must pass with zero warnings)
make build-all  # compile server and flowctl binaries
make test       # run unit + integration tests
make dev        # start Temporal + server (Ctrl+C stops both)
```
