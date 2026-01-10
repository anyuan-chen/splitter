package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"separate/server/models"
)

// getAccessTokenWithExpiry obtains an access token and expiry information using client credentials flow
func getAccessTokenWithExpiry(config models.SpotifyConfig) (*models.TokenResponse, error) {
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

	var tokenResp models.TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	return &tokenResp, nil
}

// GetAccessToken obtains an access token using client credentials flow
func GetAccessToken(config models.SpotifyConfig) (string, error) {
	tokenResp, err := getAccessTokenWithExpiry(config)
	if err != nil {
		return "", err
	}
	return tokenResp.AccessToken, nil
}

// GetAccessTokenWithDetails exposes the full response including expiry
func GetAccessTokenWithDetails(config models.SpotifyConfig) (*models.TokenResponse, error) {
	return getAccessTokenWithExpiry(config)
}

// fetchPlaylistPage fetches a single page of playlist data
func fetchPlaylistPage(playlistID, accessToken, pageURL string) (requestURL string, response *playlistResponse, err error) {
	var reqURL string
	if pageURL != "" {
		reqURL = pageURL
	} else {
		reqURL = fmt.Sprintf("https://api.spotify.com/v1/playlists/%s", playlistID)
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("failed to fetch playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("playlist request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var playlistResp playlistResponse
	if err := json.NewDecoder(resp.Body).Decode(&playlistResp); err != nil {
		return "", nil, fmt.Errorf("failed to decode playlist response: %w", err)
	}

	return reqURL, &playlistResp, nil
}

// Private structs for JSON decoding of Spotify responses
type playlistResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Tracks      struct {
		Items []struct {
			Track trackObject `json:"track"`
		} `json:"items"`
		Next  string `json:"next"`
		Total int    `json:"total"`
	} `json:"tracks"`
}

type trackObject struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	DurationMs   int    `json:"duration_ms"`
	ExternalURLs struct {
		Spotify string `json:"spotify"`
	} `json:"external_urls"`
	PreviewURL  string `json:"preview_url"`
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

// GetPlaylistMetadataWithToken fetches all metadata for a Spotify playlist using a provided access token
func GetPlaylistMetadataWithToken(playlistID, accessToken string) (*models.PlaylistMetadata, error) {
	// Fetch first page of playlist
	_, playlistResp, err := fetchPlaylistPage(playlistID, accessToken, "")
	if err != nil {
		return nil, err
	}

	metadata := &models.PlaylistMetadata{
		Name:        playlistResp.Name,
		Description: playlistResp.Description,
		TotalTracks: playlistResp.Tracks.Total,
		Tracks:      make([]models.TrackMetadata, 0, playlistResp.Tracks.Total),
	}

	// Helper to process items
	processItems := func(items []struct {
		Track trackObject `json:"track"`
	}) {
		for _, item := range items {
			track := item.Track
			artists := make([]string, len(track.Artists))
			for i, artist := range track.Artists {
				artists[i] = artist.Name
			}

			metadata.Tracks = append(metadata.Tracks, models.TrackMetadata{
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
	}

	// Process first page
	processItems(playlistResp.Tracks.Items)

	// Fetch remaining pages if playlist has more than 100 tracks
	nextURL := playlistResp.Tracks.Next
	for nextURL != "" {
		_, pageResp, err := fetchPlaylistPage(playlistID, accessToken, nextURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch page: %w", err)
		}
		processItems(pageResp.Tracks.Items)
		nextURL = pageResp.Tracks.Next
	}

	return metadata, nil
}

// GetTrackMetadata fetches metadata for a single track using Spotify API
func GetTrackMetadata(trackID, accessToken string) (*models.TrackMetadata, error) {
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

	var trackResp trackObject
	if err := json.NewDecoder(resp.Body).Decode(&trackResp); err != nil {
		return nil, fmt.Errorf("failed to decode track response: %w", err)
	}

	artists := make([]string, len(trackResp.Artists))
	for i, artist := range trackResp.Artists {
		artists[i] = artist.Name
	}

	return &models.TrackMetadata{
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
