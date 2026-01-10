package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	numWorkers       = 8
	numDemucsWorkers = 1 // Demucs is slow, process one at a time
)

// Server holds the application state
type Server struct {
	spotifyClientID     string
	spotifyClientSecret string
	accessToken         string
	tokenExpiry         time.Time
	tokenMutex          sync.RWMutex
	jobQueue            chan *DownloadJob
	demucsQueue         chan *DemucsJob
	db                  *sql.DB
	progress            *ProgressBroadcaster
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

// ProgressEvent represents a download/processing progress update (minimal)
type ProgressEvent struct {
	TrackID  string  `json:"track_id"`
	Type     string  `json:"type"`     // "download" or "demucs"
	Status   string  `json:"status"`   // "pending", "downloading"/"processing", "completed", "failed"
	Progress float64 `json:"progress"` // 0.0 to 100.0
	Error    string  `json:"error,omitempty"`
}

// TrackState represents full track metadata for /tracks endpoint
type TrackState struct {
	TrackID       string  `json:"track_id"`
	Name          string  `json:"name"`
	Artists       string  `json:"artists"`
	DownloadStatus string  `json:"download_status"`
	DownloadProgress float64 `json:"download_progress"`
	DownloadError string  `json:"download_error,omitempty"`
	DemucsStatus  string  `json:"demucs_status"`
	DemucsProgress float64 `json:"demucs_progress"`
	DemucsError   string  `json:"demucs_error,omitempty"`
}

// ProgressBroadcaster manages SSE client subscriptions
type ProgressBroadcaster struct {
	events         chan ProgressEvent
	newClients     chan chan ProgressEvent
	closingClients chan chan ProgressEvent
	clients        map[chan ProgressEvent]bool
}

// NewProgressBroadcaster creates and starts a new progress broadcaster
func NewProgressBroadcaster() *ProgressBroadcaster {
	b := &ProgressBroadcaster{
		events:         make(chan ProgressEvent, 100), // Buffered for bursts of progress updates
		newClients:     make(chan chan ProgressEvent),
		closingClients: make(chan chan ProgressEvent),
		clients:        make(map[chan ProgressEvent]bool),
	}
	go b.run()
	return b
}

func (b *ProgressBroadcaster) run() {
	for {
		select {
		case client := <-b.newClients:
			b.clients[client] = true
		case client := <-b.closingClients:
			delete(b.clients, client)
			close(client)
		case event := <-b.events:
			// Broadcast to all clients
			for client := range b.clients {
				select {
				case client <- event:
				default:
					// Client is slow/blocked, skip
				}
			}
		}
	}
}

// SendEvent broadcasts a progress event to all connected clients
func (b *ProgressBroadcaster) SendEvent(event ProgressEvent) {
	b.events <- event
}

// initDB initializes the SQLite database and creates tables
func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./queue.db")
	if err != nil {
		return nil, err
	}

	// Create schema
	schema := `
	CREATE TABLE IF NOT EXISTS tracks (
		track_id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		artists TEXT NOT NULL,
		download_status TEXT NOT NULL,
		error_message TEXT,
		demucs_status TEXT DEFAULT 'pending',
		demucs_error_message TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_download_status ON tracks(download_status);
	CREATE INDEX IF NOT EXISTS idx_demucs_status ON tracks(demucs_status);

	CREATE TABLE IF NOT EXISTS playlist_tracks (
		playlist_id TEXT NOT NULL,
		track_id TEXT NOT NULL,
		PRIMARY KEY (playlist_id, track_id),
		FOREIGN KEY (track_id) REFERENCES tracks(track_id)
	);
	CREATE INDEX IF NOT EXISTS idx_playlist_id ON playlist_tracks(playlist_id);
	`

	_, err = db.Exec(schema)
	if err != nil {
		return nil, err
	}

	// Migration: add demucs columns if they don't exist
	migrations := []string{
		`ALTER TABLE tracks ADD COLUMN demucs_status TEXT DEFAULT 'pending'`,
		`ALTER TABLE tracks ADD COLUMN demucs_error_message TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_demucs_status ON tracks(demucs_status)`,
	}

	for _, migration := range migrations {
		// Ignore errors if column already exists
		db.Exec(migration)
	}

	return db, nil
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

// verifyDownloadStatus verifies download status against actual files on disk
func (s *Server) verifyDownloadStatus() error {
	rows, err := s.db.Query("SELECT track_id, download_status FROM tracks")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var trackID, status string
		rows.Scan(&trackID, &status)

		// Check if file exists
		filePath := filepath.Join("songs", trackID, "base.mp3")
		info, err := os.Stat(filePath)

		if err == nil && info.Size() > 0 {
			// File exists and has content -> mark completed
			if status != "completed" {
				s.db.Exec("UPDATE tracks SET download_status = 'completed', updated_at = CURRENT_TIMESTAMP WHERE track_id = ?", trackID)
				log.Printf("Verified completed: %s", trackID)
			}
		} else if status == "in_progress" {
			// Was downloading when server crashed -> reset to pending
			s.db.Exec("UPDATE tracks SET download_status = 'pending', updated_at = CURRENT_TIMESTAMP WHERE track_id = ?", trackID)
			log.Printf("Reset interrupted download: %s", trackID)
		}
	}

	return rows.Err()
}

