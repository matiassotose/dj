package downloader

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Downloader handles downloading audio from YouTube
type Downloader struct {
	downloadPath string
	ytdlpPath    string
	ffmpegPath   string
}

// DownloadResult contains the result of a download
type DownloadResult struct {
	FilePath   string
	Title      string
	Artist     string
	Duration   int // seconds
	YouTubeURL string
}

// ProgressCallback is called with download progress updates
type ProgressCallback func(progress float64, status string)

// New creates a new Downloader
func New(downloadPath string) (*Downloader, error) {
	// Ensure download path exists
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create download path: %w", err)
	}

	// Find yt-dlp
	ytdlpPath, err := exec.LookPath("yt-dlp")
	if err != nil {
		return nil, fmt.Errorf("yt-dlp not found in PATH: %w", err)
	}

	// Find ffmpeg
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	return &Downloader{
		downloadPath: downloadPath,
		ytdlpPath:    ytdlpPath,
		ffmpegPath:   ffmpegPath,
	}, nil
}

// SearchAndDownload searches YouTube and downloads the first result
func (d *Downloader) SearchAndDownload(ctx context.Context, query string, callback ProgressCallback) (*DownloadResult, error) {
	if callback != nil {
		callback(0, "Searching YouTube...")
	}

	// Search for the video
	videoURL, title, err := d.searchYouTube(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	if callback != nil {
		callback(10, fmt.Sprintf("Found: %s", title))
	}

	// Download the video
	return d.Download(ctx, videoURL, callback)
}

// Download downloads audio from a YouTube URL
func (d *Downloader) Download(ctx context.Context, url string, callback ProgressCallback) (*DownloadResult, error) {
	if callback != nil {
		callback(15, "Starting download...")
	}

	// Create a unique filename based on video ID
	outputTemplate := filepath.Join(d.downloadPath, "%(title)s.%(ext)s")

	// yt-dlp command for downloading audio
	args := []string{
		"-f", "bestaudio[ext=m4a]/bestaudio/best",
		"-x",                    // Extract audio
		"--audio-format", "mp3", // Convert to MP3
		"--audio-quality", "192K", // 192kbps
		"--embed-thumbnail", // Embed thumbnail as cover art
		"--add-metadata",    // Add metadata
		"--no-playlist",     // Don't download playlists
		"--no-warnings",
		"--progress",
		"--newline", // Progress on new lines
		"-o", outputTemplate,
		"--print", "after_move:filepath", // Print final file path
		"--extractor-args", "youtube:player_client=android,web", // Use alternative clients to avoid 403
		"--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		url,
	}

	cmd := exec.CommandContext(ctx, d.ytdlpPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start yt-dlp: %w", err)
	}

	// Parse progress from stderr
	var lastFilePath string
	var stderrLines []string
	progressRegex := regexp.MustCompile(`(\d+\.?\d*)%`)

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			stderrLines = append(stderrLines, line)
			if matches := progressRegex.FindStringSubmatch(line); len(matches) > 1 {
				if progress, err := strconv.ParseFloat(matches[1], 64); err == nil {
					// Scale progress: 15-90% for download
					scaledProgress := 15 + (progress * 0.75)
					if callback != nil {
						callback(scaledProgress, "Downloading...")
					}
				}
			}
		}
	}()

	// Read final file path from stdout
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && strings.HasSuffix(line, ".mp3") {
			lastFilePath = line
		}
	}

	if err := cmd.Wait(); err != nil {
		// Include stderr output in error message
		errMsg := "yt-dlp failed"
		if len(stderrLines) > 0 {
			// Get last few error lines
			start := len(stderrLines) - 3
			if start < 0 {
				start = 0
			}
			errMsg = fmt.Sprintf("yt-dlp failed: %s", strings.Join(stderrLines[start:], "; "))
		}
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}

	if lastFilePath == "" {
		// Try to find the downloaded file
		files, err := filepath.Glob(filepath.Join(d.downloadPath, "*.mp3"))
		if err != nil || len(files) == 0 {
			return nil, fmt.Errorf("download completed but file not found")
		}
		lastFilePath = files[len(files)-1]
	}

	if callback != nil {
		callback(100, "Download complete!")
	}

	// Extract title from filename
	title := strings.TrimSuffix(filepath.Base(lastFilePath), ".mp3")

	return &DownloadResult{
		FilePath:   lastFilePath,
		Title:      title,
		YouTubeURL: url,
	}, nil
}

