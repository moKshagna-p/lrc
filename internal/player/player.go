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

// processCheckScript uses only System Events terminology, which is always
// available, so it compiles on every macOS install regardless of which apps
// are present. Never put app-specific terms (player state, current track, ...)
// in a script that also gets compiled on machines missing that app.
const processCheckScript = `
tell application "System Events"
    set musicRunning to (exists process "Music")
    set spotifyRunning to (exists process "Spotify")
    if musicRunning and spotifyRunning then return "BOTH"
    if musicRunning then return "MUSIC"
    if spotifyRunning then return "SPOTIFY"
    return "NONE"
end tell
`

// musicScript is safe to compile on every Mac: Music.app ships with macOS, so
// its scripting dictionary is always available. It is only run when the Music
// process exists.
const musicScript = `
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
return "Apple Music||NOT_PLAYING"
`

// spotifyScript references Spotify's scripting dictionary, so osascript can
// only compile it when the Spotify app is installed. It must therefore only be
// invoked when the process check reports Spotify is running (which implies the
// app and its dictionary exist). Compiling it otherwise produces a bogus
// "Expected then" syntax error and breaks the whole poll loop.
const spotifyScript = `
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
return "Spotify||NOT_PLAYING"
`

func runScript(script string) (string, error) {
	cmd := exec.Command("osascript", "-e", script)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

func pollOnce(run func(string) (string, error)) (string, error) {
	which, err := run(processCheckScript)
	if err != nil {
		return "", err
	}

	switch which {
	case "MUSIC":
		return run(musicScript)
	case "SPOTIFY":
		return run(spotifyScript)
	case "BOTH":
		musicResult, err := run(musicScript)
		if err != nil {
			return "", err
		}
		if strings.Contains(musicResult, "||PLAYING||") {
			return musicResult, nil
		}

		spotifyResult, err := run(spotifyScript)
		if err != nil {
			return "", err
		}
		if strings.Contains(spotifyResult, "||PLAYING||") {
			return spotifyResult, nil
		}
		return musicResult, nil
	default:
		return "NOT_RUNNING", nil
	}
}

// PlayerPollThread updates SharedState without blocking the UI. Apple Music takes
// precedence when both players are active; otherwise Spotify is used.
func PlayerPollThread() {
	for {
		res, err := pollOnce(runScript)
		if err != nil {
			SharedState.Update("ERROR", "", "", "", "", 0, 0)
		} else {
			applyPollResult(res)
		}
		time.Sleep(1500 * time.Millisecond)
	}
}

func applyPollResult(res string) {
	if res == "NOT_RUNNING" {
		SharedState.Update("NOT_RUNNING", "", "", "", "", 0, 0)
		return
	}
	parts := strings.Split(res, "||")
	if len(parts) >= 2 && parts[1] == "NOT_PLAYING" {
		SharedState.Update("NOT_PLAYING", parts[0], "", "", "", 0, 0)
		return
	}
	if len(parts) >= 7 && parts[1] == "PLAYING" {
		pos, posErr := strconv.ParseFloat(strings.ReplaceAll(parts[5], ",", "."), 64)
		dur, durErr := strconv.ParseFloat(strings.ReplaceAll(parts[6], ",", "."), 64)
		if posErr == nil && durErr == nil {
			SharedState.Update("PLAYING", parts[0], parts[2], parts[3], parts[4], pos, dur)
			return
		}
	}
	SharedState.Update("ERROR", "", "", "", "", 0, 0)
}

// AppleMusicPollThread is retained for compatibility with earlier callers.
func AppleMusicPollThread() { PlayerPollThread() }
