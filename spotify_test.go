package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// Unit Tests

func TestFetchPlaylistPageParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := spotifyPlaylistResponse{
			Name:        "Test Playlist",
			Description: "A test playlist",
		}
		response.Tracks.Total = 1
		response.Tracks.Items = []struct {
			Track struct {
				Name       string `json:"name"`
				DurationMs int    `json:"duration_ms"`
				ExternalURLs struct {
					Spotify string `json:"spotify"`
				} `json:"external_urls"`
				PreviewURL   string `json:"preview_url"`
				ExternalIDs struct {
					ISRC string `json:"isrc"`
				} `json:"external_ids"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name        string `json:"name"`
					ReleaseDate string `json:"release_date"`
				} `json:"album"`
			} `json:"track"`
		}{
			{},
		}
		response.Tracks.Items[0].Track.Name = "Test Song"
		response.Tracks.Items[0].Track.DurationMs = 180000
		response.Tracks.Items[0].Track.Artists = []struct {
			Name string `json:"name"`
		}{
			{Name: "Test Artist"},
		}
		response.Tracks.Items[0].Track.Album.Name = "Test Album"

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	resp, err := fetchPlaylistPage("test", "token", server.URL)
	if err != nil {
		t.Fatalf("Failed to fetch: %v", err)
	}

	if resp.Name != "Test Playlist" {
		t.Errorf("Expected 'Test Playlist', got '%s'", resp.Name)
	}

	if len(resp.Tracks.Items) != 1 {
		t.Fatalf("Expected 1 track, got %d", len(resp.Tracks.Items))
	}

	if resp.Tracks.Items[0].Track.Name != "Test Song" {
		t.Errorf("Expected 'Test Song', got '%s'", resp.Tracks.Items[0].Track.Name)
	}
}

// Integration Tests

func TestGetPlaylistMetadataIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	playlistID := os.Getenv("TEST_SPOTIFY_PLAYLIST_ID")

	if clientID == "" || clientSecret == "" || playlistID == "" {
		t.Fatal("Requires SPOTIFY_CLIENT_ID, SPOTIFY_CLIENT_SECRET, and TEST_SPOTIFY_PLAYLIST_ID environment variables")
	}

	config := SpotifyConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		PlaylistID:   playlistID,
	}

	metadata, err := GetPlaylistMetadata(config)
	if err != nil {
		t.Fatalf("GetPlaylistMetadata failed: %v", err)
	}

	if metadata.Name == "" {
		t.Error("Playlist name is empty")
	}

	if len(metadata.Tracks) != metadata.TotalTracks {
		t.Errorf("Expected %d tracks, got %d", metadata.TotalTracks, len(metadata.Tracks))
	}

	t.Logf("Playlist: %s (%d tracks)", metadata.Name, metadata.TotalTracks)

	if len(metadata.Tracks) > 0 {
		track := metadata.Tracks[0]
		t.Logf("First track: %s by %v", track.Name, track.Artists)
	}
}
