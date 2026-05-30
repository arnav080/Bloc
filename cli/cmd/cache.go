package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bloc-org/bloc/internal/config"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the local cache directory",
	Long:  `View cache status or prune the entire local cache directory (~/.cache/bloc).`,
	RunE:  runCacheStatus,
}

var cacheStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show total cache size and breakdown by models, runtimes, and temp files",
	RunE:  runCacheStatus,
}

var cachePruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Prune/wipe the entire cache directory",
	RunE:  runCachePrune,
}

func init() {
	cacheCmd.AddCommand(cacheStatusCmd)
	cacheCmd.AddCommand(cachePruneCmd)
	rootCmd.AddCommand(cacheCmd)
}

func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size, err
}

func runCacheStatus(cmd *cobra.Command, args []string) error {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}

	modelsSize, _ := getDirSize(filepath.Join(cacheDir, "models"))
	reposSize, _ := getDirSize(filepath.Join(cacheDir, "repos"))
	runtimesSize, _ := getDirSize(filepath.Join(cacheDir, "runtimes"))
	downloadsSize, _ := getDirSize(filepath.Join(cacheDir, "downloads"))

	totalSize := modelsSize + reposSize + runtimesSize + downloadsSize

	fmt.Printf("\nCache Directory: %s\n", cacheDir)
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("%-30s %10.1f MB\n", "Models (GGUF)", float64(modelsSize)/1e6)
	fmt.Printf("%-30s %10.1f MB\n", "Models (HuggingFace)", float64(reposSize)/1e6)
	fmt.Printf("%-30s %10.1f MB\n", "Runtimes (Python venvs)", float64(runtimesSize)/1e6)
	fmt.Printf("%-30s %10.1f MB\n", "Temporary / Downloads", float64(downloadsSize)/1e6)
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("%-30s %10.1f GB\n\n", "Total Cache Size", float64(totalSize)/1e9)

	return nil
}

func runCachePrune(cmd *cobra.Command, args []string) error {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}

	totalSize, _ := getDirSize(cacheDir)
	fmt.Printf("Wiping the entire cache directory will delete %.1f GB of data.\n", float64(totalSize)/1e9)
	fmt.Println("\033[33m⚠ WARNING: This will permanently delete all cached model files, Python runtimes, and local data.\033[0m")
	if !confirm("Are you absolutely sure you want to proceed? [y/N]: ") {
		fmt.Println("Cancelled.")
		return nil
	}

	fmt.Print("Pruning cache...")
	if err := os.RemoveAll(cacheDir); err != nil {
		fmt.Printf("\n\033[31m✗\033[0m  Failed to prune cache: %v\n", err)
		return err
	}
	
	// Recreate directories with tight permissions
	_ = os.MkdirAll(filepath.Join(cacheDir, "models"), 0700)
	_ = os.MkdirAll(filepath.Join(cacheDir, "downloads"), 0700)
	_ = os.MkdirAll(filepath.Join(cacheDir, "repos"), 0700)

	fmt.Println("\r✓ Cache pruned successfully.")
	return nil
}
