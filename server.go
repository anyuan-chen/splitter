package main

import (
	"log"
	"net/http"
	"os"

	"separate/server/api"
	"separate/server/core"
	"separate/server/db"
	"separate/server/models"
	"separate/server/worker"
)

const (
	numWorkers       = 8
	numDemucsWorkers = 1 // Demucs is slow, process one at a time
)

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	// Initialize database
	database, err := db.InitDB("./queue.db")
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer database.Close()

	// Configuration
	config := models.SpotifyConfig{
		ClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		ClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
	}

	if config.ClientID == "" || config.ClientSecret == "" {
		log.Fatal("SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET environment variables must be set")
	}

	// Initialize queues
	downloadQueue := make(chan *models.DownloadJob, 1000)
	demucsQueue := make(chan *models.DemucsJob, 1000)

	// Initialize progress broadcaster
	progress := core.NewProgressBroadcaster()

	// Initialize worker manager
	workerManager := worker.NewWorkerManager(database, progress, demucsQueue)

	// Verify download status against files (Phase 1 sanity check)
	log.Println("Verifying download status against files...")
	checkFileExists := func(trackID string) bool {
		_, err := os.Stat("songs/" + trackID + "/base.mp3")
		return err == nil
	}
	if err := database.VerifyDownloadStatus(checkFileExists); err != nil {
		log.Printf("Warning: Failed to verify download status: %v", err)
	}

	// Load pending jobs from database
	pendingDownloads, err := database.GetPendingDownloadJobs()
	if err != nil {
		log.Printf("Warning: Failed to load pending jobs: %v", err)
	} else {
		log.Printf("Loading %d pending jobs from database", len(pendingDownloads))
		// We need to fetch metadata for these tracks effectively.
		// For simplicity, we might just re-queue them if we had full metadata.
		// However, GetPendingDownloadJobs only returns IDs.
		// We probably need to fetch metadata again or store it fully.
		// The original code fetched it from Spotify again.

		if len(pendingDownloads) > 0 {
			token, err := core.GetAccessToken(config)
			if err == nil {
				for _, trackID := range pendingDownloads {
					track, err := core.GetTrackMetadata(trackID, token)
					if err != nil {
						log.Printf("Failed to fetch metadata for %s: %v", trackID, err)
						continue
					}
					downloadQueue <- &models.DownloadJob{Track: *track}
				}
			} else {
				log.Printf("Failed to get token for reloading jobs: %v", err)
			}
		}
	}

	// Load pending Demucs jobs
	pendingDemucs, err := database.GetPendingDemucsJobs()
	if err != nil {
		log.Printf("Warning: Failed to load pending Demucs jobs: %v", err)
	} else {
		if len(pendingDemucs) > 0 {
			log.Printf("Queued %d tracks for Demucs processing", len(pendingDemucs))
			for _, track := range pendingDemucs {
				demucsQueue <- &models.DemucsJob{
					Track:     track,
					InputPath: "songs/" + track.ID + "/base.mp3",
				}
			}
		}
	}

	// Start download worker pool
	for i := 0; i < numWorkers; i++ {
		go workerManager.DownloadWorker(downloadQueue)
	}
	log.Printf("Started %d download workers", numWorkers)

	// Start Demucs worker pool
	for i := 0; i < numDemucsWorkers; i++ {
		go workerManager.DemucsWorker(demucsQueue)
	}
	log.Printf("Started %d Demucs workers", numDemucsWorkers)

	// Initialize API handlers
	apiHandler := api.NewHandler(database, progress, downloadQueue, config)

	// Register handlers
	// Register handlers with CORS middleware
	http.Handle("/setup-playlist", enableCORS(http.HandlerFunc(apiHandler.SetupPlaylistHandler)))
	http.Handle("/tracks", enableCORS(http.HandlerFunc(apiHandler.TracksHandler)))
	http.Handle("/tracks/", enableCORS(http.HandlerFunc(apiHandler.GetTrackHandler))) // Note: Trailing slash is important for subtree matching, but for specific ID we might need careful handling
	http.Handle("/progress/stream", enableCORS(http.HandlerFunc(apiHandler.ProgressStreamHandler)))

	// Serve static files
	fs := http.FileServer(http.Dir("./songs"))
	http.Handle("/songs/", http.StripPrefix("/songs/", enableCORS(fs)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}
