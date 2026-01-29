package spotify

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2/clientcredentials"
)

// Client wraps the Spotify API client
type Client struct {
	client *spotify.Client
}

// TrackInfo contains information about a Spotify track
type TrackInfo struct {
	ID           string
	Name         string
	Artist       string
	Album        string
	SpotifyURL   string
	SearchQuery  string // For YouTube search
	BPM          float64
	Key          string
	Energy       float64
	Danceability float64
	Valence      float64
}

// PlaylistInfo contains information about a Spotify playlist
type PlaylistInfo struct {
	ID     string
	Name   string
	Owner  string
	Tracks []TrackInfo
}

// New creates a new Spotify client
func New(clientID, clientSecret string) (*Client, error) {
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("spotify credentials not configured")
	}

	config := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     spotifyauth.TokenURL,
	}

	token, err := config.Token(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get spotify token: %w", err)
	}

	httpClient := spotifyauth.New().Client(context.Background(), token)
	client := spotify.New(httpClient)

	return &Client{client: client}, nil
}

// GetTrack gets information about a Spotify track
func (c *Client) GetTrack(ctx context.Context, trackID string) (*TrackInfo, error) {
	track, err := c.client.GetTrack(ctx, spotify.ID(trackID))
	if err != nil {
		return nil, fmt.Errorf("failed to get track: %w", err)
	}

	artists := make([]string, len(track.Artists))
	for i, artist := range track.Artists {
		artists[i] = artist.Name
	}
	artistStr := strings.Join(artists, ", ")

	info := &TrackInfo{
		ID:          string(track.ID),
		Name:        track.Name,
		Artist:      artistStr,
		Album:       track.Album.Name,
		SpotifyURL:  string(track.ExternalURLs["spotify"]),
		SearchQuery: fmt.Sprintf("%s %s", artistStr, track.Name),
	}

	// Get audio features
	features, err := c.client.GetAudioFeatures(ctx, spotify.ID(trackID))
	if err == nil && len(features) > 0 && features[0] != nil {
		f := features[0]
		info.BPM = float64(f.Tempo)
		info.Key = keyToString(int(f.Key), int(f.Mode))
		info.Energy = float64(f.Energy)
		info.Danceability = float64(f.Danceability)
		info.Valence = float64(f.Valence)
	}

	return info, nil
}

// GetPlaylist gets information about a Spotify playlist
func (c *Client) GetPlaylist(ctx context.Context, playlistID string) (*PlaylistInfo, error) {
	playlist, err := c.client.GetPlaylist(ctx, spotify.ID(playlistID))
	if err != nil {
		return nil, fmt.Errorf("failed to get playlist: %w", err)
	}

	info := &PlaylistInfo{
		ID:    string(playlist.ID),
		Name:  playlist.Name,
		Owner: playlist.Owner.DisplayName,
	}

	// Get all tracks (handle pagination)
	tracks := playlist.Tracks.Tracks
	for page := 1; ; page++ {
		for _, item := range tracks {
			track := item.Track

			artists := make([]string, len(track.Artists))
			for i, artist := range track.Artists {
				artists[i] = artist.Name
			}
			artistStr := strings.Join(artists, ", ")

			trackInfo := TrackInfo{
				ID:          string(track.ID),
				Name:        track.Name,
				Artist:      artistStr,
				Album:       track.Album.Name,
				SpotifyURL:  string(track.ExternalURLs["spotify"]),
				SearchQuery: fmt.Sprintf("%s %s", artistStr, track.Name),
			}
			info.Tracks = append(info.Tracks, trackInfo)
		}

		// Check if there are more pages
		if playlist.Tracks.Next == "" {
			break
		}

		err := c.client.NextPage(ctx, &playlist.Tracks)
		if err != nil {
			break
		}
		tracks = playlist.Tracks.Tracks
	}

	// Get audio features for all tracks in batches
	if len(info.Tracks) > 0 {
		c.enrichTracksWithFeatures(ctx, info.Tracks)
	}

	return info, nil
}

// enrichTracksWithFeatures adds audio features to tracks
func (c *Client) enrichTracksWithFeatures(ctx context.Context, tracks []TrackInfo) {
	// Spotify API allows up to 100 tracks per request
	batchSize := 100

	for i := 0; i < len(tracks); i += batchSize {
		end := i + batchSize
		if end > len(tracks) {
			end = len(tracks)
		}

		ids := make([]spotify.ID, end-i)
		for j := i; j < end; j++ {
			ids[j-i] = spotify.ID(tracks[j].ID)
		}

		features, err := c.client.GetAudioFeatures(ctx, ids...)
		if err != nil {
			continue
		}

		for j, f := range features {
			if f == nil {
				continue
			}
			idx := i + j
			if idx < len(tracks) {
				tracks[idx].BPM = float64(f.Tempo)
				tracks[idx].Key = keyToString(int(f.Key), int(f.Mode))
				tracks[idx].Energy = float64(f.Energy)
				tracks[idx].Danceability = float64(f.Danceability)
				tracks[idx].Valence = float64(f.Valence)
			}
		}
	}
}

// keyToString converts Spotify's numeric key to a string representation
func keyToString(key, mode int) string {
	keys := []string{"C", "C#", "D", "D#", "E", "F", "F#", "G", "G#", "A", "A#", "B"}
	if key < 0 || key >= len(keys) {
		return ""
	}

	modeStr := "m" // minor
	if mode == 1 {
		modeStr = "" // major (no suffix)
	}

	return keys[key] + modeStr
}

// IsSpotifyURL checks if a string is a Spotify URL
func IsSpotifyURL(s string) bool {
	return strings.Contains(s, "spotify.com/") || strings.HasPrefix(s, "spotify:")
}

// IsSpotifyTrackURL checks if a string is a Spotify track URL
func IsSpotifyTrackURL(s string) bool {
	return strings.Contains(s, "spotify.com/track/") || strings.HasPrefix(s, "spotify:track:")
}

// IsSpotifyPlaylistURL checks if a string is a Spotify playlist URL
func IsSpotifyPlaylistURL(s string) bool {
	return strings.Contains(s, "spotify.com/playlist/") || strings.HasPrefix(s, "spotify:playlist:")
}

// IsSpotifyAlbumURL checks if a string is a Spotify album URL
func IsSpotifyAlbumURL(s string) bool {
	return strings.Contains(s, "spotify.com/album/") || strings.HasPrefix(s, "spotify:album:")
}

// ExtractSpotifyID extracts the ID from a Spotify URL or URI
func ExtractSpotifyID(s string) string {
	// Handle Spotify URIs (spotify:track:xxx)
	if strings.HasPrefix(s, "spotify:") {
		parts := strings.Split(s, ":")
		if len(parts) >= 3 {
			return parts[2]
		}
	}

	// Handle URLs
	patterns := []string{
		`spotify\.com/track/([a-zA-Z0-9]+)`,
		`spotify\.com/playlist/([a-zA-Z0-9]+)`,
		`spotify\.com/album/([a-zA-Z0-9]+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(s); len(matches) > 1 {
			// Remove any query parameters
			id := strings.Split(matches[1], "?")[0]
			return id
		}
	}

	return ""
}
