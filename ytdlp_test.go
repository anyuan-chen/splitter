package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Unit Tests - Test argument building logic
func TestBuildYtDlpArgs(t *testing.T) {
	tests := []struct {
		name     string
		config   DownloadConfig
		expected []string
	}{
		{
			name: "basic config without output dir",
			config: DownloadConfig{
				URL: "https://youtube.com/watch?v=test123",
			},
			expected: []string{"-x", "--audio-format", "mp3", "https://youtube.com/watch?v=test123"},
		},
		{
			name: "config with output dir",
			config: DownloadConfig{
				URL:       "https://youtube.com/watch?v=test456",
				OutputDir: "./downloads",
			},
			expected: []string{"-x", "--audio-format", "mp3", "-o", "./downloads/%(title)s.%(ext)s", "https://youtube.com/watch?v=test456"},
		},
		{
			name: "config with absolute output path",
			config: DownloadConfig{
				URL:       "https://youtube.com/watch?v=test789",
				OutputDir: "/tmp/music",
			},
			expected: []string{"-x", "--audio-format", "mp3", "-o", "/tmp/music/%(title)s.%(ext)s", "https://youtube.com/watch?v=test789"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildYtDlpArgs(tt.config)

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d args, got %d\nExpected: %v\nGot: %v", len(tt.expected), len(result), tt.expected, result)
			}

			for i, arg := range result {
				if arg != tt.expected[i] {
					t.Errorf("arg[%d]: expected %q, got %q", i, tt.expected[i], arg)
				}
			}
		})
	}
}

// Integration Tests - Actually call yt-dlp
// These tests require yt-dlp to be installed and an internet connection
// Run with: go test -tags=integration

func TestDownloadMP3Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary directory for downloads
	tempDir, err := os.MkdirTemp("", "ytdlp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Use a very short public domain video for testing
	// This is a short test video from YouTube (a few seconds)
	testURL := "https://www.youtube.com/watch?v=jNQXAC9IVRw" // "Me at the zoo" - first YouTube video, 19 seconds

	config := DownloadConfig{
		URL:       testURL,
		OutputDir: tempDir,
	}

	err = DownloadMP3(config)
	if err != nil {
		t.Fatalf("DownloadMP3 failed: %v", err)
	}

	// Check that at least one .mp3 file was created
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read temp dir: %v", err)
	}

	if len(entries) == 0 {
		t.Fatal("No files were downloaded")
	}

	foundMP3 := false
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".mp3" {
			foundMP3 = true

			// Check that file size > 0
			info, err := entry.Info()
			if err != nil {
				t.Errorf("Failed to get file info for %s: %v", entry.Name(), err)
				continue
			}

			if info.Size() == 0 {
				t.Errorf("Downloaded file %s has size 0", entry.Name())
			} else {
				t.Logf("Successfully downloaded %s (%d bytes)", entry.Name(), info.Size())
			}
		}
	}

	if !foundMP3 {
		t.Error("No .mp3 file was found in the output directory")
	}
}

func TestDownloadMP3IntegrationNoOutputDir(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create and change to a temporary directory
	tempDir, err := os.MkdirTemp("", "ytdlp-test-nodir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Save current directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current dir: %v", err)
	}
	defer os.Chdir(originalDir)

	// Change to temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	config := DownloadConfig{
		URL: "https://www.youtube.com/watch?v=jNQXAC9IVRw",
	}

	err = DownloadMP3(config)
	if err != nil {
		t.Fatalf("DownloadMP3 failed: %v", err)
	}

	// Check that at least one .mp3 file was created in current directory
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("Failed to read current dir: %v", err)
	}

	foundMP3 := false
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".mp3" {
			foundMP3 = true
			info, err := entry.Info()
			if err != nil {
				t.Errorf("Failed to get file info: %v", err)
				continue
			}
			if info.Size() > 0 {
				t.Logf("Successfully downloaded %s (%d bytes) to current directory", entry.Name(), info.Size())
			}
		}
	}

	if !foundMP3 {
		t.Error("No .mp3 file was found in the current directory")
	}
}
