package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchYouTubeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	track := TrackMetadata{
		Name:    "The Louvre",
		Artists: []string{"Lorde"},
		Album:   "Melodrama",
	}

	result, err := SearchYouTube(track)
	if err != nil {
		t.Fatalf("SearchYouTube failed: %v", err)
	}

	if result.VideoID == "" {
		t.Error("VideoID is empty")
	}

	if result.Title == "" {
		t.Error("Title is empty")
	}

	if result.URL == "" {
		t.Error("URL is empty")
	}

	expectedURL := fmt.Sprintf("https://www.youtube.com/watch?v=%s", result.VideoID)
	if result.URL != expectedURL {
		t.Errorf("Expected URL %s, got %s", expectedURL, result.URL)
	}

	t.Logf("Found video: %s", result.Title)
	t.Logf("Video ID: %s", result.VideoID)
	t.Logf("URL: %s", result.URL)
}

func TestDownloadTrackFromSpotifyIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	track := TrackMetadata{
		ID:      "5q4BpnMrYEFzLO0dYODj6J",
		Name:    "The Louvre",
		Artists: []string{"Lorde"},
		Album:   "Melodrama",
	}

	err := DownloadTrackFromSpotify(track)
	if err != nil {
		t.Fatalf("DownloadTrackFromSpotify failed: %v", err)
	}

	// Verify file exists
	expectedPath := filepath.Join("songs", track.ID, "base.mp3")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Expected file %s does not exist", expectedPath)
	} else {
		info, _ := os.Stat(expectedPath)
		t.Logf("Downloaded file: %s (%d bytes)", expectedPath, info.Size())

		// Cleanup
		os.RemoveAll("songs")
	}
}
