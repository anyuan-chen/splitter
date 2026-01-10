package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"separate/server/core"
	"separate/server/db"
	"separate/server/models"
)

type Handler struct {
	DB            *db.DB
	Progress      *core.ProgressBroadcaster
	JobQueue      chan *models.DownloadJob
	SpotifyConfig models.SpotifyConfig
}

func NewHandler(db *db.DB, progress *core.ProgressBroadcaster, jobQueue chan *models.DownloadJob, config models.SpotifyConfig) *Handler {
	return &Handler{
		DB:            db,
		Progress:      progress,
		JobQueue:      jobQueue,
		SpotifyConfig: config,
	}
}

// SetupPlaylistHandler creates directories for all tracks in a Spotify playlist
func (h *Handler) SetupPlaylistHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.SetupPlaylistRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.PlaylistID == "" {
		http.Error(w, "playlist_id is required", http.StatusBadRequest)
		return
	}

	// Update config with playlist ID (volatile, but needed for token fetching if strictly bound)
	// Actually, GetPlaylistMetadataWithToken just needs a token.
	// We'll get a token using client credentials.
	token, err := core.GetAccessToken(h.SpotifyConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get Spotify access token: %v", err), http.StatusInternalServerError)
		return
	}

	// Fetch playlist metadata using cached token
	metadata, err := core.GetPlaylistMetadataWithToken(req.PlaylistID, token)
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

	// Save to DB
	err = h.DB.SavePlaylistTracks(req.PlaylistID, metadata.Tracks)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Enqueue download jobs for each track
	for _, track := range metadata.Tracks {
		h.JobQueue <- &models.DownloadJob{Track: track}
	}

	// Return response immediately
	response := models.SetupPlaylistResponse{
		PlaylistName: metadata.Name,
		TotalTracks:  metadata.TotalTracks,
		TrackIDs:     trackIDs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	log.Printf("Setup playlist: %s (%d tracks), downloads queued", metadata.Name, metadata.TotalTracks)
}

// TracksHandler returns current state snapshot of all tracks
func (h *Handler) TracksHandler(w http.ResponseWriter, r *http.Request) {
	tracks, err := h.DB.GetAllTracks()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tracks)
}

// GetTrackHandler returns metadata for a single track
func (h *Handler) GetTrackHandler(w http.ResponseWriter, r *http.Request) {
	// Extract track ID from URL path (assuming /tracks/{id})
	// Since we are using standard net/http, we might need to parse it manually if using StripPrefix or similar,
	// but here we will assume it's passed as a query param or parse it from path if using main router.
	// Actually, let's assume the router in main.go handles the path pattern or we parse it here.
	// For simplicity with standard mux, we'll strip prefix in main.go or simple parsing:
	// Path: /tracks/<id>

	id := filepath.Base(r.URL.Path)
	if id == "" || id == "tracks" {
		http.Error(w, "Track ID required", http.StatusBadRequest)
		return
	}

	track, err := h.DB.GetTrack(id)
	if err != nil {
		http.Error(w, "Track not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(track)
}

// ProgressStreamHandler streams progress updates via SSE
func (h *Handler) ProgressStreamHandler(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client channel
	clientChan := h.Progress.RegisterClient()

	// Cleanup on disconnect
	defer func() {
		h.Progress.UnregisterClient(clientChan)
	}()

	// Stream updates
	for {
		select {
		case event := <-clientChan:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}
