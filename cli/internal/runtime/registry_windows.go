//go:build windows

package runtime

import (
	"fmt"

	"github.com/bloc-org/bloc/internal/recipe"
)

// Resolve returns the correct Runtime implementation for the given recipe.
// On Windows, only llama.cpp (native) is supported.
// vLLM requires Linux with CUDA drivers and is not supported on Windows.
func Resolve(r *recipe.Recipe, runtimeOverride string) (Runtime, error) {
	engineName := r.Engine.Name
	if engineName == "" {
		engineName = "llama.cpp" // zero-value default
	}

	switch engineName {
	case "llama.cpp", "llama-cpp":
		return &LlamaCppRuntime{}, nil

	case "vllm":
		return nil, fmt.Errorf(
			"the vLLM engine is not supported on Windows\n" +
				"  vLLM requires Linux with NVIDIA CUDA drivers.\n" +
				"  To run vLLM on Windows, use WSL2 (Windows Subsystem for Linux):\n" +
				"  https://docs.vllm.ai/en/latest/getting_started/installation.html",
		)

	default:
		return nil, fmt.Errorf("unsupported engine %q — supported engines: llama.cpp, vllm", engineName)
	}
}
