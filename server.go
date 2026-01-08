package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	numWorkers = 8
)

// Server holds the application state
type Server struct {
	spotifyClientID     string
	spotifyClientSecret string
	accessToken         string
	tokenExpiry         time.Time
	tokenMutex          sync.RWMutex
	jobQueue            chan *DownloadJob
}

// SetupPlaylistRequest represents the request to setup a playlist
type SetupPlaylistRequest struct {
	PlaylistID string `json:"playlist_id"`
}

// SetupPlaylistResponse represents the response after setting up directories
type SetupPlaylistResponse struct {
	PlaylistName string   `json:"playlist_name"`
	TotalTracks  int      `json:"total_tracks"`
	TrackIDs     []string `json:"track_ids"`
}

// DownloadJob represents a track download job
type DownloadJob struct {
	Track TrackMetadata
}

// getAccessToken returns a valid access token, fetching a new one if needed
// Uses RWMutex to allow concurrent reads of cached token while ensuring
// only one goroutine fetches a new token when expired.
func (s *Server) getAccessToken() (string, error) {
	// Fast path: check if we have a valid cached token
	// RLock allows multiple goroutines to read simultaneously
	s.tokenMutex.RLock()
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry) {
		token := s.accessToken
		s.tokenMutex.RUnlock()
		return token, nil
	}
	s.tokenMutex.RUnlock()

	// Slow path: need to fetch new token
	s.tokenMutex.Lock()
	defer s.tokenMutex.Unlock()

	// Double-check: prevents TOCTOU race where multiple goroutines see expired token
	// between RLock check and Lock acquisition. Without this, if 100 requests arrive
	// when token is expired, all 100 would wait for Lock, and each would fetch a new
	// token sequentially (wasteful). With double-check, only the first fetches; the
	// rest see the newly-cached token and return immediately.
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry) {
		return s.accessToken, nil
	}

	// Fetch new token from Spotify
	config := SpotifyConfig{
		ClientID:     s.spotifyClientID,
		ClientSecret: s.spotifyClientSecret,
	}

	tokenResp, err := getAccessTokenWithExpiry(config)
	if err != nil {
		return "", err
	}

	s.accessToken = tokenResp.AccessToken
	// Use the actual expiry from Spotify, but subtract 5 minutes to refresh early
	expiryDuration := time.Duration(tokenResp.ExpiresIn) * time.Second
	s.tokenExpiry = time.Now().Add(expiryDuration - 5*time.Minute)

	log.Printf("Fetched new Spotify access token (expires in %d seconds, will refresh at %s)",
		tokenResp.ExpiresIn, s.tokenExpiry.Format(time.RFC3339))
	return s.accessToken, nil
}

// worker processes download jobs from the job queue
// This loop runs forever until the channel is closed
// In Phase 1, we never close the channel, so workers run until server process exits
func (s *Server) worker() {
	for job := range s.jobQueue {
		log.Printf("Downloading track: %s by %s",
			job.Track.Name,
			strings.Join(job.Track.Artists, ", "))

		err := DownloadTrackFromSpotify(job.Track)
		if err != nil {
			log.Printf("Failed to download %s: %v", job.Track.Name, err)
		} else {
			log.Printf("Downloaded: %s → songs/%s/base.mp3",
				job.Track.Name,
				job.Track.ID)
		}
	}
}

// setupPlaylistHandler creates directories for all tracks in a Spotify playlist
func (s *Server) setupPlaylistHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SetupPlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.PlaylistID == "" {
		http.Error(w, "playlist_id is required", http.StatusBadRequest)
		return
	}

	// Get or refresh access token
	token, err := s.getAccessToken()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get Spotify access token: %v", err), http.StatusInternalServerError)
		return
	}

	// Fetch playlist metadata using cached token
	metadata, err := GetPlaylistMetadataWithToken(req.PlaylistID, token)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch playlist: %v", err), http.StatusInternalServerError)
		return
	}

	// Create directory structure for each track
	trackIDs := make([]string, 0, len(metadata.Tracks))
	for _, track := range metadata.Tracks {
		trackDir := filepath.Join("songs", track.ID)
		if err := os.MkdirAll(trackDir, 0755); err != nil {
			http.Error(w, fmt.Sprintf("Failed to create directory: %v", err), http.StatusInternalServerError)
			return
		}
		trackIDs = append(trackIDs, track.ID)
	}

	// Enqueue download jobs for each track
	for _, track := range metadata.Tracks {
		s.jobQueue <- &DownloadJob{Track: track}
	}

	// Return response immediately (don't wait for downloads)
	response := SetupPlaylistResponse{
		PlaylistName: metadata.Name,
		TotalTracks:  metadata.TotalTracks,
		TrackIDs:     trackIDs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("Setup playlist: %s (%d tracks), downloads queued", metadata.Name, metadata.TotalTracks)
}

func main() {
	// Initialize server with Spotify credentials
	server := &Server{
		spotifyClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		spotifyClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		jobQueue:            make(chan *DownloadJob, 1000),
	}

	if server.spotifyClientID == "" || server.spotifyClientSecret == "" {
		log.Fatal("SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET environment variables must be set")
	}

	// Start worker pool BEFORE starting HTTP server
	for i := 0; i < numWorkers; i++ {
		go server.worker()
	}
	log.Printf("Started %d download workers", numWorkers)

	// Register handlers
	http.HandleFunc("/setup-playlist", server.setupPlaylistHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
