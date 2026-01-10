package db

import (
	"database/sql"
	"fmt"
	"strings"

	"separate/server/models"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

// InitDB initializes the SQLite database and creates tables
func InitDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path)
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

	return &DB{db}, nil
}

// GetPendingDownloadJobs returns all tracks that are pending download
func (db *DB) GetPendingDownloadJobs() ([]string, error) {
	rows, err := db.Query("SELECT track_id FROM tracks WHERE download_status = 'pending'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trackIDs []string
	for rows.Next() {
		var trackID string
		rows.Scan(&trackID)
		trackIDs = append(trackIDs, trackID)
	}
	return trackIDs, nil
}

// GetPendingDemucsJobs returns all tracks that are downloaded but pending Demucs processing
func (db *DB) GetPendingDemucsJobs() ([]models.TrackMetadata, error) {
	rows, err := db.Query(`
		SELECT track_id, name, artists
		FROM tracks
		WHERE download_status = 'completed' AND demucs_status = 'pending'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []models.TrackMetadata
	for rows.Next() {
		var trackID, name, artists string
		if err := rows.Scan(&trackID, &name, &artists); err != nil {
			continue
		}

		artistsList := strings.Split(artists, ", ")
		tracks = append(tracks, models.TrackMetadata{
			ID:      trackID,
			Name:    name,
			Artists: artistsList,
		})
	}
	return tracks, nil
}

// UpdateDownloadStatus updates the download status of a track
func (db *DB) UpdateDownloadStatus(trackID, status, errorMessage string) error {
	var err error
	if errorMessage != "" {
		_, err = db.Exec(`
			UPDATE tracks
			SET download_status = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP
			WHERE track_id = ?
		`, status, errorMessage, trackID)
	} else {
		_, err = db.Exec(`
			UPDATE tracks
			SET download_status = ?, error_message = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE track_id = ?
		`, status, trackID)
	}
	return err
}

// UpdateDemucsStatus updates the Demucs processing status of a track
func (db *DB) UpdateDemucsStatus(trackID, status, errorMessage string) error {
	var err error
	if errorMessage != "" {
		_, err = db.Exec(`
			UPDATE tracks
			SET demucs_status = ?, demucs_error_message = ?, updated_at = CURRENT_TIMESTAMP
			WHERE track_id = ?
		`, status, errorMessage, trackID)
	} else {
		_, err = db.Exec(`
			UPDATE tracks
			SET demucs_status = ?, demucs_error_message = NULL, updated_at = CURRENT_TIMESTAMP
			WHERE track_id = ?
		`, status, trackID)
	}
	return err
}

// SavePlaylistTracks saves tracks and their playlist association
func (db *DB) SavePlaylistTracks(playlistID string, tracks []models.TrackMetadata) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	if len(tracks) > 0 {
		// Build bulk insert query for tracks
		trackValuesClause := strings.Repeat("(?, ?, ?, 'pending'),", len(tracks))
		trackValuesClause = trackValuesClause[:len(trackValuesClause)-1]

		trackQuery := fmt.Sprintf(`
			INSERT INTO tracks (track_id, name, artists, download_status)
			VALUES %s
			ON CONFLICT(track_id) DO NOTHING
		`, trackValuesClause)

		trackArgs := make([]interface{}, 0, len(tracks)*3)
		for _, track := range tracks {
			artistsStr := strings.Join(track.Artists, ", ")
			trackArgs = append(trackArgs, track.ID, track.Name, artistsStr)
		}

		_, err = tx.Exec(trackQuery, trackArgs...)
		if err != nil {
			tx.Rollback()
			return err
		}

		// Build bulk insert query for playlist_tracks
		playlistValuesClause := strings.Repeat("(?, ?),", len(tracks))
		playlistValuesClause = playlistValuesClause[:len(playlistValuesClause)-1]

		playlistQuery := fmt.Sprintf(`
			INSERT INTO playlist_tracks (playlist_id, track_id)
			VALUES %s
			ON CONFLICT(playlist_id, track_id) DO NOTHING
		`, playlistValuesClause)

		playlistArgs := make([]interface{}, 0, len(tracks)*2)
		for _, track := range tracks {
			playlistArgs = append(playlistArgs, playlistID, track.ID)
		}

		_, err = tx.Exec(playlistQuery, playlistArgs...)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// GetAllTracks returns the current state of all tracks
func (db *DB) GetAllTracks() ([]models.TrackState, error) {
	rows, err := db.Query(`
		SELECT track_id, name, artists,
		       download_status, error_message,
		       demucs_status, demucs_error_message
		FROM tracks
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tracks []models.TrackState
	for rows.Next() {
		var trackID, name, artists, downloadStatus, demucsStatus string
		var downloadError, demucsError sql.NullString
		rows.Scan(&trackID, &name, &artists, &downloadStatus, &downloadError, &demucsStatus, &demucsError)

		// Map status to progress (simplified for snapshot)
		var downloadProgress float64
		if downloadStatus == "completed" {
			downloadProgress = 100
		}

		var demucsProgress float64
		if demucsStatus == "completed" {
			demucsProgress = 100
		}

		track := models.TrackState{
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
	return tracks, nil
}

// VerifyDownloadStatus checks actual files against DB status
func (db *DB) VerifyDownloadStatus(checkFileExists func(string) bool) error {
	rows, err := db.Query("SELECT track_id, download_status FROM tracks")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var trackID, status string
		rows.Scan(&trackID, &status)

		if checkFileExists(trackID) {
			if status != "completed" {
				db.UpdateDownloadStatus(trackID, "completed", "")
			}
		} else if status == "in_progress" {
			db.UpdateDownloadStatus(trackID, "pending", "")
		}
	}
	return nil
}

// GetTrack returns a single track by ID
func (db *DB) GetTrack(trackID string) (*models.TrackState, error) {
	var track models.TrackState
	var downloadError, demucsError sql.NullString
	var downloadStatus, demucsStatus string

	err := db.QueryRow(`
		SELECT track_id, name, artists,
		       download_status, error_message,
		       demucs_status, demucs_error_message
		FROM tracks
		WHERE track_id = ?
	`, trackID).Scan(
		&track.TrackID, &track.Name, &track.Artists,
		&downloadStatus, &downloadError,
		&demucsStatus, &demucsError,
	)
	if err != nil {
		return nil, err
	}

	track.DownloadStatus = downloadStatus
	track.DemucsStatus = demucsStatus

	if downloadStatus == "completed" {
		track.DownloadProgress = 100
	}
	if demucsStatus == "completed" {
		track.DemucsProgress = 100
	}

	if downloadError.Valid {
		track.DownloadError = downloadError.String
	}
	if demucsError.Valid {
		track.DemucsError = demucsError.String
	}

	return &track, nil
}
