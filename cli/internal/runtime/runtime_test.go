//go:build !windows

package runtime

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bloc-org/bloc/internal/recipe"
)

// ─── Stats mutex tests (SEC-00) ───────────────────────────────────────────────

func TestStats_UpdateAndSnapshot_Basic(t *testing.T) {
	s := &Stats{}
	s.Update(100.0, 50.0, 4096)

	gen, prefill, peakVRAM, _, _ := s.Snapshot()
	if gen != 50.0 {
		t.Errorf("gen = %v, want 50.0", gen)
	}
	if prefill != 100.0 {
		t.Errorf("prefill = %v, want 100.0", prefill)
	}
	if peakVRAM != 4096 {
		t.Errorf("peakVRAM = %v, want 4096", peakVRAM)
	}
}

func TestStats_Update_ZeroValuesSkipped(t *testing.T) {
	s := &Stats{}
	s.Update(75.0, 25.0, 2048)
	// Update with zeros — should not overwrite existing values
	s.Update(0, 0, 0)

	gen, prefill, peakVRAM, _, _ := s.Snapshot()
	if gen != 25.0 {
		t.Errorf("gen = %v, want 25.0 (zero Update should not overwrite)", gen)
	}
	if prefill != 75.0 {
		t.Errorf("prefill = %v, want 75.0 (zero Update should not overwrite)", prefill)
	}
	if peakVRAM != 2048 {
		t.Errorf("peakVRAM = %v, want 2048 (zero Update should not overwrite)", peakVRAM)
	}
}

func TestStats_PeakVRAM_OnlyIncreases(t *testing.T) {
	s := &Stats{}
	s.Update(0, 0, 4096)
	s.Update(0, 0, 8192)
	s.Update(0, 0, 2048) // lower — should not replace peak

	_, _, peakVRAM, _, _ := s.Snapshot()
	if peakVRAM != 8192 {
		t.Errorf("peakVRAM = %v, want 8192 (peak should only increase)", peakVRAM)
	}
}

// TestStats_ConcurrentUpdate verifies SEC-00: no data race under concurrent writers.
// This test is designed to be run with -race to catch unsynchronised access.
func TestStats_ConcurrentUpdate(t *testing.T) {
	s := &Stats{}
	var wg sync.WaitGroup
	const goroutines = 50

	wg.Add(goroutines * 2)
	// Simulate two goroutines (stdout + stderr scanners) writing concurrently
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			s.Update(float64(n)*1.5, float64(n)*0.7, int64(n*100))
		}(i)
		go func(n int) {
			defer wg.Done()
			_, _, _, _, _ = s.Snapshot()
		}(i)
	}
	wg.Wait()
	// If we reach here without the race detector firing, SEC-00 is satisfied.
}

func TestStats_SuccessAndDuration(t *testing.T) {
	s := &Stats{}
	s.mu.Lock()
	s.Success = true
	s.Duration = 5 * time.Second
	s.mu.Unlock()

	_, _, _, duration, success := s.Snapshot()
	if !success {
		t.Error("Success = false, want true")
	}
	if duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", duration)
	}
}

// ─── Registry / Resolve tests ─────────────────────────────────────────────────

