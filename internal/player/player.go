package player

import (
	"bytes"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PlayerState struct {
	Status       string
	TrackName    string
	ArtistName   string
	AlbumName    string
	Position     float64
	Duration     float64
	LastSyncTime time.Time
	mu           sync.RWMutex
}

var SharedState = &PlayerState{Status: "NOT_RUNNING"}

func (p *PlayerState) Update(status, track, artist, album string, pos, dur float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	songChanged := p.TrackName != track || p.ArtistName != artist

	p.Status = status
	if status == "PLAYING" {
		p.TrackName = track
		p.ArtistName = artist
		p.AlbumName = album
		p.Duration = dur

		currentSmoothed := p.Position
		if !songChanged && !p.LastSyncTime.IsZero() {
			currentSmoothed += time.Since(p.LastSyncTime).Seconds()
		}

		if !songChanged && pos < currentSmoothed && (currentSmoothed-pos) < 3.0 {
			p.Position = currentSmoothed
		} else {
			p.Position = pos
		}

		p.LastSyncTime = time.Now()
	}
}

func (p *PlayerState) GetSmoothPosition() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.Status == "PLAYING" {
		elapsed := time.Since(p.LastSyncTime).Seconds()
		return math.Min(p.Position+elapsed, p.Duration)
	}
	return p.Position
}

func (p *PlayerState) GetState() (status, track, artist, album string, dur float64) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Status, p.TrackName, p.ArtistName, p.AlbumName, p.Duration
}

func AppleMusicPollThread() {
	script := `
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
    `
	for {
		cmd := exec.Command("osascript", "-e", script)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			SharedState.Update("ERROR", "", "", "", 0, 0)
		} else {
			res := strings.TrimSpace(out.String())
			if res == "NOT_RUNNING" {
				SharedState.Update("NOT_RUNNING", "", "", "", 0, 0)
			} else if res == "NOT_PLAYING" {
				SharedState.Update("NOT_PLAYING", "", "", "", 0, 0)
			} else {
				parts := strings.Split(res, "||")
				if len(parts) >= 5 {
					pos, _ := strconv.ParseFloat(strings.ReplaceAll(parts[3], ",", "."), 64)
					dur, _ := strconv.ParseFloat(strings.ReplaceAll(parts[4], ",", "."), 64)
					SharedState.Update("PLAYING", parts[0], parts[1], parts[2], pos, dur)
				} else {
					SharedState.Update("ERROR", "", "", "", 0, 0)
				}
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}
}
