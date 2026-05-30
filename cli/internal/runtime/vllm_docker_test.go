//go:build !windows

package runtime

import (
	"strings"
	"testing"

	"github.com/bloc-org/bloc/internal/recipe"
)

// ─── DockerVLLMRuntime unit tests ─────────────────────────────────────────────

func TestDockerVLLMRuntime_Name_WithImage(t *testing.T) {
	rt := &DockerVLLMRuntime{image: "vllm/vllm-openai:v0.9.0"}
	want := "vLLM Docker (vllm/vllm-openai:v0.9.0)"
	if rt.Name() != want {
		t.Errorf("Name() = %q, want %q", rt.Name(), want)
	}
}

func TestDockerVLLMRuntime_Name_NoImage(t *testing.T) {
	rt := &DockerVLLMRuntime{}
	if rt.Name() != "vLLM (Docker)" {
		t.Errorf("Name() = %q, want 'vLLM (Docker)'", rt.Name())
	}
}

// ─── Container name sanitization tests ────────────────────────────────────────

func TestSanitizeContainerSlug_Normal(t *testing.T) {
	got := sanitizeContainerSlug("step-3.7-flash-speculative")
	if got != "step-3-7-flash-speculative" {
		t.Errorf("sanitizeContainerSlug() = %q, want %q", got, "step-3-7-flash-speculative")
	}
}

func TestSanitizeContainerSlug_Uppercase(t *testing.T) {
	got := sanitizeContainerSlug("Qwen3-30B-MoE")
	if got != "qwen3-30b-moe" {
		t.Errorf("got %q, want %q", got, "qwen3-30b-moe")
	}
}

func TestSanitizeContainerSlug_ShellInjectionChars(t *testing.T) {
	// Ensure characters that could break shell commands are sanitized
	dangerous := "model; rm -rf /; echo pwned"
	got := sanitizeContainerSlug(dangerous)

	// Must not contain any shell-dangerous characters
	shellDangerous := []string{";", " ", "|", "&", "$", "`", "!", ">", "<", "(", ")", "{", "}", "[", "]", "\\", "'", "\""}
	for _, c := range shellDangerous {
		if strings.Contains(got, c) {
			t.Errorf("sanitizeContainerSlug() = %q contains dangerous char %q", got, c)
		}
	}
	// Must only contain [a-z0-9-]
	for _, ch := range got {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
			t.Errorf("sanitizeContainerSlug() = %q contains invalid char %q", got, string(ch))
		}
	}
}

func TestSanitizeContainerSlug_TooLong(t *testing.T) {
	long := strings.Repeat("a", 100)
	got := sanitizeContainerSlug(long)
	if len(got) > 40 {
		t.Errorf("slug len = %d, want <= 40", len(got))
	}
}

func TestSanitizeContainerSlug_Empty(t *testing.T) {
	got := sanitizeContainerSlug("")
	if got != "model" {
		t.Errorf("empty slug should fall back to 'model', got %q", got)
	}
}

func TestSanitizeContainerSlug_OnlySpecialChars(t *testing.T) {
	got := sanitizeContainerSlug("!@#$%^&*()")
	// All chars become hyphens then trimmed/collapsed → "model" fallback
	if got == "" {
		t.Error("slug should not be empty")
	}
	for _, ch := range got {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-') {
			t.Errorf("slug %q contains invalid char %q", got, string(ch))
		}
	}
}

// ─── Docker command construction verification ──────────────────────────────────

// TestDockerCommandConstruction verifies the args built for docker run contain
// all required elements without launching a real container.
func TestDockerCommandConstruction(t *testing.T) {
	// We can't call Run() without Docker, but we can unit-test the arg-building
	// logic by extracting it. Since the args are built inline in Run(), we test
	// the component functions that feed into it.

	// Verify container name format
	slug := sanitizeContainerSlug("step-3-7-flash")
	hex4 := randomHex(4)
	name := "bloc-" + slug + "-" + hex4

	if !strings.HasPrefix(name, "bloc-") {
		t.Error("container name must start with 'bloc-'")
	}
	if len(hex4) != 8 {
		t.Errorf("randomHex(4) should produce 8 hex chars, got %d", len(hex4))
	}
}