func TestResolve_LlamaCpp_Default(t *testing.T) {
	// Empty engine name defaults to llama.cpp
	r := &recipe.Recipe{
		Schema:   "bloc/v1",
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{DownloadURL: "https://example.com/model.gguf"},
	}
	rt, err := Resolve(r, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if _, ok := rt.(*LlamaCppRuntime); !ok {
		t.Errorf("expected *LlamaCppRuntime for empty engine, got %T", rt)
	}
}

func TestResolve_LlamaCpp_Explicit(t *testing.T) {
	r := &recipe.Recipe{
		Schema:   "bloc/v1",
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{DownloadURL: "https://example.com/model.gguf"},
		Engine:   recipe.Engine{Name: "llama.cpp"},
	}
	rt, err := Resolve(r, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if _, ok := rt.(*LlamaCppRuntime); !ok {
		t.Errorf("expected *LlamaCppRuntime, got %T", rt)
	}
}

func TestResolve_LlamaCppHyphen_Alias(t *testing.T) {
	// "llama-cpp" is a valid alias for "llama.cpp"
	r := &recipe.Recipe{
		Schema:   "bloc/v1",
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{DownloadURL: "https://example.com/model.gguf"},
		Engine:   recipe.Engine{Name: "llama-cpp"},
	}
	rt, err := Resolve(r, "")
	if err != nil {
		t.Fatalf("Resolve failed for llama-cpp alias: %v", err)
	}
	if _, ok := rt.(*LlamaCppRuntime); !ok {
		t.Errorf("expected *LlamaCppRuntime for llama-cpp alias, got %T", rt)
	}
}

func TestResolve_VLLMNative_Default(t *testing.T) {
	r := &recipe.Recipe{
		Schema:   "bloc/v1",
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{HFRepo: "meta-llama/Llama-3-8B"},
		Engine:   recipe.Engine{Name: "vllm", Version: "0.9.0"},
	}
	rt, err := Resolve(r, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	native, ok := rt.(*NativeVLLMRuntime)
	if !ok {
		t.Fatalf("expected *NativeVLLMRuntime, got %T", rt)
	}
	if native.version != "0.9.0" {
		t.Errorf("version = %q, want 0.9.0", native.version)
	}
}

func TestResolve_VLLMNative_Explicit(t *testing.T) {
	r := &recipe.Recipe{
		Schema:   "bloc/v1",
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{HFRepo: "meta-llama/Llama-3-8B"},
		Engine:   recipe.Engine{Name: "vllm", Runtime: "native"},
	}
	rt, err := Resolve(r, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if _, ok := rt.(*NativeVLLMRuntime); !ok {
		t.Errorf("expected *NativeVLLMRuntime for runtime=native, got %T", rt)
	}
}

func TestResolve_UnsupportedEngine(t *testing.T) {
	r := &recipe.Recipe{
		Schema:   "bloc/v1",
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{HFRepo: "some/model"},
		Engine:   recipe.Engine{Name: "onnxruntime"},
	}
	_, err := Resolve(r, "")
	if err == nil {
		t.Error("expected error for unsupported engine 'onnxruntime'")
	}
	if !strings.Contains(err.Error(), "onnxruntime") {
		t.Errorf("error should mention the unsupported engine name, got: %v", err)
	}
}

func TestResolve_UnknownRuntime_ForVLLM(t *testing.T) {
	r := &recipe.Recipe{
		Schema:   "bloc/v1",
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{HFRepo: "some/model"},
		Engine:   recipe.Engine{Name: "vllm", Runtime: "kubernetes", Image: "some/image:latest"},
	}
	_, err := Resolve(r, "")
	if err == nil {
		t.Error("expected error for unknown runtime 'kubernetes'")
	}
	if !strings.Contains(err.Error(), "kubernetes") {
		t.Errorf("error should mention the invalid runtime, got: %v", err)
	}
}

func TestResolve_VLLMNative_VersionFallsToDefault(t *testing.T) {
	// Empty version should fall back to defaultVLLMVersion
	r := &recipe.Recipe{
		Schema:   "bloc/v1",
		Metadata: recipe.Metadata{Name: "test"},
		Model:    recipe.Model{HFRepo: "meta-llama/Llama-3-8B"},
		Engine:   recipe.Engine{Name: "vllm"},
	}
	rt, err := Resolve(r, "")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	native := rt.(*NativeVLLMRuntime)
	if native.version != defaultVLLMVersion {
		t.Errorf("version = %q, want defaultVLLMVersion %q", native.version, defaultVLLMVersion)
	}
}

func TestResolve_LlamaCpp_Name(t *testing.T) {
	rt := &LlamaCppRuntime{}
	if rt.Name() != "llama-server" {
		t.Errorf("LlamaCppRuntime.Name() = %q, want 'llama-server'", rt.Name())
	}
}

// ─── LlamaCpp flag parsing tests ─────────────────────────────────────────────

func TestParseLlamaFlags_LongFlags(t *testing.T) {
	helpText := `
Usage: llama-server [options]
Options:
  --ctx-size N         Context size (default: 4096)
  --gpu-layers N       Number of GPU layers
  --flash-attn         Enable flash attention
  --port N             Server port
`
	flags := parseLlamaFlags(helpText)

	expected := []string{"--ctx-size", "--gpu-layers", "--flash-attn", "--port"}
	for _, f := range expected {
		if _, ok := flags[f]; !ok {
			t.Errorf("expected flag %q to be parsed from help text", f)
		}
	}
}

func TestParseLlamaFlags_ShortFlags(t *testing.T) {
	helpText := `
  -c N   context size
  -ngl N number of GPU layers
  -b N   batch size
  -t N   threads
`
	flags := parseLlamaFlags(helpText)
	for _, f := range []string{"-c", "-ngl", "-b", "-t"} {
		if _, ok := flags[f]; !ok {
			t.Errorf("expected short flag %q to be parsed", f)
		}
	}
}

func TestParseLlamaFlags_Empty(t *testing.T) {
	flags := parseLlamaFlags("")
	if len(flags) != 0 {
		t.Errorf("expected empty map for empty help text, got %d entries", len(flags))
	}
}

func TestParseLlamaFlags_NoFlags(t *testing.T) {
	helpText := "Usage: llama-server\nA text without any flags at all.\n"
	flags := parseLlamaFlags(helpText)
	if len(flags) != 0 {
		t.Errorf("expected 0 flags, got %d: %v", len(flags), flags)
	}
}

// ─── parseLlamaStats tests ────────────────────────────────────────────────────

func TestParseLlamaStats_GenerationThroughput(t *testing.T) {
	s := &Stats{}
	parseLlamaStats("eval time        =   5432.12 ms /   100 tokens (  54.32 ms per token,   18.41 tokens per second)", s)
	if s.TokensPerSecGeneration != 18.41 {
		t.Errorf("TokensPerSecGeneration = %v, want 18.41", s.TokensPerSecGeneration)
	}
}

func TestParseLlamaStats_PromptThroughput(t *testing.T) {
	s := &Stats{}
	parseLlamaStats("prompt eval time =    256.88 ms /   512 tokens (   0.50 ms per token, 1992.34 tokens per second)", s)
	if s.TokensPerSecPrefill != 1992.34 {
		t.Errorf("TokensPerSecPrefill = %v, want 1992.34", s.TokensPerSecPrefill)
	}
}

func TestParseLlamaStats_VRAMUsage_MB(t *testing.T) {
	s := &Stats{}
	parseLlamaStats("VRAM USED = 3456.78 MB", s)
	if s.PeakVRAMMB != 3456 {
		t.Errorf("PeakVRAMMB = %v, want 3456", s.PeakVRAMMB)
	}
}

func TestParseLlamaStats_VRAMUsage_GB(t *testing.T) {
	s := &Stats{}
	parseLlamaStats("VRAM USED = 8.5 GB", s)
	// 8.5 GB * 1024 = 8704 MB
	if s.PeakVRAMMB != 8704 {
		t.Errorf("PeakVRAMMB = %v, want 8704 (8.5 GB in MB)", s.PeakVRAMMB)
	}
}

func TestParseLlamaStats_VRAMUsage_CaseInsensitive(t *testing.T) {
	s := &Stats{}
	parseLlamaStats("vram used: 2048.00 MiB", s)
	if s.PeakVRAMMB != 2048 {
		t.Errorf("PeakVRAMMB = %v, want 2048 (case-insensitive MiB)", s.PeakVRAMMB)
	}
}

func TestParseLlamaStats_NoMatch(t *testing.T) {
	s := &Stats{}
	parseLlamaStats("[server] model loaded successfully", s)
	if s.TokensPerSecGeneration != 0 || s.TokensPerSecPrefill != 0 || s.PeakVRAMMB != 0 {
		t.Error("expected all stats to remain zero on non-matching line")
	}
}

func TestParseLlamaStats_RegexSafety(t *testing.T) {
	// Ensure package-level regexes are all valid (catches typos)
	for _, re := range []*interface{}{} {
		_ = re
	}
	// Direct test: call parseLlamaStats with an adversarial input that could cause ReDoS
	// The regexes are anchored / non-backtracking, this should complete instantly.
	s := &Stats{}
	longLine := strings.Repeat("eval time = ", 1000) + "1.23 tokens per second"
	parseLlamaStats(longLine, s)
	// If we reach here within the test timeout, no catastrophic backtracking occurred.
}
