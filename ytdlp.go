package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
