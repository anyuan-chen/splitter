package models

import "time"

// TrackMetadata represents metadata for a single track (from Spotify)
type TrackMetadata struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Artists     []string `json:"artists"`
	Album       string   `json:"album"`
	DurationMs  int      `json:"duration_ms"`
	SpotifyURL  string   `json:"spotify_url"`
	PreviewURL  string   `json:"preview_url"`
	ReleaseDate string   `json:"release_date"`
	ISRC        string   `json:"isrc"`
}

// PlaylistMetadata represents metadata for an entire playlist
type PlaylistMetadata struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	TotalTracks int             `json:"total_tracks"`
	Tracks      []TrackMetadata `json:"tracks"`
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

// DemucsJob represents a Demucs separation job
type DemucsJob struct {
	Track     TrackMetadata
	InputPath string
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
	TrackID          string  `json:"track_id"`
	Name             string  `json:"name"`
	Artists          string  `json:"artists"`
	DownloadStatus   string  `json:"download_status"`
	DownloadProgress float64 `json:"download_progress"`
	DownloadError    string  `json:"download_error,omitempty"`
	DemucsStatus     string  `json:"demucs_status"`
	DemucsProgress   float64 `json:"demucs_progress"`
	DemucsError      string  `json:"demucs_error,omitempty"`
}

// SpotifyConfig holds configuration for Spotify API access
type SpotifyConfig struct {
	ClientID     string
	ClientSecret string
	PlaylistID   string
}

// TokenResponse represents the OAuth token response from Spotify
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// ServerConfig holds the main application configuration
type ServerConfig struct {
	SpotifyClientID     string
	SpotifyClientSecret string
	Port                string
	NumWorkers          int
	NumDemucsWorkers    int
}

// AppState holds the application state
type AppState struct {
	Config      ServerConfig
	AccessToken string
	TokenExpiry time.Time
}
