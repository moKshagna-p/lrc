#!/usr/bin/env python3
"""
lyrics.py — A beautiful, full-screen lyrics viewer for Apple Music with cover art.
"""

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