// searchYouTube searches YouTube and returns the URL and title of the first result
func (d *Downloader) searchYouTube(ctx context.Context, query string) (url string, title string, err error) {
	// Use yt-dlp to search YouTube
	args := []string{
		"ytsearch1:" + query,
		"--get-url",
		"--get-title",
		"--no-warnings",
		"--no-playlist",
	}

	cmd := exec.CommandContext(ctx, d.ytdlpPath, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("search failed: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return "", "", fmt.Errorf("no results found for: %s", query)
	}

	title = strings.TrimSpace(lines[0])
	url = strings.TrimSpace(lines[1])

	// The URL from --get-url is the direct stream URL, we need the watch URL
	// So let's get the video ID instead
	args = []string{
		"ytsearch1:" + query,
		"--get-id",
		"--get-title",
		"--no-warnings",
		"--no-playlist",
	}

	cmd = exec.CommandContext(ctx, d.ytdlpPath, args...)
	output, err = cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("search failed: %w", err)
	}

	lines = strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return "", "", fmt.Errorf("no results found for: %s", query)
	}

	title = strings.TrimSpace(lines[0])
	videoID := strings.TrimSpace(lines[1])
	url = "https://www.youtube.com/watch?v=" + videoID

	return url, title, nil
}

// GetVideoInfo gets information about a YouTube video without downloading
func (d *Downloader) GetVideoInfo(ctx context.Context, url string) (title, artist string, duration int, err error) {
	args := []string{
		url,
		"--get-title",
		"--get-duration",
		"--no-warnings",
		"--no-playlist",
	}

	cmd := exec.CommandContext(ctx, d.ytdlpPath, args...)
	output, err := cmd.Output()
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to get video info: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 1 {
		return "", "", 0, fmt.Errorf("no info found")
	}

	title = strings.TrimSpace(lines[0])

	// Try to extract artist from title (common format: "Artist - Title")
	if parts := strings.SplitN(title, " - ", 2); len(parts) == 2 {
		artist = strings.TrimSpace(parts[0])
		title = strings.TrimSpace(parts[1])
	}

	if len(lines) > 1 {
		duration = parseDuration(strings.TrimSpace(lines[1]))
	}

	return title, artist, duration, nil
}

// parseDuration parses a duration string like "3:45" or "1:23:45" into seconds
func parseDuration(s string) int {
	parts := strings.Split(s, ":")
	total := 0
	multiplier := 1

	for i := len(parts) - 1; i >= 0; i-- {
		if val, err := strconv.Atoi(parts[i]); err == nil {
			total += val * multiplier
		}
		multiplier *= 60
	}

	return total
}

// Cleanup removes a downloaded file
func (d *Downloader) Cleanup(filePath string) error {
	return os.Remove(filePath)
}

// IsYouTubeURL checks if a string is a YouTube URL
func IsYouTubeURL(s string) bool {
	patterns := []string{
		`youtube\.com/watch\?v=`,
		`youtu\.be/`,
		`youtube\.com/shorts/`,
		`music\.youtube\.com/watch\?v=`,
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, s); matched {
			return true
		}
	}
	return false
}

// ExtractYouTubeID extracts the video ID from a YouTube URL
func ExtractYouTubeID(url string) string {
	patterns := []struct {
		regex *regexp.Regexp
		group int
	}{
		{regexp.MustCompile(`youtube\.com/watch\?v=([a-zA-Z0-9_-]{11})`), 1},
		{regexp.MustCompile(`youtu\.be/([a-zA-Z0-9_-]{11})`), 1},
		{regexp.MustCompile(`youtube\.com/shorts/([a-zA-Z0-9_-]{11})`), 1},
		{regexp.MustCompile(`music\.youtube\.com/watch\?v=([a-zA-Z0-9_-]{11})`), 1},
	}

	for _, p := range patterns {
		if matches := p.regex.FindStringSubmatch(url); len(matches) > p.group {
			return matches[p.group]
		}
	}
	return ""
}
