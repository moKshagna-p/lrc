# Lyrics Viewer for Apple Music

A beautiful, full-screen terminal UI for viewing synced lyrics of the currently playing Apple Music song. 
Built with Python and `rich`.

## Setup

1. Install the required dependencies:
   ```bash
   pip3 install rich requests
   ```

2. Make the script executable:
   ```bash
   chmod +x lyrics.py
   ```

3. Add this alias to your `~/.zshrc` (or `~/.bashrc`):
   ```bash
   alias lyrics="python3 /Users/mokshagna/Desktop/dev/projects/lrc/lyrics.py"
   ```

4. Reload your shell config:
   ```bash
   source ~/.zshrc
   ```

## Usage

Simply run `lyrics` in your terminal while Apple Music is playing a song!
Press `Q` or `Ctrl+C` to quit.
