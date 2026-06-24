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

