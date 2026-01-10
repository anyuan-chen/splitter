package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	demucsContainerName = "demucs-worker"
	demucsImage         = "xserrat/facebook-demucs:latest"
)

var (
	dockerInitOnce sync.Once
	dockerInitErr  error
)

// DemucsJob represents a Demucs separation job
type DemucsJob struct {
	Track     TrackMetadata
	InputPath string
}

// ensureDockerContainer ensures the Demucs Docker container is running
// Uses sync.Once to ensure we only initialize once across all workers
func ensureDockerContainer() error {
	dockerInitOnce.Do(func() {
		dockerInitErr = startDockerContainer()
	})
	return dockerInitErr
}

// startDockerContainer starts or reuses the Demucs Docker container
func startDockerContainer() error {
	// Check if container already exists
	checkCmd := exec.Command("docker", "ps", "-a", "--filter", fmt.Sprintf("name=%s", demucsContainerName), "--format", "{{.Names}}")
	output, err := checkCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check for existing container: %w", err)
	}

	containerExists := strings.TrimSpace(string(output)) == demucsContainerName

	if containerExists {
		// Check if it's running
		checkRunning := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", demucsContainerName), "--format", "{{.Names}}")
		output, err := checkRunning.Output()
		if err != nil {
			return fmt.Errorf("failed to check if container is running: %w", err)
		}

		isRunning := strings.TrimSpace(string(output)) == demucsContainerName

		if !isRunning {
			// Start existing container
			startCmd := exec.Command("docker", "start", demucsContainerName)
			if err := startCmd.Run(); err != nil {
				return fmt.Errorf("failed to start existing container: %w", err)
			}
			fmt.Printf("Started existing Demucs container: %s\n", demucsContainerName)
		} else {
			fmt.Printf("Demucs container already running: %s\n", demucsContainerName)
		}
	} else {
		// Pull image if not present
		pullCmd := exec.Command("docker", "pull", demucsImage)
		pullCmd.Stdout = os.Stdout
		pullCmd.Stderr = os.Stderr
		if err := pullCmd.Run(); err != nil {
			return fmt.Errorf("failed to pull Demucs image: %w", err)
		}

		// Get absolute path for volume mount
		absPath, err := filepath.Abs("songs")
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}

		// Create new container that stays running
		// Use --entrypoint to override the image's entrypoint and keep it alive
		createCmd := exec.Command("docker", "run", "-d",
			"--name", demucsContainerName,
			"--entrypoint", "sleep",
			"-v", fmt.Sprintf("%s:/songs", absPath),
			demucsImage,
			"infinity", // Keep container alive forever
		)
		if err := createCmd.Run(); err != nil {
			return fmt.Errorf("failed to create Demucs container: %w", err)
		}
		fmt.Printf("Created new Demucs container: %s\n", demucsContainerName)
	}

	return nil
}

