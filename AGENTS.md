# LRC Agent Guidelines

This document serves as a guide for AI agents/LLMs working on the `lrc` codebase to ensure that the core logic, rendering mechanisms, and UI aesthetics are preserved during refactors or migrations (including any potential Go rewrite).

## Core Architecture & State Management
- **Media Polling**: The application queries Apple Music state via macOS `osascript` (AppleScript). It expects continuous polling to retrieve track name, artist, album, duration, and current playback position.
- **Shared State**: The player's state is maintained centrally and updated asynchronously by a background thread. The UI thread reads from this state to avoid blocking the main rendering loop.
- **Smooth Interpolation**: Apple Music polling is relatively slow. To maintain a 60fps-like smooth progress bar and lyric scrolling, the playback position is interpolated using the system clock (`time.time() - last_sync_time`) between AppleScript updates. This logic **must** be preserved.

## UI & Layout (Strict Guidelines)
The UI is built with a specific aesthetic that must be maintained:
- **Layout**: A 4-section vertical split:
  1. Header (size=10)
  2. Main body (ratio=1) - horizontally split into Artwork (left) and Lyrics (right)
  3. Progress Bar (size=3)
  4. Footer (size=3)
- **Colors**:
  - Track Name: Bold bright white.
  - Artist: Italic bright cyan.
  - Album: Dim grey62.
  - Active Lyric: Bold italic bright magenta with a `▶` prefix.
  - Surrounding Lyrics: Pure white for immediate neighbors, dim grey62 for outer lyrics.
  - Progress Bar: Bright magenta for filled `█`, grey30 for empty `░`.
- **Large Text**: The track name uses `pyfiglet` (small font) for giant ASCII art text. 
- **Double Spacing**: The lyrics panel uses double spacing for an "airy" and readable look.

## Image Processing & Artwork
- **Fetching**: Artwork is fetched from the iTunes API (`itunes.apple.com/search`).
- **Rendering**: The raw image is downloaded, resized, and converted into terminal block characters (`▀`). 
- **Block Colors**: Each character uses the top pixel for the foreground (`fg`) and the bottom pixel for the background (`bg`). E.g., `rgb(r,g,b) on rgb(r,g,b)`. 

## Future Changes
- When refactoring (e.g., to Python modules or rewriting in Go), ensure background tasks (polling, network requests) never block the UI thread.
- The UI must remain responsive to keyboard inputs (like `q` or `Ctrl+C`).
