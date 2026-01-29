package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/yourusername/dj-bot/internal/downloader"
	"github.com/yourusername/dj-bot/internal/spotify"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
)

func main() {
	// Load .env file silently
	godotenv.Load()

	// Define flags
	outputDir := flag.String("o", ".", "Output directory")
	inputFile := flag.String("f", "", "Text file with songs (one per line)")
	spotifyID := flag.String("spotify-id", os.Getenv("SPOTIFY_CLIENT_ID"), "Spotify Client ID")
	spotifySecret := flag.String("spotify-secret", os.Getenv("SPOTIFY_CLIENT_SECRET"), "Spotify Client Secret")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `dj - Download music from YouTube

Usage:
  dj [options] <song>...
  dj [options] -f <file.txt>
  dj [options] <spotify-playlist-url>

Options:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  dj "Daft Punk - Around The World"
  dj "Song 1" "Song 2" "Song 3"
  dj -o ~/Music "Tame Impala Let It Happen"
  dj -f playlist.txt
  dj -f songs.txt -o ./downloads
  dj "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"

Supported inputs:
  - Song names: "Artist - Song Title"
  - YouTube URLs
  - Spotify track URLs
  - Spotify playlist URLs (downloads all tracks)
  - Text file with songs (one per line)

Environment variables (.env supported):
  SPOTIFY_CLIENT_ID      For Spotify URL support
  SPOTIFY_CLIENT_SECRET  For Spotify URL support
`)
	}
	flag.Parse()

	// Initialize Spotify client first (needed for playlist expansion)
	var spotifyClient *spotify.Client
	if *spotifyID != "" && *spotifySecret != "" {
		var err error
		spotifyClient, err = spotify.New(*spotifyID, *spotifySecret)
		if err != nil {
			fmt.Printf("%sWarning: Spotify init failed: %v%s\n", colorYellow, err, colorReset)
		}
	}

	// Setup context for API calls
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nCancelling...")
		cancel()
	}()

	// Collect songs from args and/or file
	var songs []string

	// From arguments (expand Spotify playlists)
	for _, arg := range flag.Args() {
		expanded := expandInput(ctx, arg, spotifyClient)
		songs = append(songs, expanded...)
	}

	// From file
	if *inputFile != "" {
		fileSongs, err := readSongsFromFile(*inputFile)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			os.Exit(1)
		}
		// Expand any Spotify playlists in the file
		for _, song := range fileSongs {
			expanded := expandInput(ctx, song, spotifyClient)
			songs = append(songs, expanded...)
		}
	}

	if len(songs) == 0 {
		fmt.Println("Error: No songs specified")
		fmt.Println("Use -h for help")
		os.Exit(1)
	}

	// Setup output directory
	outDir, err := filepath.Abs(*outputDir)
	if err != nil {
		fmt.Printf("Error: Invalid output directory: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("Error: Cannot create output directory: %v\n", err)
		os.Exit(1)
	}

	// Initialize downloader
	dl, err := downloader.New(outDir)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		fmt.Println("Make sure yt-dlp and ffmpeg are installed")
		os.Exit(1)
	}

	// Print header
	fmt.Printf("\n%s📁 %s%s\n", colorDim, outDir, colorReset)
	fmt.Printf("%s🎵 %d song(s)%s\n\n", colorCyan, len(songs), colorReset)

	// Download each song
	success, failed := 0, 0

	for i, song := range songs {
		select {
		case <-ctx.Done():
			fmt.Println("Cancelled")
			os.Exit(1)
		default:
		}

		fmt.Printf("%s[%d/%d]%s %s\n", colorBlue, i+1, len(songs), colorReset, truncate(song, 55))

		// Resolve Spotify track URL to search query
		query := song
		if spotify.IsSpotifyTrackURL(song) && spotifyClient != nil {
			if info, err := spotifyClient.GetTrack(ctx, spotify.ExtractSpotifyID(song)); err == nil {
				query = info.SearchQuery
				fmt.Printf("  %s→ %s - %s", colorDim, info.Artist, info.Name)
				if info.BPM > 0 {
					fmt.Printf(" [%.0f BPM, %s]", info.BPM, info.Key)
				}
				fmt.Printf("%s\n", colorReset)
			}
		}

		// Download
		result, err := download(ctx, dl, query)
		if err != nil {
			fmt.Printf("  %s✗ %v%s\n\n", colorRed, err, colorReset)
			failed++
			continue
		}

		fmt.Printf("  %s✓ %s%s\n\n", colorGreen, filepath.Base(result.FilePath), colorReset)
		success++
	}

	// Summary
	if failed > 0 {
		fmt.Printf("%sDone: %s%d downloaded%s, %s%d failed%s\n", colorBold, colorGreen, success, colorReset+colorBold, colorRed, failed, colorReset)
		os.Exit(1)
	} else {
		fmt.Printf("%sDone: %s%d downloaded%s\n", colorBold, colorGreen, success, colorReset)
	}
}

