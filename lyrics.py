#!/usr/bin/env python3
"""
lyrics.py — A beautiful, full-screen lyrics viewer for Apple Music with cover art.
"""

import os
import sys
import time
import re
import subprocess
import threading
import select
import termios
import tty
from io import BytesIO
from typing import Optional, Dict, Any, List, Tuple

try:
    import requests
    from rich.live import Live
    from rich.layout import Layout
    from rich.panel import Panel
    from rich.text import Text
    from rich.align import Align
    from PIL import Image
    import pyfiglet
except ImportError:
    print("Please install required packages:")
    print("pip3 install rich requests Pillow pyfiglet")
    sys.exit(1)

# Global flag for exit
EXIT_FLAG = False

# Shared state for ultra-smooth UI
class PlayerState:
    def __init__(self):
        self.status = "NOT_RUNNING"
        self.track_name = ""
        self.artist_name = ""
        self.album_name = ""
        self.position = 0.0
        self.duration = 0.0
        self.last_sync_time = time.time()
        self.lock = threading.Lock()

    def update(self, data: Dict[str, Any]):
        with self.lock:
            self.status = data.get("status", "ERROR")
            if self.status == "PLAYING":
                self.track_name = data.get("track_name", "")
                self.artist_name = data.get("artist_name", "")
                self.album_name = data.get("album_name", "")
                self.position = data.get("position", 0.0)
                self.duration = data.get("duration", 0.0)
                self.last_sync_time = time.time()

    def get_smooth_position(self) -> float:
        with self.lock:
            if self.status == "PLAYING":
                # Interpolate based on system clock for butter-smooth 60fps-like timer
                elapsed = time.time() - self.last_sync_time
                return min(self.position + elapsed, self.duration)
            return self.position

shared_state = PlayerState()

def input_thread_func():
    global EXIT_FLAG
    fd = sys.stdin.fileno()
    try:
        old_settings = termios.tcgetattr(fd)
    except termios.error:
        return

    try:
        tty.setcbreak(fd)
        while not EXIT_FLAG:
            if select.select([sys.stdin], [], [], 0.1)[0]:
                char = sys.stdin.read(1)
                if char.lower() == 'q' or char == '\x03': # q or Ctrl+C
                    EXIT_FLAG = True
                    break
    except Exception:
        pass
    finally:
        termios.tcsetattr(fd, termios.TCSADRAIN, old_settings)

def apple_music_poll_thread():
    """Background thread to poll Apple Music without stuttering the UI."""
    global EXIT_FLAG
    script = '''
    tell application "System Events"
        if not (exists process "Music") then
            return "NOT_RUNNING"
        end if
    end tell
    tell application "Music"
        if player state is playing then
            set t to name of current track
            set ar to artist of current track
            set al to album of current track
            set pos to player position
            set dur to duration of current track
            return t & "||" & ar & "||" & al & "||" & pos & "||" & dur
        else
            return "NOT_PLAYING"
        end if
    end tell
    '''
    while not EXIT_FLAG:
        try:
            result = subprocess.run(['osascript', '-e', script], capture_output=True, text=True, check=True)
            out = result.stdout.strip()
            if out == "NOT_RUNNING":
                shared_state.update({"status": "NOT_RUNNING"})
            elif out == "NOT_PLAYING":
                shared_state.update({"status": "NOT_PLAYING"})
            else:
                parts = out.split("||")
                if len(parts) >= 5:
                    try:
                        pos = float(parts[3].replace(',', '.'))
                        dur = float(parts[4].replace(',', '.'))
                    except ValueError:
                        pos, dur = 0.0, 0.0
                    shared_state.update({
                        "status": "PLAYING",
                        "track_name": parts[0],
                        "artist_name": parts[1],
                        "album_name": parts[2],
                        "position": pos,
                        "duration": dur
                    })
                else:
                    shared_state.update({"status": "ERROR"})
        except Exception:
            shared_state.update({"status": "ERROR"})
            
        # Sleep for 1.5 seconds before syncing again; the UI interpolates the gaps!
        for _ in range(15):
            if EXIT_FLAG:
                break
            time.sleep(0.1)

def parse_lrc(lrc_text: str) -> List[Tuple[float, str]]:
    if not lrc_text:
        return []
    lines = []
    pattern = re.compile(r'\[(\d+):(\d+(?:\.\d+)?)\](.*)')
    for line in lrc_text.splitlines():
        match = pattern.search(line)
        if match:
            m = int(match.group(1))
            s = float(match.group(2))
            text = match.group(3).strip()
            time_sec = m * 60 + s
            lines.append((time_sec, text))
    return lines

