package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// SpotifyConfig holds configuration for Spotify API access
type SpotifyConfig struct {
	ClientID     string
	ClientSecret string
	PlaylistID   string
}

// TrackMetadata represents metadata for a single track
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

// spotifyTokenResponse represents the OAuth token response
type spotifyTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// spotifyPlaylistResponse represents the API response for a playlist
type spotifyPlaylistResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Tracks      struct {
		Items []struct {
			Track struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				DurationMs int    `json:"duration_ms"`
				ExternalURLs struct {
					Spotify string `json:"spotify"`
				} `json:"external_urls"`
				PreviewURL   string `json:"preview_url"`
				ExternalIDs struct {
					ISRC string `json:"isrc"`
				} `json:"external_ids"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name        string `json:"name"`
					ReleaseDate string `json:"release_date"`
				} `json:"album"`
			} `json:"track"`
		} `json:"items"`
		Next  string `json:"next"`
		Total int    `json:"total"`
	} `json:"tracks"`
}

// getAccessToken obtains an access token using client credentials flow
func getAccessToken(config SpotifyConfig) (string, error) {
	tokenResp, err := getAccessTokenWithExpiry(config)
	if err != nil {
		return "", err
	}
	return tokenResp.AccessToken, nil
}

// getAccessTokenWithExpiry obtains an access token and expiry information using client credentials flow
func getAccessTokenWithExpiry(config SpotifyConfig) (*spotifyTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(config.ClientID, config.ClientSecret)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp spotifyTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// fetchPlaylistPage fetches a single page of playlist data
func fetchPlaylistPage(playlistID, accessToken, pageURL string) (*spotifyPlaylistResponse, error) {
	var reqURL string
	if pageURL != "" {
		reqURL = pageURL
	} else {
		reqURL = fmt.Sprintf("https://api.spotify.com/v1/playlists/%s", playlistID)
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("playlist request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var playlistResp spotifyPlaylistResponse
	if err := json.NewDecoder(resp.Body).Decode(&playlistResp); err != nil {
		return nil, fmt.Errorf("failed to decode playlist response: %w", err)
	}

	return &playlistResp, nil
}

// GetPlaylistMetadata fetches all metadata for a Spotify playlist
func GetPlaylistMetadata(config SpotifyConfig) (*PlaylistMetadata, error) {
	// Get access token
	accessToken, err := getAccessToken(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	return GetPlaylistMetadataWithToken(config.PlaylistID, accessToken)
}

// GetPlaylistMetadataWithToken fetches all metadata for a Spotify playlist using a provided access token
func GetPlaylistMetadataWithToken(playlistID, accessToken string) (*PlaylistMetadata, error) {
	// Fetch first page of playlist
	playlistResp, err := fetchPlaylistPage(playlistID, accessToken, "")
	if err != nil {
		return nil, err
	}

	metadata := &PlaylistMetadata{
		Name:        playlistResp.Name,
		Description: playlistResp.Description,
		TotalTracks: playlistResp.Tracks.Total,
		Tracks:      make([]TrackMetadata, 0, playlistResp.Tracks.Total),
	}

	// Process first page
	for _, item := range playlistResp.Tracks.Items {
		track := item.Track
		artists := make([]string, len(track.Artists))
		for i, artist := range track.Artists {
			artists[i] = artist.Name
		}

		metadata.Tracks = append(metadata.Tracks, TrackMetadata{
			ID:          track.ID,
			Name:        track.Name,
			Artists:     artists,
			Album:       track.Album.Name,
			DurationMs:  track.DurationMs,
			SpotifyURL:  track.ExternalURLs.Spotify,
			PreviewURL:  track.PreviewURL,
			ReleaseDate: track.Album.ReleaseDate,
			ISRC:        track.ExternalIDs.ISRC,
		})
	}

	// Fetch remaining pages if playlist has more than 100 tracks
	nextURL := playlistResp.Tracks.Next
	for nextURL != "" {
		pageResp, err := fetchPlaylistPage(playlistID, accessToken, nextURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch page: %w", err)
		}

		for _, item := range pageResp.Tracks.Items {
			track := item.Track
			artists := make([]string, len(track.Artists))
			for i, artist := range track.Artists {
				artists[i] = artist.Name
			}

			metadata.Tracks = append(metadata.Tracks, TrackMetadata{
				ID:          track.ID,
				Name:        track.Name,
				Artists:     artists,
				Album:       track.Album.Name,
				DurationMs:  track.DurationMs,
				SpotifyURL:  track.ExternalURLs.Spotify,
				PreviewURL:  track.PreviewURL,
				ReleaseDate: track.Album.ReleaseDate,
				ISRC:        track.ExternalIDs.ISRC,
			})
		}

		nextURL = pageResp.Tracks.Next
	}

	return metadata, nil
}

// GetTrackMetadata fetches metadata for a single track using Spotify API
func GetTrackMetadata(trackID, accessToken string) (*TrackMetadata, error) {
	reqURL := fmt.Sprintf("https://api.spotify.com/v1/tracks/%s", trackID)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch track: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("track request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var trackResp struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		DurationMs int    `json:"duration_ms"`
		ExternalURLs struct {
			Spotify string `json:"spotify"`
		} `json:"external_urls"`
		PreviewURL   string `json:"preview_url"`
		ExternalIDs struct {
			ISRC string `json:"isrc"`
		} `json:"external_ids"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Album struct {
			Name        string `json:"name"`
			ReleaseDate string `json:"release_date"`
		} `json:"album"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&trackResp); err != nil {
		return nil, fmt.Errorf("failed to decode track response: %w", err)
	}

	artists := make([]string, len(trackResp.Artists))
	for i, artist := range trackResp.Artists {
		artists[i] = artist.Name
	}

	return &TrackMetadata{
		ID:          trackResp.ID,
		Name:        trackResp.Name,
		Artists:     artists,
		Album:       trackResp.Album.Name,
		DurationMs:  trackResp.DurationMs,
		SpotifyURL:  trackResp.ExternalURLs.Spotify,
		PreviewURL:  trackResp.PreviewURL,
		ReleaseDate: trackResp.Album.ReleaseDate,
		ISRC:        trackResp.ExternalIDs.ISRC,
	}, nil
}
