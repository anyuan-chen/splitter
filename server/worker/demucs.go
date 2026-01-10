package worker

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

	"separate/server/models"
)

const (
	demucsContainerName = "demucs-worker"
	demucsImage         = "xserrat/facebook-demucs:latest"
)

var (
	dockerInitOnce sync.Once
	dockerInitErr  error
)

// ensureDockerContainer ensures the Demucs Docker container is running
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
func ProcessTrackWithDemucs(track models.TrackMetadata, inputPath string, progressChan chan<- models.ProgressEvent) error {
	// Ensure Docker container is running
	if err := ensureDockerContainer(); err != nil {
		return fmt.Errorf("failed to ensure Docker container: %w", err)
	}

	// Convert to paths inside container
	trackID := track.ID
	containerInputPath := fmt.Sprintf("/songs/%s/base.mp3", trackID)
	containerOutputDir := fmt.Sprintf("/songs/%s", trackID)

	// Run demucs command
	args := []string{
		"exec",
		"-e", "PYTHONUNBUFFERED=1",
		demucsContainerName,
		"demucs",
		"--device", "cpu",
		"-v",
		"-o", containerOutputDir,
		containerInputPath,
	}

	cmd := exec.Command("docker", args...)

	// Create pipes
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
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
	processDemucsOutput := func(line, trackID string, currentModel *int, lastProgress *float64, ansiRegex *regexp.Regexp, progressChan chan<- models.ProgressEvent) {
		cleanLine := ansiRegex.ReplaceAllString(line, "")
		cleanLine = strings.TrimSpace(cleanLine)

		if !strings.Contains(cleanLine, "%") {
			return
		}

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

		if modelProgress < *lastProgress-50 {
			*currentModel++
		}
		*lastProgress = modelProgress

		// Calculate progress by averaging all 4 models:
		// - Completed models contribute 100%
		// - Current model contributes its actual progress
		// - Future models contribute 0%
		var totalProgress float64
		for i := 0; i < 4; i++ {
			if i < *currentModel {
				totalProgress += 100.0 // Completed models
			} else if i == *currentModel {
				totalProgress += modelProgress // Current model
			}
			// Future models contribute 0
		}
		overallProgress := totalProgress / 4.0

		if overallProgress > 100 {
			overallProgress = 100
		}

		progressChan <- models.ProgressEvent{
			TrackID:  trackID,
			Type:     "demucs",
			Status:   "processing",
			Progress: overallProgress,
		}
	}

	// Read stderr (tqdm progress)
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			updates := strings.Split(line, "\r")
			for _, update := range updates {
				if update == "" {
					continue
				}
				processDemucsOutput(update, trackID, &currentModel, &lastProgress, ansiRegex, progressChan)
			}
		}
	}()

	// Read stdout (Docker messages) - discard but must read
	wg.Add(1)
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			_ = scanner.Text()
		}
	}()

	cmdErr := cmd.Wait()
	wg.Wait()

	if cmdErr != nil {
		return fmt.Errorf("demucs processing failed: %w", cmdErr)
	}

	fmt.Printf("Demucs processing completed: %s → songs/%s/\n", inputPath, trackID)
	return nil
}