def fetch_lyrics(artist: str, track: str, album: str) -> Dict[str, Any]:
    track_clean = re.sub(r'\(feat\..*?\)', '', track, flags=re.IGNORECASE).strip()
    url = "https://lrclib.net/api/get"
    params = {"artist_name": artist, "track_name": track_clean, "album_name": album}
    headers = {"User-Agent": "LyricsViewer/1.0 (https://github.com/moKshagna-p/lrc)"}
    
    for _ in range(3):
        try:
            resp = requests.get(url, params=params, headers=headers, timeout=20)
            if resp.status_code == 404:
                if "album_name" in params:
                    del params["album_name"]
                resp = requests.get(url, params=params, headers=headers, timeout=20)
                
            if resp.status_code == 200:
                data = resp.json()
                synced = data.get("syncedLyrics")
                plain = data.get("plainLyrics")
                return {
                    "found": True,
                    "synced": parse_lrc(synced) if synced else None,
                    "plain": plain
                }
            break 
        except Exception:
            time.sleep(0.5)
            continue
            
    return {"found": False, "synced": None, "plain": None}

def fetch_and_render_artwork(artist: str, album: str, track: str, width: int = 36) -> Optional[Text]:
    url = "https://itunes.apple.com/search"
    term = f"{artist} {track}"
    params = {"term": term, "media": "music", "entity": "song", "limit": 1}
    headers = {"User-Agent": "LyricsViewer/1.0 (https://github.com/moKshagna-p/lrc)"}
    
    for _ in range(3):
        try:
            resp = requests.get(url, params=params, headers=headers, timeout=5)
            if resp.status_code == 200:
                results = resp.json().get("results", [])
                if results:
                    art_url = results[0].get("artworkUrl100")
                    if art_url:
                        art_url = art_url.replace("100x100bb", "600x600bb")
                        img_resp = requests.get(art_url, timeout=5)
                        if img_resp.status_code == 200:
                            img = Image.open(BytesIO(img_resp.content)).convert("RGB")
                            img = img.resize((width, width), Image.Resampling.LANCZOS)
                            pixels = img.load()
                            
                            lines = []
                            for y in range(0, width, 2):
                                line = Text()
                                for x in range(width):
                                    top = pixels[x, y]
                                    bottom = pixels[x, y+1] if y+1 < width else (0,0,0)
                                    style = f"rgb({top[0]},{top[1]},{top[2]}) on rgb({bottom[0]},{bottom[1]},{bottom[2]})"
                                    line.append("▀", style=style)
                                lines.append(line)
                                
                            return Text("\n").join(lines)
            break
        except Exception:
            time.sleep(0.5)
            continue
            
    return None

def make_layout(artwork_size: int = 42) -> Layout:
    layout = Layout()
    layout.split_column(
        Layout(name="header", size=10),
        Layout(name="main", ratio=1),
        Layout(name="progress", size=3),
        Layout(name="footer", size=3)
    )
    
    layout["main"].split_row(
        Layout(name="artwork", size=artwork_size),
        Layout(name="lyrics", ratio=1)
    )
    return layout

def render_header() -> Panel:
    with shared_state.lock:
        track = shared_state.track_name or "Unknown"
        artist = shared_state.artist_name or "Unknown"
        album = shared_state.album_name or ""

    text = Text()
    try:
        # Use pyfiglet 'small' font for giant, readable terminal text
        fig = pyfiglet.Figlet(font='small', width=120)
        large_track = fig.renderText(track)
        # Clean up empty lines from ascii art
        large_track = "\n".join([line for line in large_track.split('\n') if line.strip("\r\n")])
        text.append(f"{large_track}\n", style="bold bright_white")
    except Exception:
        text.append(f"♫ {track} \n", style="bold white")
        
    text.append(f"✦ {artist} ✦", style="italic bright_cyan")
    if album:
        text.append(f"\n{album}", style="dim grey62")
        
    return Panel(Align.center(text, vertical="middle"), border_style="grey23")

def render_lyrics(lyrics_data: Dict[str, Any]) -> Panel:
    if not lyrics_data.get('found'):
        return Panel(Align.center(Text("No lyrics found for this song", style="dim white"), vertical="middle"), border_style="grey23")
    
    if lyrics_data.get('synced'):
        current_time = shared_state.get_smooth_position()
        lines = lyrics_data['synced']
        
        current_idx = -1
        for i, (t, _) in enumerate(lines):
            if current_time >= t:
                current_idx = i
            else:
                break
                
        lines_to_show = []
        # We'll use double spacing for an aesthetic, airy look.
        # Show 3 lines above and 3 lines below to fit nicely with double spacing.
        for i in range(current_idx - 3, current_idx + 4):
            if i < 0 or i >= len(lines):
                lines_to_show.append(Text(" "))
            else:
                _, line_text = lines[i]
                if not line_text.strip():
                    line_text = "✦ ✦ ✦"
                
                if i == current_idx:
                    t = Text(f"▶  {line_text}", style="bold italic bright_magenta")
                elif i in (current_idx - 1, current_idx + 1):
                    t = Text(f"   {line_text}", style="white")
                else:
                    t = Text(f"   {line_text}", style="dim grey62")
                lines_to_show.append(t)
            
            # Double spacing
            if i != current_idx + 3:
                lines_to_show.append(Text(" "))
        
        combined_text = Text("\n").join(lines_to_show)
        return Panel(Align.center(combined_text, vertical="middle"), border_style="grey23")
    else:
        plain_text = lyrics_data.get('plain') or ''
        return Panel(Align.center(Text(plain_text, style="white")), border_style="grey23")