// loadPendingJobs loads all pending jobs from database and enqueues them
func (s *Server) loadPendingJobs() error {
	rows, err := s.db.Query("SELECT track_id FROM tracks WHERE download_status = 'pending'")
	if err != nil {
		return err
	}
	defer rows.Close()

	var trackIDs []string
	for rows.Next() {
		var trackID string
		rows.Scan(&trackID)
		trackIDs = append(trackIDs, trackID)
	}

	if len(trackIDs) == 0 {
		return nil
	}

	log.Printf("Loading %d pending jobs from database", len(trackIDs))

	// Fetch access token once for all tracks
	token, err := s.getAccessToken()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// Enqueue all pending tracks
	for _, trackID := range trackIDs {
		track, err := GetTrackMetadata(trackID, token)
		if err != nil {
			log.Printf("Failed to fetch metadata for %s: %v", trackID, err)
			continue
		}
		s.jobQueue <- &DownloadJob{Track: *track}
	}

	return nil
}

// loadPendingDemucsJobs loads tracks that are downloaded but need Demucs processing
func (s *Server) loadPendingDemucsJobs() error {
	rows, err := s.db.Query(`
		SELECT track_id, name, artists
		FROM tracks
		WHERE download_status = 'completed' AND demucs_status = 'pending'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var trackID, name, artists string
		if err := rows.Scan(&trackID, &name, &artists); err != nil {
			log.Printf("Failed to scan row: %v", err)
			continue
		}

		// Parse artists string back to slice
		artistsList := strings.Split(artists, ", ")

		track := TrackMetadata{
			ID:      trackID,
			Name:    name,
			Artists: artistsList,
		}

		inputPath := filepath.Join("songs", trackID, "base.mp3")
		s.demucsQueue <- &DemucsJob{
			Track:     track,
			InputPath: inputPath,
		}
		count++
	}

	if count > 0 {
		log.Printf("Queued %d tracks for Demucs processing", count)
	}

	return nil
}

// worker processes download jobs from the job queue
// This loop runs forever until the channel is closed
// In Phase 1, we never close the channel, so workers run until server process exits
func (s *Server) worker() {
	for job := range s.jobQueue {
		artistsStr := strings.Join(job.Track.Artists, ", ")
		log.Printf("Downloading track: %s by %s", job.Track.Name, artistsStr)

		// Send pending event
		s.progress.SendEvent(ProgressEvent{
			TrackID:  job.Track.ID,
			Type:     "download",
			Status:   "pending",
			Progress: 0,
		})

		// Mark as in_progress in database
		s.db.Exec("UPDATE tracks SET download_status = 'in_progress', updated_at = CURRENT_TIMESTAMP WHERE track_id = ?", job.Track.ID)

		// Download with progress reporting
		err := DownloadTrackFromSpotifyWithProgress(job.Track, s.progress.events)

		if err != nil {
			log.Printf("Failed to download %s: %v", job.Track.Name, err)
			s.db.Exec(`
				UPDATE tracks
				SET download_status = 'failed', error_message = ?, updated_at = CURRENT_TIMESTAMP
				WHERE track_id = ?
			`, err.Error(), job.Track.ID)

			// Send failed event
			s.progress.SendEvent(ProgressEvent{
				TrackID:  job.Track.ID,
				Type:     "download",
				Status:   "failed",
				Progress: 0,
				Error:    err.Error(),
			})
		} else {
			outputPath := filepath.Join("songs", job.Track.ID, "base.mp3")
			log.Printf("Downloaded: %s → %s", job.Track.Name, outputPath)
			s.db.Exec(`
				UPDATE tracks
				SET download_status = 'completed', error_message = NULL, updated_at = CURRENT_TIMESTAMP
				WHERE track_id = ?
			`, job.Track.ID)

			// Send completed event
			s.progress.SendEvent(ProgressEvent{
				TrackID:  job.Track.ID,
				Type:     "download",
				Status:   "completed",
				Progress: 100,
			})

			// Automatically queue Demucs processing
			s.demucsQueue <- &DemucsJob{
				Track:     job.Track,
				InputPath: outputPath,
			}
		}
	}
}

// demucsWorker processes Demucs separation jobs from the demucs queue
func (s *Server) demucsWorker() {
	for job := range s.demucsQueue {
		artistsStr := strings.Join(job.Track.Artists, ", ")
		log.Printf("Processing Demucs: %s by %s", job.Track.Name, artistsStr)

		// Send pending event
		s.progress.SendEvent(ProgressEvent{
			TrackID:  job.Track.ID,
			Type:     "demucs",
			Status:   "pending",
			Progress: 0,
		})

		// Mark as in_progress in database
		s.db.Exec("UPDATE tracks SET demucs_status = 'in_progress', updated_at = CURRENT_TIMESTAMP WHERE track_id = ?", job.Track.ID)

		// Process with Demucs and progress reporting
		err := ProcessTrackWithDemucs(job.Track, job.InputPath, s.progress.events)

		if err != nil {
			log.Printf("Failed to process Demucs for %s: %v", job.Track.Name, err)
			s.db.Exec(`
				UPDATE tracks
				SET demucs_status = 'failed', demucs_error_message = ?, updated_at = CURRENT_TIMESTAMP
				WHERE track_id = ?
			`, err.Error(), job.Track.ID)

			// Send failed event
			s.progress.SendEvent(ProgressEvent{
				TrackID:  job.Track.ID,
				Type:     "demucs",
				Status:   "failed",
				Progress: 0,
				Error:    err.Error(),
			})
		} else {
			log.Printf("Demucs completed: %s → songs/%s/demucs/", job.Track.Name, job.Track.ID)
			s.db.Exec(`
				UPDATE tracks
				SET demucs_status = 'completed', demucs_error_message = NULL, updated_at = CURRENT_TIMESTAMP
				WHERE track_id = ?
			`, job.Track.ID)

			// Send completed event
			s.progress.SendEvent(ProgressEvent{
				TrackID:  job.Track.ID,
				Type:     "demucs",
				Status:   "completed",
				Progress: 100,
			})
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

	// Persist jobs to database
	tx, err := s.db.Begin()
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Bulk insert tracks
	if len(metadata.Tracks) > 0 {
		// Build bulk insert query for tracks
		trackValuesClause := strings.Repeat("(?, ?, ?, 'pending'),", len(metadata.Tracks))
		trackValuesClause = trackValuesClause[:len(trackValuesClause)-1] // Remove trailing comma

		trackQuery := fmt.Sprintf(`
			INSERT INTO tracks (track_id, name, artists, download_status)
			VALUES %s
			ON CONFLICT(track_id) DO NOTHING
		`, trackValuesClause)

		trackArgs := make([]interface{}, 0, len(metadata.Tracks)*3)
		for _, track := range metadata.Tracks {
			artistsStr := strings.Join(track.Artists, ", ")
			trackArgs = append(trackArgs, track.ID, track.Name, artistsStr)
		}

		_, err = tx.Exec(trackQuery, trackArgs...)
		if err != nil {
			tx.Rollback()
			http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
			return
		}

		// Build bulk insert query for playlist_tracks
		playlistValuesClause := strings.Repeat("(?, ?),", len(metadata.Tracks))
		playlistValuesClause = playlistValuesClause[:len(playlistValuesClause)-1] // Remove trailing comma

		playlistQuery := fmt.Sprintf(`
			INSERT INTO playlist_tracks (playlist_id, track_id)
			VALUES %s
			ON CONFLICT(playlist_id, track_id) DO NOTHING
		`, playlistValuesClause)

		playlistArgs := make([]interface{}, 0, len(metadata.Tracks)*2)
		for _, track := range metadata.Tracks {
			playlistArgs = append(playlistArgs, req.PlaylistID, track.ID)
		}

		_, err = tx.Exec(playlistQuery, playlistArgs...)
		if err != nil {
			tx.Rollback()
			http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
			return
		}
	}

	tx.Commit()

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

// tracksHandler returns current state snapshot of all tracks
func (s *Server) tracksHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT track_id, name, artists,
		       download_status, error_message,
		       demucs_status, demucs_error_message
		FROM tracks
	`)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tracks []TrackState
	for rows.Next() {
		var trackID, name, artists, downloadStatus, demucsStatus string
		var downloadError, demucsError sql.NullString
		rows.Scan(&trackID, &name, &artists, &downloadStatus, &downloadError, &demucsStatus, &demucsError)

		// Map download status
		var downloadProgress float64
		switch downloadStatus {
		case "completed":
			downloadProgress = 100
		case "in_progress":
			downloadProgress = 0 // Real-time progress via SSE
		default:
			downloadProgress = 0
		}

		// Map demucs status
		var demucsProgress float64
		switch demucsStatus {
		case "completed":
			demucsProgress = 100
		case "in_progress":
			demucsProgress = 0 // Real-time progress via SSE
		default:
			demucsProgress = 0
		}

		track := TrackState{
			TrackID:          trackID,
			Name:             name,
			Artists:          artists,
			DownloadStatus:   downloadStatus,
			DownloadProgress: downloadProgress,
			DemucsStatus:     demucsStatus,
			DemucsProgress:   demucsProgress,
		}
		if downloadError.Valid {
			track.DownloadError = downloadError.String
		}
		if demucsError.Valid {
			track.DemucsError = demucsError.String
		}
		tracks = append(tracks, track)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tracks)
}

