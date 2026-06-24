# lrc — Live Lyrics in Your Terminal

**lrc** is a full-screen terminal UI that displays real-time synced lyrics for whatever song is playing in Apple Music — complete with album art, a progress bar, and ASCII-styled track headers.

## About

Apple Music shows lyrics inline, but this is a focused, immersive experience built for the terminal. It polls Apple Music via AppleScript, fetches synced lyrics from lrclib.net, downloads album artwork from iTunes, and renders everything in a polished `rich` TUI layout.

### Features

- **Synced lyrics** — highlights the current line, scrolls automatically as the song plays
- **Album art** — rendered in the terminal using Unicode half-block characters (`▀`)
- **Smooth progress bar** — interpolates between polls for butter-smooth ticking at 20 FPS
- **ASCII track header** — pyfiglet-generated song title
- **Ghostty / Kitty support** — optional native image protocol for true album art rendering (install `term-image`)
- **Instant song switching** — caches per track, background-fetches without UI stutter

### Tech Stack

| Component | Technology |
|---|---|
| UI framework | `rich` (Live, Layout, Panel, Text) |
| Lyrics API | [lrclib.net](https://lrclib.net) |
| Artwork API | iTunes Search API |
| Apple Music | AppleScript via `osascript` |
| Image rendering | `Pillow` + half-block chars / `term-image` (opt-in) |
| ASCII art | `pyfiglet` |

## Setup

```bash
pip3 install rich requests Pillow pyfiglet
chmod +x lyrics.py
```

Add an alias to your `~/.zshrc`:

```bash
alias lyrics="python3 /path/to/lrc/lyrics.py"
```

Then `source ~/.zshrc`.

## Usage

```bash
lyrics
```

Press `Q` or `Ctrl+C` to quit.

## Optional: Native Image Rendering (Ghostty / Kitty)

If you use Ghostty, Kitty, or any terminal that supports the Kitty image protocol:

```bash
pip3 install term-image
```

lrc auto-detects the library and renders album art as true images instead of half-block pixel art.
