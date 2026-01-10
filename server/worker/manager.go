package worker

import (
	"log"
	"path/filepath"
	"strings"

	"separate/server/core"
	"separate/server/db"
	"separate/server/models"
)

type WorkerManager struct {
	db          *db.DB
	progress    *core.ProgressBroadcaster
	demucsQueue chan *models.DemucsJob
}

func NewWorkerManager(db *db.DB, progress *core.ProgressBroadcaster, demucsQueue chan *models.DemucsJob) *WorkerManager {
	return &WorkerManager{
		db:          db,
		progress:    progress,
		demucsQueue: demucsQueue,
	}
}

// DownloadWorker processes download jobs
func (wm *WorkerManager) DownloadWorker(jobQueue <-chan *models.DownloadJob) {
	for job := range jobQueue {
		artistsStr := strings.Join(job.Track.Artists, ", ")
		log.Printf("Downloading track: %s by %s", job.Track.Name, artistsStr)

		// Send pending event
		wm.progress.SendEvent(models.ProgressEvent{
			TrackID:  job.Track.ID,
			Type:     "download",
			Status:   "pending",
			Progress: 0,
		})

		// Mark as in_progress in database
		wm.db.UpdateDownloadStatus(job.Track.ID, "in_progress", "")

		// Download with progress reporting
		err := DownloadTrackFromSpotifyWithProgress(job.Track, wm.progress.Events())

		if err != nil {
			log.Printf("Failed to download %s: %v", job.Track.Name, err)
			wm.db.UpdateDownloadStatus(job.Track.ID, "failed", err.Error())

			// Send failed event
			wm.progress.SendEvent(models.ProgressEvent{
				TrackID:  job.Track.ID,
				Type:     "download",
				Status:   "failed",
				Progress: 0,
				Error:    err.Error(),
			})
		} else {
			outputPath := filepath.Join("songs", job.Track.ID, "base.mp3")
			log.Printf("Downloaded: %s → %s", job.Track.Name, outputPath)
			wm.db.UpdateDownloadStatus(job.Track.ID, "completed", "")

			// Send completed event
			wm.progress.SendEvent(models.ProgressEvent{
				TrackID:  job.Track.ID,
				Type:     "download",
				Status:   "completed",
				Progress: 100,
			})

			// Automatically queue Demucs processing
			wm.demucsQueue <- &models.DemucsJob{
				Track:     job.Track,
				InputPath: outputPath,
			}
		}
	}
}

// DemucsWorker processes Demucs separation jobs
func (wm *WorkerManager) DemucsWorker(demucsQueue <-chan *models.DemucsJob) {
	for job := range demucsQueue {
		artistsStr := strings.Join(job.Track.Artists, ", ")
		log.Printf("Processing Demucs: %s by %s", job.Track.Name, artistsStr)

		// Send pending event
		wm.progress.SendEvent(models.ProgressEvent{
			TrackID:  job.Track.ID,
			Type:     "demucs",
			Status:   "pending",
			Progress: 0,
		})

		// Mark as in_progress in database
		wm.db.UpdateDemucsStatus(job.Track.ID, "in_progress", "")

		// Process with Demucs and progress reporting
		err := ProcessTrackWithDemucs(job.Track, job.InputPath, wm.progress.Events())

		if err != nil {
			log.Printf("Failed to process Demucs for %s: %v", job.Track.Name, err)
			wm.db.UpdateDemucsStatus(job.Track.ID, "failed", err.Error())

			// Send failed event
			wm.progress.SendEvent(models.ProgressEvent{
				TrackID:  job.Track.ID,
				Type:     "demucs",
				Status:   "failed",
				Progress: 0,
				Error:    err.Error(),
			})
		} else {
			log.Printf("Demucs completed: %s → songs/%s/demucs/", job.Track.Name, job.Track.ID)
			wm.db.UpdateDemucsStatus(job.Track.ID, "completed", "")

			// Send completed event
			wm.progress.SendEvent(models.ProgressEvent{
				TrackID:  job.Track.ID,
				Type:     "demucs",
				Status:   "completed",
				Progress: 100,
			})
		}
	}
}
