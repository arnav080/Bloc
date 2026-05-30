//go:build !windows

package runtime

import (
	"regexp"
	"testing"
)

// ─── NativeVLLMRuntime unit tests ─────────────────────────────────────────────

func TestResolveVLLMVersion_Pinned(t *testing.T) {
	got := resolveVLLMVersion("0.8.5")
	if got != "0.8.5" {
		t.Errorf("expected 0.8.5, got %s", got)
	}
}

func TestResolveVLLMVersion_Default(t *testing.T) {
	got := resolveVLLMVersion("")
	if got != defaultVLLMVersion {
		t.Errorf("expected defaultVLLMVersion %q, got %q", defaultVLLMVersion, got)
	}
}

func TestResolveVLLMVersion_Whitespace(t *testing.T) {
	// Whitespace-only version string should fall back to default
	got := resolveVLLMVersion("   ")
	if got != defaultVLLMVersion {
		t.Errorf("expected defaultVLLMVersion %q for whitespace input, got %q", defaultVLLMVersion, got)
	}
}

func TestNativeVLLMRuntime_Name_WithVersion(t *testing.T) {
	rt := &NativeVLLMRuntime{version: "0.9.0"}
	want := "vLLM 0.9.0 (native)"
	if rt.Name() != want {
		t.Errorf("Name() = %q, want %q", rt.Name(), want)
	}
}

func TestNativeVLLMRuntime_Name_NoVersion(t *testing.T) {
	rt := &NativeVLLMRuntime{}
	want := "vLLM (native)"
	if rt.Name() != want {
		t.Errorf("Name() = %q, want %q", rt.Name(), want)
	}
}

// ─── vLLM stats parser tests ──────────────────────────────────────────────────

func TestParseVLLMStats_PromptThroughput(t *testing.T) {
	s := &Stats{}
	parseVLLMStats("Avg prompt throughput: 123.4 tokens/s, Avg generation throughput: 45.6 tokens/s, ...", s)
	if s.TokensPerSecPrefill != 123.4 {
		t.Errorf("TokensPerSecPrefill = %v, want 123.4", s.TokensPerSecPrefill)
	}
	if s.TokensPerSecGeneration != 45.6 {
		t.Errorf("TokensPerSecGeneration = %v, want 45.6", s.TokensPerSecGeneration)
	}
}

func TestParseVLLMStats_KVCacheUsage(t *testing.T) {
	s := &Stats{}
	parseVLLMStats("GPU KV cache usage: 72.5%, CPU KV cache usage: 0.0%.", s)
	// 72.5 * 100 = 7250
	if s.PeakVRAMMB != 7250 {
		t.Errorf("PeakVRAMMB = %v, want 7250", s.PeakVRAMMB)
	}
}

func TestParseVLLMStats_PeakOnly_Increases(t *testing.T) {
	s := &Stats{}
	parseVLLMStats("GPU KV cache usage: 50.0%.", s)
	parseVLLMStats("GPU KV cache usage: 80.0%.", s)
	parseVLLMStats("GPU KV cache usage: 60.0%.", s)
	// Should retain the peak (8000)
	if s.PeakVRAMMB != 8000 {
		t.Errorf("PeakVRAMMB = %v, want 8000 (peak)", s.PeakVRAMMB)
	}
}

func TestParseVLLMStats_NoMatch(t *testing.T) {
	s := &Stats{}
	parseVLLMStats("INFO:     Application startup complete.", s)
	if s.TokensPerSecPrefill != 0 || s.TokensPerSecGeneration != 0 || s.PeakVRAMMB != 0 {
		t.Error("expected all stats to remain zero for non-matching log line")
	}
}

func TestParseVLLMStats_CaseInsensitive(t *testing.T) {
	s := &Stats{}
	// vLLM may vary capitalisation across versions
	parseVLLMStats("AVG PROMPT THROUGHPUT: 88.8 TOKENS/S", s)
	if s.TokensPerSecPrefill != 88.8 {
		t.Errorf("TokensPerSecPrefill = %v, want 88.8 (case-insensitive)", s.TokensPerSecPrefill)
	}
}

// ─── Regex compile sanity ─────────────────────────────────────────────────────

func TestVLLMRegexPatterns(t *testing.T) {
	// Ensure all three package-level regexes compile (catches typos in init)
	patterns := []string{
		vllmPromptRe.String(),
		vllmGenRe.String(),
		vllmKVRe.String(),
	}
	for _, p := range patterns {
		if _, err := regexp.Compile(p); err != nil {
			t.Errorf("regex pattern failed to compile: %q: %v", p, err)
		}
	}
}
