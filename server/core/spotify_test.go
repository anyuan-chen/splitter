package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/joho/godotenv"

	"separate/server/models"
)

func init() {
	// Try to load .env from root (../../.env relative to server/core)
	_ = godotenv.Load("../../.env")
}

// Unit Tests

func TestFetchPlaylistPageParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := playlistResponse{
			Name:        "Test Playlist",
			Description: "A test playlist",
		}
		response.Tracks.Total = 1
		response.Tracks.Items = []struct {
			Track trackObject `json:"track"`
		}{
			{},
		}
		response.Tracks.Items[0].Track.ID = "test123"
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

	_, resp, err := fetchPlaylistPage("test", "token", server.URL)
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

	config := models.SpotifyConfig{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		PlaylistID:   playlistID,
	}

	// Used GetAccessToken to get token first
	token, err := GetAccessToken(config)
	if err != nil {
		t.Fatalf("Failed to get token: %v", err)
	}

	metadata, err := GetPlaylistMetadataWithToken(config.PlaylistID, token)
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