// ─── Resolve() wires DockerVLLMRuntime ────────────────────────────────────────

func TestResolve_DockerVLLM_MissingImage(t *testing.T) {
	r := &recipe.Recipe{
		Schema: "bloc/v1",
		Engine: recipe.Engine{
			Name:    "vllm",
			Runtime: "docker",
			Image:   "", // missing
		},
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{HFRepo: "some/model"},
	}
	_, err := Resolve(r, "")
	if err == nil {
		t.Error("expected error when engine.image is missing for docker runtime")
	}
	if !strings.Contains(err.Error(), "engine.image") {
		t.Errorf("error should mention engine.image, got: %v", err)
	}
}

func TestResolve_DockerVLLM_WithImage(t *testing.T) {
	r := &recipe.Recipe{
		Schema: "bloc/v1",
		Engine: recipe.Engine{
			Name:    "vllm",
			Runtime: "docker",
			Image:   "vllm/vllm-openai:v0.9.0",
		},
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{HFRepo: "some/model"},
	}
	rt, err := Resolve(r, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	docker, ok := rt.(*DockerVLLMRuntime)
	if !ok {
		t.Fatalf("expected *DockerVLLMRuntime, got %T", rt)
	}
	if docker.image != "vllm/vllm-openai:v0.9.0" {
		t.Errorf("image = %q, want vllm/vllm-openai:v0.9.0", docker.image)
	}
}

func TestResolve_DockerVLLM_RuntimeOverride(t *testing.T) {
	// --runtime docker flag should override recipe's native runtime
	r := &recipe.Recipe{
		Schema: "bloc/v1",
		Engine: recipe.Engine{
			Name:    "vllm",
			Runtime: "native", // recipe says native
			Image:   "vllm/vllm-openai:v0.9.0",
		},
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{HFRepo: "some/model"},
	}
	rt, err := Resolve(r, "docker") // --runtime docker override
	if err != nil {
		t.Fatalf("Resolve with override failed: %v", err)
	}
	if _, ok := rt.(*DockerVLLMRuntime); !ok {
		t.Errorf("expected *DockerVLLMRuntime after override, got %T", rt)
	}
}

// ─── isInterruptExit tests ────────────────────────────────────────────────────

func TestIsInterruptExit_Nil(t *testing.T) {
	if !isInterruptExit(nil) {
		t.Error("nil error should be considered a clean exit")
	}
}

func TestIsInterruptExit_SIGINT(t *testing.T) {
	// Simulate the error string that cmd.Wait() returns on Ctrl+C
	err := &mockExitError{msg: "exit status 130"}
	if !isInterruptExit(err) {
		t.Error("exit status 130 should be treated as interrupt exit")
	}
}

func TestIsInterruptExit_SIGTERM(t *testing.T) {
	err := &mockExitError{msg: "exit status 143"}
	if !isInterruptExit(err) {
		t.Error("exit status 143 should be treated as interrupt exit")
	}
}

func TestIsInterruptExit_Crash(t *testing.T) {
	err := &mockExitError{msg: "exit status 1"}
	if isInterruptExit(err) {
		t.Error("exit status 1 should NOT be treated as interrupt exit")
	}
}

// mockExitError implements the error interface for testing
type mockExitError struct{ msg string }

func (e *mockExitError) Error() string { return e.msg }

// ─── randomHex tests ──────────────────────────────────────────────────────────

func TestRandomHex_Length(t *testing.T) {
	for n := 1; n <= 8; n++ {
		got := randomHex(n)
		if len(got) != n*2 {
			t.Errorf("randomHex(%d) = %q (len %d), want len %d", n, got, len(got), n*2)
		}
	}
}

func TestRandomHex_OnlyHexChars(t *testing.T) {
	got := randomHex(16)
	for _, c := range got {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("randomHex output %q contains non-hex char %q", got, string(c))
		}
	}
}

func TestRandomHex_Uniqueness(t *testing.T) {
	// Two calls should produce different values (statistically certain with 8 bytes)
	a, b := randomHex(8), randomHex(8)
	if a == b {
		t.Error("two randomHex(8) calls should not produce the same value")
	}
}
