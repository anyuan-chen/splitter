package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// buildYtDlpArgsWithPath builds yt-dlp arguments with a specific output path
func buildYtDlpArgsWithPath(url, outputPath string) []string {
	return []string{"-x", "--audio-format", "mp3", "-o", outputPath, url}
}

// YouTubeSearchResult represents a YouTube search result
type YouTubeSearchResult struct {
	VideoID string
	Title   string
	URL     string
}

// SearchYouTube searches YouTube for a track and returns the top result
func SearchYouTube(track TrackMetadata) (*YouTubeSearchResult, error) {
	// Build search query from track metadata
	query := fmt.Sprintf("%s %s", strings.Join(track.Artists, " "), track.Name)
	searchQuery := fmt.Sprintf("ytsearch1:%s", query)

	// Use yt-dlp to search and get video ID and title
	cmd := exec.Command("yt-dlp", "--get-id", "--get-title", searchQuery)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("youtube search failed: %w\nOutput: %s", err, string(output))
	}

	// Parse output: filter out warning lines, then get title and video ID
	allLines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var contentLines []string
	for _, line := range allLines {
		// Skip warning and info lines
		if !strings.HasPrefix(line, "WARNING:") && !strings.HasPrefix(line, "[") && line != "" {
			contentLines = append(contentLines, line)
		}
	}

	if len(contentLines) < 2 {
		return nil, fmt.Errorf("unexpected yt-dlp output format: %s", string(output))
	}

	title := contentLines[0]
	videoID := contentLines[1]
	url := fmt.Sprintf("https://www.youtube.com/watch?v=%s", videoID)

	return &YouTubeSearchResult{
		VideoID: videoID,
		Title:   title,
		URL:     url,
	}, nil
}

// DownloadTrackFromSpotify searches YouTube for a Spotify track and downloads it
// Files are saved to songs/[spotify_track_id]/base.mp3
func DownloadTrackFromSpotify(track TrackMetadata) error {
	// Search YouTube for the track
	result, err := SearchYouTube(track)
	if err != nil {
		return fmt.Errorf("failed to search YouTube: %w", err)
	}

	// Create directory structure: songs/[track_id]
	trackDir := filepath.Join("songs", track.ID)
	if err := os.MkdirAll(trackDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Download using helper
	outputPath := filepath.Join(trackDir, "base.mp3")
	args := buildYtDlpArgsWithPath(result.URL, outputPath)
	cmd := exec.Command("yt-dlp", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("yt-dlp download failed: %w\nOutput: %s", err, string(output))
	}

	fmt.Printf("Downloaded: %s by %s -> %s\n", track.Name, strings.Join(track.Artists, ", "), outputPath)
	return nil
}

// DownloadTrackFromSpotifyWithProgress downloads and reports progress
// Each worker calls this with its own track, so track context is preserved.
// Each yt-dlp process has its own stderr pipe, so there's no mixing between workers.
func DownloadTrackFromSpotifyWithProgress(track TrackMetadata, progressChan chan<- ProgressEvent) error {
	// Search YouTube for the track
	result, err := SearchYouTube(track)
	if err != nil {
		return fmt.Errorf("failed to search YouTube: %w", err)
	}

	// Create directory structure
	trackDir := filepath.Join("songs", track.ID)
	if err := os.MkdirAll(trackDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Build command (each worker spawns its own yt-dlp process)
	outputPath := filepath.Join(trackDir, "base.mp3")
	args := buildYtDlpArgsWithPath(result.URL, outputPath)
	args = append(args, "--progress") // Force progress output even when piped
	args = append(args, "--newline")  // Force newline after each progress update
	cmd := exec.Command("yt-dlp", args...)

	// Get stdout pipe (progress goes to stdout with --progress flag)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start yt-dlp: %w", err)
	}

	// Parse progress from stdout in a separate goroutine
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()

			// Parse download progress lines
			if strings.Contains(line, "[download]") && strings.Contains(line, "%") {
				progress := parseProgress(line)
				if progress >= 0 {
					// Send event with this track's ID
					progressChan <- ProgressEvent{
						TrackID:  track.ID, // From function parameter, not from stdout
						Status:   "downloading",
						Progress: progress,
					}
				}
			}
		}
	}()

	// Wait for command to finish
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("yt-dlp download failed: %w", err)
	}

	fmt.Printf("Downloaded: %s by %s -> %s\n", track.Name, strings.Join(track.Artists, ", "), outputPath)
	return nil
}

// parseProgress extracts percentage from yt-dlp output line
func parseProgress(line string) float64 {
	// Example: "[download]   42.8% of ~5.23MiB at  1.15MiB/s ETA 00:02"
	parts := strings.Fields(line)
	for i, part := range parts {
		if strings.HasSuffix(part, "%") && i > 0 {
			percentStr := strings.TrimSuffix(part, "%")
			if percent, err := strconv.ParseFloat(percentStr, 64); err == nil {
				return percent
			}
		}
	}
	return -1
}