// expandInput expands a single input into one or more songs
// Handles Spotify playlists by fetching all tracks
func expandInput(ctx context.Context, input string, spotifyClient *spotify.Client) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	// Check if it's a Spotify playlist URL
	if spotify.IsSpotifyPlaylistURL(input) {
		if spotifyClient == nil {
			fmt.Printf("%sWarning: Spotify credentials required for playlist: %s%s\n", colorYellow, truncate(input, 50), colorReset)
			return nil
		}

		playlistID := spotify.ExtractSpotifyID(input)
		if playlistID == "" {
			fmt.Printf("%sWarning: Could not extract playlist ID from: %s%s\n", colorYellow, truncate(input, 50), colorReset)
			return nil
		}

		fmt.Printf("%s📋 Fetching Spotify playlist...%s\n", colorDim, colorReset)
		playlist, err := spotifyClient.GetPlaylist(ctx, playlistID)
		if err != nil {
			fmt.Printf("%sWarning: Failed to fetch playlist: %v%s\n", colorYellow, err, colorReset)
			return nil
		}

		fmt.Printf("%s📋 Playlist: %s%s%s (%d tracks)\n\n", colorCyan, colorBold, playlist.Name, colorReset, len(playlist.Tracks))

		// Convert tracks to search queries
		var songs []string
		for _, track := range playlist.Tracks {
			songs = append(songs, track.SearchQuery)
		}
		return songs
	}

	// Not a playlist, return as-is
	return []string{input}
}

// readSongsFromFile reads songs from a text file
func readSongsFromFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var songs []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		songs = append(songs, line)
	}
	return songs, scanner.Err()
}

// download downloads a song with progress bar
func download(ctx context.Context, dl *downloader.Downloader, query string) (*downloader.DownloadResult, error) {
	var lastPct float64
	barWidth := 30

	progress := func(pct float64, status string) {
		if pct < lastPct {
			return // Don't go backwards
		}
		lastPct = pct

		// Calculate filled portion
		filled := int(pct / 100 * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}

		// Build colored progress bar
		bar := colorGreen + strings.Repeat("█", filled) + colorDim + strings.Repeat("░", barWidth-filled) + colorReset

		// Format status/speed display
		statusDisplay := ""
		if strings.Contains(status, "/s") {
			// It's a speed indicator
			statusDisplay = fmt.Sprintf(" %s%s%s", colorCyan, status, colorReset)
		} else if status != "" {
			// It's a status message - show dimmed
			statusDisplay = fmt.Sprintf(" %s%s%s", colorDim, truncate(status, 40), colorReset)
		}

		// Print with carriage return to overwrite (add padding to clear old content)
		fmt.Printf("\r  [%s] %s%3.0f%%%s%-45s", bar, colorYellow, pct, colorReset, statusDisplay)

		// New line when complete
		if pct >= 100 {
			fmt.Println()
		}
	}

	if downloader.IsYouTubeURL(query) {
		return dl.Download(ctx, query, progress)
	}
	return dl.SearchAndDownload(ctx, query, progress)
}

// truncate shortens a string
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
