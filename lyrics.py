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

