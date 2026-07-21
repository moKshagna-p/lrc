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
	Source       string
	TrackName    string
	ArtistName   string
	AlbumName    string
	Position     float64
	Duration     float64
	LastSyncTime time.Time
	mu           sync.RWMutex
}

var SharedState = &PlayerState{Status: "NOT_RUNNING"}

func (p *PlayerState) Update(status, source, track, artist, album string, pos, dur float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	songChanged := p.Source != source || p.TrackName != track || p.ArtistName != artist

	p.Status = status
	p.Source = source
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

func (p *PlayerState) GetState() (status, source, track, artist, album string, dur float64) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Status, p.Source, p.TrackName, p.ArtistName, p.AlbumName, p.Duration
}

const pollScript = `
tell application "System Events"
    set musicRunning to (exists process "Music")
    set spotifyRunning to (exists process "Spotify")
end tell

if musicRunning then
    tell application "Music"
        if player state is playing then
            set t to name of current track
            set ar to artist of current track
            set al to album of current track
            set pos to player position
            set dur to duration of current track
            return "Apple Music||PLAYING||" & t & "||" & ar & "||" & al & "||" & pos & "||" & dur
        end if
    end tell
end if

if spotifyRunning then
    tell application "Spotify"
        if player state is playing then
            set t to name of current track
            set ar to artist of current track
            set al to album of current track
            set pos to player position / 1000
            set dur to duration of current track / 1000
            return "Spotify||PLAYING||" & t & "||" & ar & "||" & al & "||" & pos & "||" & dur
        end if
    end tell
end if

if musicRunning then return "Apple Music||NOT_PLAYING"
if spotifyRunning then return "Spotify||NOT_PLAYING"
return "NOT_RUNNING"
`

// PlayerPollThread updates SharedState without blocking the UI. Apple Music takes
// precedence when both players are active; otherwise Spotify is used.
func PlayerPollThread() {
	for {
		cmd := exec.Command("osascript", "-e", pollScript)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			SharedState.Update("ERROR", "", "", "", "", 0, 0)
		} else {
			res := strings.TrimSpace(out.String())
			if res == "NOT_RUNNING" {
				SharedState.Update("NOT_RUNNING", "", "", "", "", 0, 0)
			} else {
				parts := strings.Split(res, "||")
				if len(parts) >= 2 && parts[1] == "NOT_PLAYING" {
					SharedState.Update("NOT_PLAYING", parts[0], "", "", "", 0, 0)
				} else if len(parts) >= 7 && parts[1] == "PLAYING" {
					pos, posErr := strconv.ParseFloat(strings.ReplaceAll(parts[5], ",", "."), 64)
					dur, durErr := strconv.ParseFloat(strings.ReplaceAll(parts[6], ",", "."), 64)
					if posErr == nil && durErr == nil {
						SharedState.Update("PLAYING", parts[0], parts[2], parts[3], parts[4], pos, dur)
					} else {
						SharedState.Update("ERROR", "", "", "", "", 0, 0)
					}
				} else {
					SharedState.Update("ERROR", "", "", "", "", 0, 0)
				}
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}
}

// AppleMusicPollThread is retained for compatibility with earlier callers.
func AppleMusicPollThread() { PlayerPollThread() }
