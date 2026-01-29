# dj - YouTube Music Downloader

Simple CLI to download music from YouTube as MP3 files.

## Features

- Download songs by name or YouTube URL
- Download from Spotify track URLs
- Download entire Spotify playlists
- Batch download from a text file
- MP3 output at 192kbps

## Requirements

- Go 1.22+
- [yt-dlp](https://github.com/yt-dlp/yt-dlp)
- [ffmpeg](https://ffmpeg.org/)

### Install dependencies

```bash
# macOS
brew install yt-dlp ffmpeg

# Ubuntu/Debian
sudo apt install ffmpeg
pip install yt-dlp

# Windows
winget install yt-dlp ffmpeg
```

## Installation

```bash
# Clone and build
git clone <repo>
cd dj
go build -o dj ./cmd

# Or install directly
go install ./cmd
```

## Usage

```bash
# Download a single song
./dj "Daft Punk - Around The World"

# Download multiple songs
./dj "Song 1" "Song 2" "Song 3"

# Download to specific folder
./dj -o ~/Music "Tame Impala Let It Happen"

# Download from a text file
./dj -f playlist.txt

# Download a Spotify playlist (requires credentials)
./dj "https://open.spotify.com/playlist/xxxxx"

# Download a Spotify track
./dj "https://open.spotify.com/track/xxxxx"
```

## Text File Format

Create a text file with one song per line:

```
# playlist.txt
Daft Punk - Around The World
Tame Impala - Let It Happen
MGMT - Electric Feel

# Comments start with #
# URLs also work:
https://www.youtube.com/watch?v=dQw4w9WgXcQ
https://open.spotify.com/track/xxxxx
https://open.spotify.com/playlist/xxxxx
```

Then run:
```bash
./dj -f playlist.txt -o ~/Music
```

## Spotify Support

To use Spotify URLs (tracks or playlists), create a `.env` file:

```bash
SPOTIFY_CLIENT_ID=your_client_id
SPOTIFY_CLIENT_SECRET=your_client_secret
```

Get credentials from [Spotify Developer Dashboard](https://developer.spotify.com/dashboard):
1. Create a new app
2. Copy the Client ID and Client Secret

### Spotify Playlist Example

```bash
# Download entire playlist
./dj "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M" -o ~/Music

# The CLI will:
# 1. Fetch playlist info from Spotify
# 2. Get all track names
# 3. Search and download each from YouTube
```

## Options

| Flag | Description | Default |
|------|-------------|---------|
| `-o` | Output directory | Current directory |
| `-f` | Input file with songs | - |
| `-spotify-id` | Spotify Client ID | env var |
| `-spotify-secret` | Spotify Client Secret | env var |

## Project Structure

```
dj/
├── cmd/main.go           # CLI entry point
├── internal/
│   ├── downloader/       # YouTube download logic
│   └── spotify/          # Spotify API client
├── go.mod
├── Makefile
└── README.md
```

## License

MIT
