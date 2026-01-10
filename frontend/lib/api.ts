export interface SetupPlaylistRequest {
  playlist_id: string;
}

export interface SetupPlaylistResponse {
  playlist_name: string;
  total_tracks: number;
  track_ids: string[];
}

export interface TrackState {
  track_id: string;
  name: string;
  artists: string;
  download_status: 'pending' | 'in_progress' | 'completed' | 'failed';
  download_progress: number;
  download_error?: string;
  demucs_status: 'pending' | 'in_progress' | 'completed' | 'failed';
  demucs_progress: number;
  demucs_error?: string;
}

export interface ProgressEvent {
  track_id: string;
  type: 'download' | 'demucs';
  status: 'pending' | 'downloading' | 'processing' | 'completed' | 'failed';
  progress: number;
  error?: string;
}

// Default to localhost:8080 if not specified
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = 'ApiError';
  }
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const errorText = await response.text().catch(() => 'Unknown error');
    throw new ApiError(response.status, errorText || response.statusText);
  }
  return response.json();
}

export const api = {
  /**
   * Initialize a playlist for downloading.
   * Creates directories and queues tracks for download.
   */
  async setupPlaylist(playlistId: string): Promise<SetupPlaylistResponse> {
    const response = await fetch(`${API_BASE_URL}/setup-playlist`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ playlist_id: playlistId }),
    });
    return handleResponse<SetupPlaylistResponse>(response);
  },

  /**
   * Fetch the current state of all tracks.
   */
  async getTracks(): Promise<TrackState[]> {
    const response = await fetch(`${API_BASE_URL}/tracks`);
    return handleResponse<TrackState[]>(response);
  },

  /**
   * Get the EventSource function for progress updates.
   * Returns a function that connects to the SSE stream.
   * To be used with useEffect or similar.
   */
  createProgressSource(): EventSource {
    return new EventSource(`${API_BASE_URL}/progress/stream`);
  },

  /**
   * Fetch a single track by ID.
   */
  async getTrack(trackId: string): Promise<TrackState> {
    const response = await fetch(`${API_BASE_URL}/tracks/${trackId}`);
    return handleResponse<TrackState>(response);
  },

  /**
   * Get the base URL for audio files.
   */
  getAudioUrl(trackId: string, filename: string): string {
    return `${API_BASE_URL}/songs/${trackId}/${filename}`;
  },
};
