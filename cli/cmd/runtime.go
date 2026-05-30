package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bloc-org/bloc/internal/config"
	"github.com/spf13/cobra"
)

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Manage local model execution runtimes",
	Long:  `View status, list versioned virtual environments, or prune unused runtimes (llama.cpp, native vLLM, Docker).`,
	RunE:  runRuntimeStatus,
}

var runtimeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Checks all runtimes and prints version and path info",
	RunE:  runRuntimeStatus,
}

var runtimeListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all installed vLLM versioned virtual environments with disk usage",
	RunE:  runRuntimeList,
}

var runtimePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Interactive: remove unused versioned virtual environments",
	RunE:  runRuntimePrune,
}

func init() {
	runtimeCmd.AddCommand(runtimeStatusCmd)
	runtimeCmd.AddCommand(runtimeListCmd)
	runtimeCmd.AddCommand(runtimePruneCmd)
	rootCmd.AddCommand(runtimeCmd)
}

type InstalledVenv struct {
	Version       string    `json:"version"`
	PythonVersion string    `json:"python_version"`
	InstalledAt   time.Time `json:"installed_at"`
	Path          string
	SizeBytes     int64
}

func getInstalledVenvs(cacheDir string) ([]InstalledVenv, error) {
	vllmDir := filepath.Join(cacheDir, "runtimes", "vllm")
	dirs, err := os.ReadDir(vllmDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var venvs []InstalledVenv
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		verName := d.Name()
		metaPath := filepath.Join(vllmDir, verName, "installed.json")
		venvPath := filepath.Join(vllmDir, verName, "venv")

		// Read installed.json
		var meta InstalledVenv
		data, err := os.ReadFile(metaPath)
		if err == nil {
			_ = json.Unmarshal(data, &meta)
		} else {
			meta.Version = verName
		}

		meta.Path = filepath.Join(vllmDir, verName)
		meta.SizeBytes, _ = getDirSize(venvPath)
		
		// If size is 0, venv is likely incomplete/broken
		if meta.SizeBytes > 0 {
			venvs = append(venvs, meta)
		}
	}
	return venvs, nil
}

func runRuntimeStatus(cmd *cobra.Command, args []string) error {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}

	fmt.Println("\nRuntime Status:")

	// PERF-NEW-D: All subprocess checks use a shared 5-second timeout context.
	// A stalled llama-server binary or wedged Docker socket previously froze
	// the CLI indefinitely. 5 seconds is generous for a local status check.
	ctx5s, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. llama.cpp
	llamaPath, llamaErr := exec.LookPath("llama-server")
	if llamaErr == nil {
		llamaVer := "installed"
		// Try to run --version or parse help
		out, _ := exec.CommandContext(ctx5s, llamaPath, "--version").Output()
		if len(out) > 0 {
			llamaVer = strings.TrimSpace(string(out))
		} else {
			outHelp, _ := exec.CommandContext(ctx5s, llamaPath, "--help").CombinedOutput()
			lines := strings.Split(string(outHelp), "\n")
			if len(lines) > 0 {
				firstLine := strings.TrimSpace(lines[0])
				if strings.Contains(strings.ToLower(firstLine), "version") || strings.Contains(strings.ToLower(firstLine), "build") {
					llamaVer = firstLine
				}
			}
		}
		// Truncate to keep clean
		if len(llamaVer) > 30 {
			llamaVer = llamaVer[:27] + "..."
		}
		fmt.Printf("  \033[32m✓\033[0m  llama.cpp   %-25s %s\n", llamaVer, llamaPath)
	} else {
		fmt.Printf("  \033[31m✗\033[0m  llama.cpp   not found in PATH\n")
	}

	// 2. Native vLLM (venvs)
	venvs, _ := getInstalledVenvs(cacheDir)
	if len(venvs) > 0 {
		for _, v := range venvs {
			pyPath := filepath.Join(v.Path, "venv", "bin", "python3")
			fmt.Printf("  \033[32m✓\033[0m  vLLM        %-25s %s\n", fmt.Sprintf("%s (venv)", v.Version), pyPath)
		}
	} else {
		fmt.Printf("  \033[31m✗\033[0m  vLLM        no versioned venvs installed\n")
	}

	// 3. Docker
	dockerPath, dockerErr := exec.LookPath("docker")
	if dockerErr == nil {
		infoOut, err := exec.CommandContext(ctx5s, dockerPath, "info", "--format", "{{.ServerVersion}}").Output()
		if err == nil {
			dockerVer := strings.TrimSpace(string(infoOut))
			fmt.Printf("  \033[32m✓\033[0m  Docker      %-25s %s\n", dockerVer, dockerPath)

			// Check NVIDIA Container Toolkit: reuse the formatted output from the
			// first docker info call to avoid a second blocking subprocess.
			// PERF-NEW-D: The old code did a second unformatted docker info + a
			// potential docker run smoke test with no timeout at all.
			hasToolkit := false
			// Fetch full docker info once more for the nvidia check (formatted output above is too brief)
			fullInfoOut, fullInfoErr := exec.CommandContext(ctx5s, dockerPath, "info").CombinedOutput()
			if fullInfoErr == nil {
				outStr := strings.ToLower(string(fullInfoOut))
				if strings.Contains(outStr, "nvidia") || strings.Contains(outStr, "gpus") {
					hasToolkit = true
				}
			}
			// Smoke test: use a short dedicated context (15s) to avoid blocking indefinitely.
			// PERF-NEW-D: Previously had no timeout at all — could pull an image and block for minutes.
			if !hasToolkit {
				smokeCtx, smokeCancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer smokeCancel()
				smokeCmd := exec.CommandContext(smokeCtx, dockerPath, "run", "--rm", "--gpus", "all", "nvidia/cuda:12.0-base-ubuntu20.04", "nvidia-smi")
				if smokeCmd.Run() == nil {
					hasToolkit = true
				}
			}

			if hasToolkit {
				fmt.Println("       nvidia-container-toolkit: \033[32m✓\033[0m")
			} else {
				fmt.Println("       nvidia-container-toolkit: \033[33m✗ (GPU passthrough may be unavailable)\033[0m")
			}
		} else {
			fmt.Printf("  \033[33m⚠\033[0m  Docker      daemon is not running     %s\n", dockerPath)
		}
	} else {
		fmt.Printf("  \033[31m✗\033[0m  Docker      not found in PATH\n")
	}

	fmt.Println()
	return nil
}

