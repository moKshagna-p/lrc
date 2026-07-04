# Lyrics Viewer for Apple Music

A beautiful, ultra-smooth, full-screen terminal UI for viewing synced lyrics of the currently playing Apple Music song. 
Built with **Go**, [Bubble Tea](https://github.com/charmbracelet/bubbletea), and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Features
- **Live Syncing**: Lyrics automatically scroll along with Apple Music.
- **Ultra-Smooth Progress Bar**: Interpolates time and utilizes fractional Unicode blocks (8x resolution) for a continuously gliding progress bar.
- **Responsive Design**: The album artwork, lyrics view, and layout automatically adapt dynamically to your terminal window size.
- **Album Artwork**: Fetches and renders high-quality album art directly in your terminal using ANSI block characters.

## Setup

1. Make sure you have [Go](https://go.dev/) installed.

2. Clone this repository and build the binary:
   ```bash
   git clone https://github.com/moKshagna-p/lrc.git
   cd lrc
   go build -o lrc_go ./cmd/lrc
   ```

3. Add this alias to your `~/.zshrc` (or `~/.bashrc`) so you can run it from anywhere:
   ```bash
   alias lyrics="/path/to/lrc/lrc_go"
   ```
   *(Make sure to replace `/path/to/lrc` with the actual absolute path where you cloned the repo)*

4. Reload your shell config:
   ```bash
   source ~/.zshrc
   ```

## Usage

Simply run `lyrics` in your terminal while Apple Music is playing a song!
Press `Q` or `Ctrl+C` to quit.

## Acknowledgements
- The original version was prototyped with Python and `rich`.
- Lyrics provided via the excellent [lrclib.net](https://lrclib.net/) API.
- Artwork provided via the iTunes Search API.
