package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Manage local Docker images pulled by bloc",
	Long:  `List or prune local Docker images that were pulled for vLLM recipes.`,
	RunE:  runImagesList,
}

var imagesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all Docker images pulled by bloc recipes",
	RunE:  runImagesList,
}

var imagesPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove unused Docker images pulled by bloc recipes",
	RunE:  runImagesPrune,
}

func init() {
	imagesCmd.AddCommand(imagesListCmd)
	imagesCmd.AddCommand(imagesPruneCmd)
	rootCmd.AddCommand(imagesCmd)
}

type DockerImage struct {
	Repository string
	Tag        string
	ID         string
	Size       string
	Created    string
}

func getDockerImages() ([]DockerImage, error) {
	dockerPath, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("docker not found in PATH")
	}

	// PERF-NEW-D: Use CommandContext with a 5-second timeout so a wedged
	// Docker daemon (stalled Unix socket) never freezes the CLI indefinitely.
	ctx5s, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verify daemon is running
	if err := exec.CommandContext(ctx5s, dockerPath, "info").Run(); err != nil {
		return nil, fmt.Errorf("Docker daemon is not running")
	}

	// List images with format tab-separated
	// Reuse the same 5-second context — both calls are quick when daemon is healthy.
	out, err := exec.CommandContext(ctx5s, dockerPath, "images", "--format", "{{.Repository}}\t{{.Tag}}\t{{.ID}}\t{{.Size}}\t{{.CreatedAt}}").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run docker images: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var images []DockerImage
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}
		repo := parts[0]
		// Filter for vllm images
		if strings.Contains(repo, "vllm") {
			images = append(images, DockerImage{
				Repository: repo,
				Tag:        parts[1],
				ID:         parts[2],
				Size:       parts[3],
				Created:    parts[4],
			})
		}
	}
	return images, nil
}

func runImagesList(cmd *cobra.Command, args []string) error {
	images, err := getDockerImages()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠  %v\n", err)
		return nil
	}

	if len(images) == 0 {
		fmt.Println("No bloc-related Docker images found (none with 'vllm' in the repository name).")
		return nil
	}

	fmt.Printf("\n%-40s %-12s %-15s %-10s %s\n", "REPOSITORY", "TAG", "IMAGE ID", "SIZE", "CREATED")
	fmt.Println(strings.Repeat("─", 90))
	for _, img := range images {
		fmt.Printf("%-40s %-12s %-15s %-10s %s\n",
			img.Repository,
			img.Tag,
			img.ID,
			img.Size,
			img.Created,
		)
	}
	fmt.Println()
	return nil
}

func runImagesPrune(cmd *cobra.Command, args []string) error {
	images, err := getDockerImages()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠  %v\n", err)
		return nil
	}

	if len(images) == 0 {
		fmt.Println("No cached Docker images to prune.")
		return nil
	}

	fmt.Println("\nCached Docker Images:")
	for i, img := range images {
		fmt.Printf("  [%d] %s:%s (%s, size: %s, created: %s)\n",
			i+1,
			img.Repository,
			img.Tag,
			img.ID,
			img.Size,
			img.Created,
		)
	}

	fmt.Println()
	fmt.Print("Enter numbers to delete (e.g. 1,3 or 'all', 'q' to cancel): ")
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
		for i := range images {
			selected = append(selected, i)
		}
	} else {
		parts := strings.Split(input, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			// support range like 1-3
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
		if idx >= 0 && idx < len(images) {
			if !uniqueSelected[idx] {
				uniqueSelected[idx] = true
				validSelected = append(validSelected, idx)
			}
		}
	}

	if len(validSelected) == 0 {
		fmt.Println("Invalid selection. No images selected to prune.")
		return nil
	}

	fmt.Println("\nSelected images to remove:")
	for _, idx := range validSelected {
		img := images[idx]
		fmt.Printf("  - %s:%s (%s)\n", img.Repository, img.Tag, img.ID)
	}

	fmt.Println()
	if !confirm("Are you sure you want to remove these images? [y/N]: ") {
		fmt.Println("Cancelled.")
		return nil
	}

	dockerPath, _ := exec.LookPath("docker")
	for _, idx := range validSelected {
		img := images[idx]
		fmt.Printf("Removing image %s:%s...", img.Repository, img.Tag)
		// Run rmi
		cmd := exec.Command(dockerPath, "rmi", img.ID)
		if err := cmd.Run(); err != nil {
			fmt.Printf(" \033[31m✗ failed\033[0m: %v\n", err)
		} else {
			fmt.Println(" \033[32m✓ success\033[0m")
		}
	}

	fmt.Println("✓ Docker images pruned.")
	return nil
}
