import { useState, useEffect, useCallback } from 'react';
import { api, TrackState, ProgressEvent } from '@/lib/api';

export function useTracks() {
  const [tracks, setTracks] = useState<TrackState[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchTracks = useCallback(async () => {
    try {
      const initialTracks = await api.getTracks();
      setTracks(initialTracks);
      setLoading(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch tracks');
      setLoading(false);
    }
  }, []);

  // Initial fetch
  useEffect(() => {
    fetchTracks();
  }, [fetchTracks]);

  // Handle progress events
  const handleProgressEvent = useCallback((event: ProgressEvent) => {
    setTracks((currentTracks) => {
      return currentTracks.map((track) => {
        if (track.track_id !== event.track_id) {
          return track;
        }

        const updatedTrack = { ...track };

        if (event.type === 'download') {
          // Map event status to track status
          if (event.status === 'downloading') {
            updatedTrack.download_status = 'in_progress';
            updatedTrack.download_progress = event.progress;
          } else if (event.status === 'completed') {
            updatedTrack.download_status = 'completed';
            updatedTrack.download_progress = 100;
            updatedTrack.download_error = undefined;
          } else if (event.status === 'failed') {
            updatedTrack.download_status = 'failed';
            updatedTrack.download_error = event.error;
          }
        } else if (event.type === 'demucs') {
          if (event.status === 'pending') {
            updatedTrack.demucs_status = 'in_progress';
            updatedTrack.demucs_progress = 0;
          } else if (event.status === 'processing') {
            updatedTrack.demucs_status = 'in_progress';
            updatedTrack.demucs_progress = event.progress;
          } else if (event.status === 'completed') {
            updatedTrack.demucs_status = 'completed';
            updatedTrack.demucs_progress = 100;
            updatedTrack.demucs_error = undefined;
          } else if (event.status === 'failed') {
            updatedTrack.demucs_status = 'failed';
            updatedTrack.demucs_error = event.error;
          }
        }

        return updatedTrack;
      });
    });
  }, []);

  // SSE Subscription
  useEffect(() => {
    if (loading) return; // Wait for initial fetch

    const eventSource = api.createProgressSource();

    eventSource.onmessage = (event) => {
      try {
        const parsedEvent: ProgressEvent = JSON.parse(event.data);
        handleProgressEvent(parsedEvent);
      } catch (e) {
        console.error('Failed to parse progress event:', e);
      }
    };

    eventSource.onerror = (e) => {
      console.error('SSE Error:', e);
      // Optional: logic to reconnect or show status
    };

    return () => {
      eventSource.close();
    };
  }, [loading, handleProgressEvent]);

  return { tracks, loading, error, refresh: fetchTracks };
}
