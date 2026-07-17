# Composable Operations

A workflow engine where business processes are expressed as chains of operations defined in YAML. Backed by Temporal for durable execution, human-in-the-loop approval gates, and retries.

The demo scenario is an **incident-response pipeline**: check service metrics, check recent logs, ask an LLM to identify the root cause, then either require a human to approve remediation (v1) or let the LLM decide autonomously (v2). Swapping from v1 to v2 is a single YAML edit — no code change, no redeploy.

---

## Prerequisites

| Tool | Purpose | Install |
|------|---------|---------|
| Go 1.21+ | Build the binaries | https://go.dev/dl |
| Temporal CLI | Local Temporal server (no Docker needed) | https://docs.temporal.io/cli#install |
| Ollama | Run LLMs locally | https://ollama.com |

### Start Temporal (pick one)

**Option A — Temporal CLI (recommended, no Docker):**
```bash
temporal server start-dev
```
Web UI: http://localhost:8233

**Option B — Docker:**
```bash
docker run --rm -p 7233:7233 -p 8233:8233 temporalio/temporal-dev-server
```

### Start Ollama and pull a model

```bash
ollama serve            # if not already running as a service
ollama pull llama3.2    # or any model you prefer
```

---

## Running the demo

Open three terminals.

### Terminal 1 — Worker

```bash
OLLAMA_MODEL=llama3.2 go run ./cmd/worker
```

The worker connects to Temporal and registers the RunFlow workflow and ExecuteOperation activity.

### Terminal 2 — API server

```bash
OLLAMA_MODEL=llama3.2 go run ./cmd/server
```

Starts the HTTP API on `:8080`.

### Terminal 3 — CLI

**v1: Human-in-the-loop** (you will be prompted to approve or reject remediation):

```bash
go run ./cmd/flowctl run --flow incident-response-v1
```

**v2: Fully autonomous** (LLM decides whether to remediate):

```bash
go run ./cmd/flowctl run --flow incident-response-v2
```

The CLI polls the run status, renders per-step progress, and — for v1 — prompts you to approve or reject when the flow reaches the human gate.

---

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LLM_PROVIDER` | `ollama` | LLM backend to use |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Ollama server URL |
| `OLLAMA_MODEL` | `llama3.2` | Model to use for all LLM steps |
| `TEMPORAL_ADDRESS` | `localhost:7233` | Temporal server address |
| `FLOWS_DIR` | `flows` | Directory containing YAML flow definitions |
| `LISTEN_ADDR` | `:8080` | HTTP API listen address |

---

## Flow definitions

| File | Description |
|------|-------------|
| `flows/incident-response-v1.yaml` | 5-step HITL pipeline: metrics → logs → LLM analysis → human approval → remediate |
| `flows/incident-response-v2.yaml` | Same pipeline with LLM decision replacing the human gate |
| `flows/incident-response.sample.json` | Sample trigger payload for the CLI |

The fixture metrics and log lines are embedded in the YAML `params` so the demo runs without real infrastructure. In production, swap `metrics.check` and `logs.check` for ops that call Prometheus, Loki, etc.

---

## Adding a new op type

1. Implement `core.Operation` (and `core.ActivityOp` or `core.HumanGate`) in `internal/ops/`.
2. Register it in `internal/ops/register.go` via `RegisterBuiltins`.
3. Reference the new `type` in any flow YAML.

No workflow code changes needed — the engine dispatches by type name via the registry.

---

## Development

```bash
make fmt        # format
make lint       # lint (must pass with zero warnings)
make build-all  # compile all binaries
make test       # run unit + integration tests
make pr         # run all checks (fmt + lint + build + test)
```
