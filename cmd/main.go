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

File format (one song per line):
  Daft Punk - Around The World
  Tame Impala - Let It Happen
  # This is a comment
  https://youtube.com/watch?v=xxx

Environment variables (.env supported):
  SPOTIFY_CLIENT_ID      For Spotify URL support
  SPOTIFY_CLIENT_SECRET  For Spotify URL support
`)
	}
	flag.Parse()

	// Collect songs from args and/or file
	var songs []string

	// From arguments
	songs = append(songs, flag.Args()...)

	// From file
	if *inputFile != "" {
		fileSongs, err := readSongsFromFile(*inputFile)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			os.Exit(1)
		}
		songs = append(songs, fileSongs...)
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

	// Initialize Spotify (optional)
	var spotifyClient *spotify.Client
	if *spotifyID != "" && *spotifySecret != "" {
		spotifyClient, _ = spotify.New(*spotifyID, *spotifySecret)
	}

	// Setup cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nCancelling...")
		cancel()
	}()

	// Print header
	fmt.Printf("\n📁 %s\n", outDir)
	fmt.Printf("🎵 %d song(s)\n\n", len(songs))

	// Download each song
	success, failed := 0, 0

	for i, song := range songs {
		select {
		case <-ctx.Done():
			fmt.Println("Cancelled")
			os.Exit(1)
		default:
		}

		fmt.Printf("[%d/%d] %s\n", i+1, len(songs), truncate(song, 55))

		// Resolve Spotify URL to search query
		query := song
		if spotify.IsSpotifyTrackURL(song) && spotifyClient != nil {
			if info, err := spotifyClient.GetTrack(ctx, spotify.ExtractSpotifyID(song)); err == nil {
				query = info.SearchQuery
				fmt.Printf("  → %s - %s", info.Artist, info.Name)
				if info.BPM > 0 {
					fmt.Printf(" [%.0f BPM, %s]", info.BPM, info.Key)
				}
				fmt.Println()
			}
		}

		// Download
		result, err := download(ctx, dl, query)
		if err != nil {
			fmt.Printf("  ✗ %v\n\n", err)
			failed++
			continue
		}

		fmt.Printf("  ✓ %s\n\n", filepath.Base(result.FilePath))
		success++
	}

	// Summary
	fmt.Printf("Done: %d downloaded, %d failed\n", success, failed)
	if failed > 0 {
		os.Exit(1)
	}
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

// download downloads a song with progress
func download(ctx context.Context, dl *downloader.Downloader, query string) (*downloader.DownloadResult, error) {
	var lastPct float64
	progress := func(pct float64, status string) {
		if pct-lastPct >= 20 || pct >= 100 {
			fmt.Printf("  → %.0f%%\n", pct)
			lastPct = pct
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