// progressStreamHandler streams progress updates via SSE
func (s *Server) progressStreamHandler(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Create client channel (unbuffered - backpressure from slow clients)
	clientChan := make(chan ProgressEvent)
	s.progress.newClients <- clientChan

	// Cleanup on disconnect
	defer func() {
		s.progress.closingClients <- clientChan
	}()

	// Stream updates only (no initial state - client gets that from /tracks)
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

func main() {
	// Initialize database
	db, err := initDB()
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer db.Close()

	// Initialize server with Spotify credentials
	server := &Server{
		spotifyClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		spotifyClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		jobQueue:            make(chan *DownloadJob, 1000),
		demucsQueue:         make(chan *DemucsJob, 1000),
		db:                  db,
		progress:            NewProgressBroadcaster(),
	}

	if server.spotifyClientID == "" || server.spotifyClientSecret == "" {
		log.Fatal("SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET environment variables must be set")
	}

	// Verify download status against files
	log.Println("Verifying download status against files...")
	if err := server.verifyDownloadStatus(); err != nil {
		log.Printf("Warning: Failed to verify download status: %v", err)
	}

	// Load pending jobs from database into channel
	if err := server.loadPendingJobs(); err != nil {
		log.Printf("Warning: Failed to load pending jobs: %v", err)
	}

	// Load pending Demucs jobs
	if err := server.loadPendingDemucsJobs(); err != nil {
		log.Printf("Warning: Failed to load pending Demucs jobs: %v", err)
	}

	// Start download worker pool BEFORE starting HTTP server
	for i := 0; i < numWorkers; i++ {
		go server.worker()
	}
	log.Printf("Started %d download workers", numWorkers)

	// Start Demucs worker pool
	for i := 0; i < numDemucsWorkers; i++ {
		go server.demucsWorker()
	}
	log.Printf("Started %d Demucs workers", numDemucsWorkers)

	// Register handlers
	http.HandleFunc("/setup-playlist", server.setupPlaylistHandler)
	http.HandleFunc("/tracks", server.tracksHandler)
	http.HandleFunc("/progress/stream", server.progressStreamHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
