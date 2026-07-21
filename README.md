# Lyrics Viewer for macOS

A beautiful, ultra-smooth, full-screen terminal UI for viewing synced lyrics of the currently playing Apple Music or Spotify song.
Built with **Go**, [Bubble Tea](https://github.com/charmbracelet/bubbletea), and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Features
- **Live Syncing**: Lyrics automatically scroll along with Apple Music or Spotify.
- **Ultra-Smooth Progress Bar**: Interpolates time and utilizes fractional Unicode blocks (8x resolution) for a continuously gliding progress bar.
- **Responsive Design**: The album artwork, lyrics view, and layout automatically adapt dynamically to your terminal window size.
- **Album Artwork**: Fetches and renders high-quality album art directly in your terminal using ANSI block characters.

## Install

### Homebrew

```bash
brew install moKshagna-p/tap/lrc
```

This installs both `lrc` and its `lyrics` alias. `brew install lrc` becomes available once the formula is accepted into [Homebrew/core](https://github.com/Homebrew/homebrew-core).

### From source

```bash
go install github.com/moKshagna-p/lrc/cmd/lrc@latest
ln -sf "$(go env GOPATH)/bin/lrc" "$(go env GOPATH)/bin/lyrics"
```

## Usage

Simply run `lyrics` in your terminal while Apple Music or Spotify is playing a song. If both are playing, Apple Music is shown.
Press `Q` or `Ctrl+C` to quit.

## Acknowledgements
- The original version was prototyped with Python and `rich`.
- Lyrics provided via the excellent [lrclib.net](https://lrclib.net/) API.
- Artwork provided via the iTunes Search API.