// ProcessTrackWithDemucs separates audio using Demucs and reports progress
func ProcessTrackWithDemucs(track TrackMetadata, inputPath string, progressChan chan<- ProgressEvent) error {
	// Ensure Docker container is running
	if err := ensureDockerContainer(); err != nil {
		return fmt.Errorf("failed to ensure Docker container: %w", err)
	}

	// Convert to paths inside container
	// Input: songs/{track_id}/base.mp3 -> /songs/{track_id}/base.mp3
	// Output: songs/{track_id}/demucs/ -> /songs/{track_id}
	trackID := track.ID
	containerInputPath := fmt.Sprintf("/songs/%s/base.mp3", trackID)
	containerOutputDir := fmt.Sprintf("/songs/%s", trackID)

	// Create output directory on host
	outputDir := filepath.Join("songs", trackID, "demucs")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create demucs output directory: %w", err)
	}

	// Run demucs command in the persistent container using docker exec
	// Use default mdx_extra_q model - best quality available
	// Output as WAV (MP3 requires ffmpeg which isn't in the container)
	// Note: Uses ~6-7GB RAM, may hit memory limits on systems with <8GB
	args := []string{
		"exec",
		"-e", "PYTHONUNBUFFERED=1", // Force unbuffered output
		demucsContainerName,
		"demucs",
		"--device", "cpu",
		"-v", // Verbose mode for more output
		"-o", containerOutputDir,
		containerInputPath,
	}

	cmd := exec.Command("docker", args...)

	// Create pipes for BOTH stdout and stderr (critical!)
	stderr, err := cmd.StderrPipe() // tqdm progress goes here
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe() // Docker exec messages go here
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start demucs: %w", err)
	}

	var wg sync.WaitGroup

	// State for tracking model progress
	currentModel := 0
	lastProgress := 0.0
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)

	// Helper function to process each progress update
	processDemucsOutput := func(line, trackID string, currentModel *int, lastProgress *float64, ansiRegex *regexp.Regexp, progressChan chan<- ProgressEvent) {
		// Strip ANSI escape codes
		cleanLine := ansiRegex.ReplaceAllString(line, "")
		cleanLine = strings.TrimSpace(cleanLine)

		// Look for percentage: "  45%|..."
		if !strings.Contains(cleanLine, "%") {
			return
		}

		// Extract percentage using regex: find digits followed by %
		percentRegex := regexp.MustCompile(`^\s*(\d+)%`)
		matches := percentRegex.FindStringSubmatch(cleanLine)
		if len(matches) < 2 {
			return
		}

		percentStr := matches[1]
		modelProgress, err := strconv.ParseFloat(percentStr, 64)
		if err != nil || modelProgress < 0 || modelProgress > 100 {
			return
		}

		// Detect model transitions (progress dropped from high to low)
		if modelProgress < *lastProgress-50 {
			*currentModel++
		}
		*lastProgress = modelProgress

		// Map to overall 0-100%
		// Model 0 (first): 0-100% → 0-25%
		// Model 1: 0-100% → 25-50%
		// Model 2: 0-100% → 50-75%
		// Model 3: 0-100% → 75-100%
		baseProgress := float64(*currentModel) * 25.0
		overallProgress := baseProgress + (modelProgress / 4.0)

		// Clamp to 0-100
		if overallProgress > 100 {
			overallProgress = 100
		}

		progressChan <- ProgressEvent{
			TrackID:  trackID,
			Type:     "demucs",
			Status:   "processing",
			Progress: overallProgress,
		}
	}

	// Read stderr (tqdm progress) in goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()

			// tqdm uses \r (carriage returns) to overwrite progress on the same line
			// Split the line by \r to get individual progress updates
			updates := strings.Split(line, "\r")
			for _, update := range updates {
				if update == "" {
					continue
				}
				processDemucsOutput(update, trackID, &currentModel, &lastProgress, ansiRegex, progressChan)
			}
		}
	}()

	// Read stdout (Docker messages) in goroutine - MUST read this too!
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			// Just discard stdout (contains informational messages)
			_ = scanner.Text()
		}
	}()

	// Wait for command AND goroutines
	cmdErr := cmd.Wait()
	wg.Wait() // Ensure both readers finish

	if cmdErr != nil {
		return fmt.Errorf("demucs processing failed: %w", cmdErr)
	}

	fmt.Printf("Demucs processing completed: %s → %s\n", inputPath, outputDir)
	return nil
}

// parseDemucsProgress extracts progress from Demucs output
// Demucs output typically looks like: "Separated sources for 1 song in 45.2s"
// or progress bars like: "Processing: 23%"
func parseDemucsProgress(line, trackID string, progressChan chan<- ProgressEvent) {
	// Check for percentage patterns
	// Demucs may output: "23%", "Processing: 45%", etc.
	if strings.Contains(line, "%") {
		parts := strings.Fields(line)
		for _, part := range parts {
			if strings.HasSuffix(part, "%") {
				percentStr := strings.TrimSuffix(part, "%")
				if progress, err := strconv.ParseFloat(percentStr, 64); err == nil && progress >= 0 && progress <= 100 {
					progressChan <- ProgressEvent{
						TrackID:  trackID,
						Type:     "demucs",
						Status:   "processing",
						Progress: progress,
					}
					return
				}
			}
		}
	}
}