def render_progress() -> Panel:
    pos = shared_state.get_smooth_position()
    with shared_state.lock:
        dur = shared_state.duration
    
    def format_time(seconds: float) -> str:
        if seconds < 0:
            seconds = 0
        m = int(seconds // 60)
        s = int(seconds % 60)
        # Adding fractions of a second gives it that smooth, fast-ticking aesthetic!
        ms = int((seconds % 1) * 10) 
        return f"{m}:{s:02d}.{ms}"
        
    pos_str = format_time(pos)
    dur_str = format_time(dur)
    
    ratio = pos / dur if dur > 0 else 0.0
    ratio = min(max(ratio, 0.0), 1.0)
    
    bar_width = 40
    filled = int(bar_width * ratio)
    empty = bar_width - filled
    
    bar_text = Text()
    bar_text.append("█" * filled, style="bright_magenta")
    bar_text.append("░" * empty, style="grey30")
    bar_text.append(f"  {pos_str} / {dur_str}", style="bright_white")
    
    return Panel(Align.center(bar_text, vertical="middle"), border_style="grey23")

def render_footer() -> Panel:
    return Panel(Align.center(Text("press Q to quit", style="dim grey50"), vertical="middle"), border_style="grey23")

def main():
    global EXIT_FLAG
    
    input_thread = threading.Thread(target=input_thread_func, daemon=True)
    input_thread.start()
    
    poll_thread = threading.Thread(target=apple_music_poll_thread, daemon=True)
    poll_thread.start()
    
    term_cols = os.get_terminal_size().columns
    artwork_width = max(20, min(term_cols // 4, 60))
    layout = make_layout(artwork_size=artwork_width + 6)
    cache: Dict[str, Any] = {}
    current_song_key: Optional[str] = None
    
    try:
        # High refresh rate for ultra-smooth UI animations (20 FPS)
        with Live(layout, screen=True, refresh_per_second=20):
            while not EXIT_FLAG:
                with shared_state.lock:
                    status = shared_state.status
                    track = shared_state.track_name
                    artist = shared_state.artist_name
                    album = shared_state.album_name
                
                if status == "NOT_RUNNING":
                    layout["lyrics"].update(Panel(Align.center(Text("Apple Music is not running", style="dim white"), vertical="middle"), border_style="grey23"))
                    layout["artwork"].update(Panel(Text(""), border_style="grey23"))
                    layout["header"].update(Panel(Text(""), border_style="grey23"))
                    layout["progress"].update(Panel(Text(""), border_style="grey23"))
                elif status == "NOT_PLAYING":
                    layout["lyrics"].update(Panel(Align.center(Text("Nothing playing in Apple Music ♫", style="dim white"), vertical="middle"), border_style="grey23"))
                    layout["artwork"].update(Panel(Text(""), border_style="grey23"))
                    layout["header"].update(Panel(Text(""), border_style="grey23"))
                    layout["progress"].update(Panel(Text(""), border_style="grey23"))
                elif status == "PLAYING":
                    key = f"{artist}-{track}"
                    if key != current_song_key:
                        current_song_key = key
                        if key not in cache:
                            # Set initial loading state so the UI responds immediately
                            cache[key] = {
                                "lyrics": {"found": True, "synced": None, "plain": "Loading lyrics..."},
                                "artwork": None
                            }
                            
                            # Spawn background thread to fetch data
                            def background_fetch(k, a, t, al, aw):
                                try:
                                    lyr = fetch_lyrics(a, t, al)
                                    cache[k] = {
                                        "lyrics": lyr,
                                        "artwork": cache.get(k, {}).get("artwork")
                                    }
                                except Exception:
                                    pass
                                try:
                                    art = fetch_and_render_artwork(a, al, t, width=aw)
                                    if k in cache:
                                        cache[k]["artwork"] = art
                                except Exception:
                                    pass
                                    
                            threading.Thread(target=background_fetch, args=(key, artist, track, album, artwork_width), daemon=True).start()
                            
                    lyrics_data = cache[key]["lyrics"]
                    artwork_text = cache[key]["artwork"]
                    
                    layout["header"].update(render_header())
                    layout["lyrics"].update(render_lyrics(lyrics_data))
                    
                    if artwork_text:
                        layout["artwork"].update(Panel(Align.center(artwork_text, vertical="middle"), border_style="grey23"))
                    else:
                        layout["artwork"].update(Panel(Align.center(Text("No Artwork Found", style="dim grey50"), vertical="middle"), border_style="grey23"))
                        
                    layout["progress"].update(render_progress())
                else:
                    layout["lyrics"].update(Panel(Align.center(Text("Error connecting to Apple Music", style="red"), vertical="middle"), border_style="grey23"))
                
                layout["footer"].update(render_footer())
                
                # Sleep briefly in main thread, updates happen cleanly via threads
                time.sleep(0.05)
                    
    except KeyboardInterrupt:
        EXIT_FLAG = True

if __name__ == "__main__":
    main()