func runRuntimeList(cmd *cobra.Command, args []string) error {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}

	venvs, err := getInstalledVenvs(cacheDir)
	if err != nil {
		return err
	}

	if len(venvs) == 0 {
		fmt.Println("No vLLM versioned virtual environments found.")
		return nil
	}

	fmt.Printf("\n%-12s %-16s %-12s %s\n", "VERSION", "PYTHON VERSION", "SIZE", "INSTALLED AT")
	fmt.Println(strings.Repeat("─", 65))
	for _, v := range venvs {
		sizeMB := float64(v.SizeBytes) / 1e6
		instAt := "unknown"
		if !v.InstalledAt.IsZero() {
			instAt = v.InstalledAt.Format("2006-01-02 15:04")
		}
		pyVer := v.PythonVersion
		if pyVer == "" {
			pyVer = "unknown"
		}
		fmt.Printf("%-12s %-16s %10.1f MB  %s\n",
			v.Version,
			pyVer,
			sizeMB,
			instAt,
		)
	}
	fmt.Println()
	return nil
}

func runRuntimePrune(cmd *cobra.Command, args []string) error {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}

	venvs, err := getInstalledVenvs(cacheDir)
	if err != nil {
		return err
	}

	if len(venvs) == 0 {
		fmt.Println("No vLLM versioned virtual environments to prune.")
		return nil
	}

	fmt.Println("\nInstalled vLLM Virtual Environments:")
	for i, v := range venvs {
		sizeMB := float64(v.SizeBytes) / 1e6
		fmt.Printf("  [%d] vLLM %s (size: %.1f MB, python: %s, installed: %s)\n",
			i+1,
			v.Version,
			sizeMB,
			v.PythonVersion,
			v.InstalledAt.Format("2006-01-02"),
		)
	}

	fmt.Println()
	fmt.Print("Enter numbers to delete (e.g. 1,2 or 'all', 'q' to cancel): ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil
	}
	input := strings.TrimSpace(scanner.Text())
	if input == "" || strings.ToLower(input) == "q" {
		fmt.Println("Cancelled.")
		return nil
	}

	var selected []int
	if strings.ToLower(input) == "all" {
		for i := range venvs {
			selected = append(selected, i)
		}
	} else {
		parts := strings.Split(input, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.Contains(part, "-") {
				rangeParts := strings.Split(part, "-")
				if len(rangeParts) == 2 {
					start, _ := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
					end, _ := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
					if start <= end && start > 0 {
						for idx := start; idx <= end; idx++ {
							selected = append(selected, idx-1)
						}
					}
				}
			} else {
				idx, err := strconv.Atoi(part)
				if err == nil {
					selected = append(selected, idx-1)
				}
			}
		}
	}

	uniqueSelected := make(map[int]bool)
	var validSelected []int
	for _, idx := range selected {
		if idx >= 0 && idx < len(venvs) {
			if !uniqueSelected[idx] {
				uniqueSelected[idx] = true
				validSelected = append(validSelected, idx)
			}
		}
	}

	if len(validSelected) == 0 {
		fmt.Println("Invalid selection. No runtimes selected to prune.")
		return nil
	}

	var totalMBToDelete float64
	fmt.Println("\nSelected runtimes to delete:")
	for _, idx := range validSelected {
		v := venvs[idx]
		sizeMB := float64(v.SizeBytes) / 1e6
		totalMBToDelete += sizeMB
		fmt.Printf("  - vLLM %s (%.1f MB)\n", v.Version, sizeMB)
	}

	fmt.Printf("\nThis will free %.1f MB of space.\n", totalMBToDelete)
	if !confirm("Are you sure you want to delete these virtual environments? [y/N]: ") {
		fmt.Println("Cancelled.")
		return nil
	}

	for _, idx := range validSelected {
		v := venvs[idx]
		fmt.Printf("Deleting vLLM %s environment...", v.Version)
		if err := os.RemoveAll(v.Path); err != nil {
			fmt.Printf(" \033[31m✗ failed\033[0m: %v\n", err)
		} else {
			fmt.Println(" \033[32m✓ success\033[0m")
		}
	}

	fmt.Println("✓ Virtual environments pruned.")
	return nil
}
