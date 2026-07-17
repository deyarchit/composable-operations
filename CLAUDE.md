# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Go workflow engine where business processes are expressed as chains of operations defined in YAML configuration. Backed by Temporal for durable execution, suspend/resume (human approval), and retries.

- **cmd/server/** - HTTP API (start runs, submit approvals, query status)
- **cmd/worker/** - Temporal worker (executes workflows and activities)
- **cmd/flowctl/** - Demo CLI (trigger runs, poll status, prompt for approval)
- **internal/** - Core packages: registry, engine, ops, loader, api
- **flows/** - YAML flow definitions (moderation.yaml, etc.)

See `.notes/feature-composable-operations.md` for full design spec.

## Code Change Workflow

After any non-trivial code change, run these steps in order:

```bash
# 1. Format code to match repository style
make fmt

# 2. Lint -- must pass with zero warnings
make lint

# 3. Build all binaries to catch compilation errors
make build-all

# 4. Full unit test suite -- all tests must pass before work is done
make test
```

## Implementation Conventions

- **Operation registry**: ops register themselves at init time via `registry.Register`. The loader validates all step types against the registry before any run starts.
- **Payload envelope**: a `map[string]any` threaded op-to-op. Ops read fields they need and add new ones; they do not remove fields.
- **FlowDefinition passed as workflow input**: ensures a run always executes the version it started with, even if the YAML changes mid-run.
- **One generic `ExecuteOperation` activity**: dispatches to the registry by step type. Activity-kind ops go through this; HumanGate ops are handled directly in the workflow via Temporal signals.
- **Signal naming**: human approval signals are scoped to the step id to support flows with multiple human gates.
- **LLMClient is a seam**: all LLM ops call through the `LLMClient` interface. No concrete provider is wired in yet.
- **Testing**: use real Temporal test environment (`go.temporal.io/sdk/testsuite`) for workflow and activity tests, not mocks of the Temporal client.
