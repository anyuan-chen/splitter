package main

import (
	"fmt"
	"os/exec"
)

// DownloadConfig holds configuration for downloading audio
type DownloadConfig struct {
	URL       string
	OutputDir string
}

// buildYtDlpArgs builds the command-line arguments for yt-dlp
func buildYtDlpArgs(config DownloadConfig) []string {
	args := []string{"-x", "--audio-format", "mp3"}

	if config.OutputDir != "" {
		args = append(args, "-o", fmt.Sprintf("%s/%%(title)s.%%(ext)s", config.OutputDir))
	}

	args = append(args, config.URL)
	return args
}

// DownloadMP3 downloads audio from a YouTube URL as MP3 using yt-dlp
func DownloadMP3(config DownloadConfig) error {
	args := buildYtDlpArgs(config)
	cmd := exec.Command("yt-dlp", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("yt-dlp failed: %w\nOutput: %s", err, string(output))
	}

	fmt.Printf("Successfully downloaded: %s\n", config.URL)
	return nil
}
