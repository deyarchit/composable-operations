package loader_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"composable-operations/internal/core"
	"composable-operations/internal/llm"
	"composable-operations/internal/loader"
	"composable-operations/internal/ops"
	"composable-operations/internal/registry"
)

func newTestLoader(t *testing.T, dir string) *loader.Loader {
	t.Helper()
	reg := registry.New()
	require.NoError(t, ops.RegisterBuiltins(reg, &llm.StubClient{}))
	return loader.New(dir, reg)
}

func writeYAML(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(content), 0o600))
}

func TestLoader_LoadsValidDefinition(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "simple", `
name: simple
steps:
  - id: scan
    type: pii.scan
    params:
      patterns:
        - '\d+'
  - id: do-score
    type: score
    params:
      weights:
        pii_found: 1.0
`)
	l := newTestLoader(t, dir)

	def, err := l.Load("simple")

	require.NoError(t, err)
	assert.Equal(t, "simple", def.Name)
	assert.Len(t, def.Steps, 2)
	assert.Equal(t, "scan", def.Steps[0].ID)
	assert.Equal(t, "pii.scan", def.Steps[0].Type)
}

func TestLoader_RejectsUnknownOpType(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "bad", `
name: bad
steps:
  - id: step1
    type: nonexistent.op
    params: {}
`)
	l := newTestLoader(t, dir)

	_, err := l.Load("bad")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown op type")
}

func TestLoader_RejectsDuplicateStepIDs(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "dupes", `
name: dupes
steps:
  - id: same-id
    type: publish
    params: {}
  - id: same-id
    type: publish
    params: {}
`)
	l := newTestLoader(t, dir)

	_, err := l.Load("dupes")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate step id")
}

func TestLoader_RejectsInvalidParams(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "bad-params", `
name: bad-params
steps:
  - id: classify
    type: llm.classify
    params:
      output_field: score
`)
	l := newTestLoader(t, dir)

	_, err := l.Load("bad-params")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt_template")
}

func TestLoader_RejectsEmptyStepList(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "empty", `
name: empty
steps: []
`)
	l := newTestLoader(t, dir)

	_, err := l.Load("empty")

	require.Error(t, err)
}

func TestLoader_RejectsMissingFile(t *testing.T) {
	l := newTestLoader(t, t.TempDir())

	_, err := l.Load("does-not-exist")

	require.Error(t, err)
}

func TestLoader_ModerationV1LoadsAndHasHumanApproval(t *testing.T) {
	l := newTestLoader(t, filepath.Join("..", "..", "flows"))
	def, err := l.Load("moderation-v1")
	require.NoError(t, err)
	assert.True(t, hasStepType(def, "human.approval"), "v1 must have a human.approval step")
}

func TestLoader_ModerationV2LoadsAndHasLLMDecision(t *testing.T) {
	l := newTestLoader(t, filepath.Join("..", "..", "flows"))
	def, err := l.Load("moderation-v2")
	require.NoError(t, err)
	assert.True(t, hasStepType(def, "llm.decision"), "v2 must have an llm.decision step")
}

func hasStepType(def *core.FlowDefinition, typeName string) bool {
	for _, s := range def.Steps {
		if s.Type == typeName {
			return true
		}
	}
	return false
}
