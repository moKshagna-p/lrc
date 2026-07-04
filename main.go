package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lukesampson/figlet/figletlib"
	"golang.org/x/image/draw"
)

// Shared State
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

		// Smooth out laggy osascript updates by preventing small backward jumps
		currentSmoothed := p.Position
		if !songChanged && !p.LastSyncTime.IsZero() {
			currentSmoothed += time.Since(p.LastSyncTime).Seconds()
		}

		// If osascript's position is slightly behind our interpolated position,
		// use the interpolated one to prevent the bar from "ticking" backwards.
		// If the difference is large (>3 seconds), the user probably scrubbed the track.
		if !songChanged && pos < currentSmoothed && (currentSmoothed - pos) < 3.0 {
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

var sharedState = &PlayerState{Status: "NOT_RUNNING"}

// Polling Thread
func appleMusicPollThread() {
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
			sharedState.Update("ERROR", "", "", "", 0, 0)
		} else {
			res := strings.TrimSpace(out.String())
			if res == "NOT_RUNNING" {
				sharedState.Update("NOT_RUNNING", "", "", "", 0, 0)
			} else if res == "NOT_PLAYING" {
				sharedState.Update("NOT_PLAYING", "", "", "", 0, 0)
			} else {
				parts := strings.Split(res, "||")
				if len(parts) >= 5 {
					pos, _ := strconv.ParseFloat(strings.ReplaceAll(parts[3], ",", "."), 64)
					dur, _ := strconv.ParseFloat(strings.ReplaceAll(parts[4], ",", "."), 64)
					sharedState.Update("PLAYING", parts[0], parts[1], parts[2], pos, dur)
				} else {
					sharedState.Update("ERROR", "", "", "", 0, 0)
				}
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}
}

// Data Types
type LyricLine struct {
	Time float64
	Text string
}

type LyricsData struct {
	Found  bool
	Synced []LyricLine
	Plain  string
}

type CacheEntry struct {
	Lyrics       *LyricsData
	ArtworkText  string
	ArtworkWidth int
	RawArtwork   image.Image
}

var (
	cache   = make(map[string]*CacheEntry)
	cacheMu sync.Mutex
)

// API Functions
func parseLRC(lrcText string) []LyricLine {
	var lines []LyricLine
	re := regexp.MustCompile(`\[(\d+):(\d+(?:\.\d+)?)\](.*)`)
	for _, line := range strings.Split(lrcText, "\n") {
		matches := re.FindStringSubmatch(line)
		if len(matches) == 4 {
			m, _ := strconv.ParseFloat(matches[1], 64)
			s, _ := strconv.ParseFloat(matches[2], 64)
			text := strings.TrimSpace(matches[3])
			lines = append(lines, LyricLine{Time: m*60 + s, Text: text})
		}
	}
	return lines
}

func fetchLyrics(artist, track, album string) *LyricsData {
	re := regexp.MustCompile(`(?i)\(feat\..*?\)`)
	trackClean := strings.TrimSpace(re.ReplaceAllString(track, ""))
	
	client := &http.Client{Timeout: 10 * time.Second}
	
	reqURL, _ := url.Parse("https://lrclib.net/api/get")
	q := reqURL.Query()
	q.Set("artist_name", artist)
	q.Set("track_name", trackClean)
	q.Set("album_name", album)
	reqURL.RawQuery = q.Encode()

	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", reqURL.String(), nil)
		req.Header.Set("User-Agent", "LyricsViewer/1.0 (https://github.com/moKshagna-p/lrc)")
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		
		if resp.StatusCode == 404 {
			q.Del("album_name")
			reqURL.RawQuery = q.Encode()
			req, _ = http.NewRequest("GET", reqURL.String(), nil)
			req.Header.Set("User-Agent", "LyricsViewer/1.0")
			resp, err = client.Do(req)
		}
		
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			var data struct {
				SyncedLyrics string `json:"syncedLyrics"`
				PlainLyrics  string `json:"plainLyrics"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&data); err == nil {
				return &LyricsData{
					Found:  true,
					Synced: parseLRC(data.SyncedLyrics),
					Plain:  data.PlainLyrics,
				}
			}
			return &LyricsData{Found: false}
		}
		if resp != nil {
			resp.Body.Close()
		}
		break
	}
	return &LyricsData{Found: false}
}

func rgbToHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

func fetchArtwork(artist, album, track string) image.Image {
	client := &http.Client{Timeout: 5 * time.Second}
	
	search := func(term, entity string) string {
		reqURL, _ := url.Parse("https://itunes.apple.com/search")
		q := reqURL.Query()
		q.Set("term", term)
		q.Set("media", "music")
		q.Set("entity", entity)
		q.Set("limit", "5")
		reqURL.RawQuery = q.Encode()

		req, _ := http.NewRequest("GET", reqURL.String(), nil)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			return ""
		}
		defer resp.Body.Close()

		var data struct {
			Results []struct {
				ArtworkUrl100 string `json:"artworkUrl100"`
			} `json:"results"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&data); err == nil && len(data.Results) > 0 {
			for _, r := range data.Results {
				if r.ArtworkUrl100 != "" {
					return strings.ReplaceAll(r.ArtworkUrl100, "100x100bb", "600x600bb")
				}
			}
		}
		return ""
	}

	imgURL := search(artist+" "+album, "album")
	if imgURL == "" {
		imgURL = search(artist+" "+track, "song")
	}
	if imgURL == "" {
		return nil
	}

	resp, err := client.Get(imgURL)
	if err != nil || resp.StatusCode != 200 {
		return nil
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil
	}
	return img
}

func renderArtwork(img image.Image, width int) string {
	if img == nil {
		return ""
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, width))
	draw.CatmullRom.Scale(dst, dst.Rect, img, img.Bounds(), draw.Over, nil)

	var lines []string
	for y := 0; y < width; y += 2 {
		var line string
		for x := 0; x < width; x++ {
			top := dst.At(x, y)
			var bottom color.Color = color.RGBA{0, 0, 0, 255}
			if y+1 < width {
				bottom = dst.At(x, y+1)
			}
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(rgbToHex(top))).Background(lipgloss.Color(rgbToHex(bottom)))
			line += style.Render("▀")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// Bubble Tea Model
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/20, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type dataFetchedMsg struct {
	Key        string
	Lyrics     *LyricsData
	RawArtwork image.Image
}

type model struct {
	width          int
	height         int
	currentSongKey string
	figletFont     *figletlib.Font
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tickCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		awWidth := m.width / 4
		if awWidth < 20 { awWidth = 20 }
		if awWidth > 60 { awWidth = 60 }
		cacheMu.Lock()
		for _, entry := range cache {
			if entry.RawArtwork != nil && entry.ArtworkWidth != awWidth {
				entry.ArtworkText = renderArtwork(entry.RawArtwork, awWidth)
				entry.ArtworkWidth = awWidth
			}
		}
		cacheMu.Unlock()
	case tickMsg:
		var fetchCmd tea.Cmd
		sharedState.mu.RLock()
		status := sharedState.Status
		track := sharedState.TrackName
		artist := sharedState.ArtistName
		album := sharedState.AlbumName
		sharedState.mu.RUnlock()

		if status == "PLAYING" {
			key := artist + "-" + track
			if key != m.currentSongKey {
				m.currentSongKey = key
				cacheMu.Lock()
				if _, ok := cache[key]; !ok {
					cache[key] = &CacheEntry{
						Lyrics: &LyricsData{Found: true, Plain: "Loading lyrics..."},
					}
					// Background fetch
					fetchCmd = func() tea.Msg {
						lyr := fetchLyrics(artist, track, album)
						art := fetchArtwork(artist, album, track)
						return dataFetchedMsg{Key: key, Lyrics: lyr, RawArtwork: art}
					}
				}
				cacheMu.Unlock()
			}
		}

		if fetchCmd != nil {
			return m, tea.Batch(tickCmd(), fetchCmd)
		}
		return m, tickCmd()

	case dataFetchedMsg:
		cacheMu.Lock()
		if entry, ok := cache[msg.Key]; ok {
			entry.Lyrics = msg.Lyrics
			if msg.RawArtwork != nil {
				entry.RawArtwork = msg.RawArtwork
				awWidth := m.width / 4
				if awWidth < 20 { awWidth = 20 }
				if awWidth > 60 { awWidth = 60 }
				entry.ArtworkText = renderArtwork(msg.RawArtwork, awWidth)
				entry.ArtworkWidth = awWidth
			}
		}
		cacheMu.Unlock()
	}
	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}

	sharedState.mu.RLock()
	status := sharedState.Status
	track := sharedState.TrackName
	artist := sharedState.ArtistName
	album := sharedState.AlbumName
	dur := sharedState.Duration
	sharedState.mu.RUnlock()
	
	pos := sharedState.GetSmoothPosition()

	// Styles matching the Python version
	boxStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("237")).Align(lipgloss.Center)
	
	// Layout dimensions
	headerH := 8
	progH := 3
	footerH := 3
	mainH := m.height - headerH - progH - footerH
	if mainH < 10 { mainH = 10 }

	if status == "NOT_RUNNING" {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, "Apple Music is not running")
	} else if status == "NOT_PLAYING" {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, "Nothing playing in Apple Music ♫")
	} else if status == "ERROR" {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("Error connecting to Apple Music"))
	}

	// HEADER
	headerText := ""
	if m.figletFont != nil {
		fWidth := m.width - 4
		if fWidth < 20 { fWidth = 20 }
		fStr := figletlib.SprintMsg(track, m.figletFont, fWidth, m.figletFont.Settings(), "left")
		// Clean up empty lines
		lines := strings.Split(fStr, "\n")
		var cl []string
		for _, l := range lines {
			if strings.TrimSpace(l) != "" { cl = append(cl, l) }
		}
		headerText += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Render(strings.Join(cl, "\n")) + "\n"
	} else {
		headerText += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Render("♫ "+track+" \n")
	}
	headerText += lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("14")).Render("✦ "+artist+" ✦")
	if album != "" {
		headerText += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(album)
	}
	headerView := boxStyle.Copy().Width(m.width - 2).Height(headerH - 2).Render(headerText)

	// MAIN (Artwork + Lyrics)
	cacheMu.Lock()
	entry := cache[m.currentSongKey]
	cacheMu.Unlock()

	awWidth := m.width / 4
	if awWidth < 20 { awWidth = 20 }
	if awWidth > 60 { awWidth = 60 }
	artView := boxStyle.Copy().Width(awWidth + 6).Height(mainH - 2).Render(lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("No Artwork Found"))
	
	var lyrText string
	if entry != nil && entry.ArtworkText != "" {
		artView = boxStyle.Copy().Width(awWidth + 6).Height(mainH - 2).Render(entry.ArtworkText)
	}

	if entry == nil || entry.Lyrics == nil || !entry.Lyrics.Found {
		lyrText = lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render("No lyrics found for this song")
	} else if len(entry.Lyrics.Synced) > 0 {
		lines := entry.Lyrics.Synced
		currentIdx := -1
		for i, l := range lines {
			if pos >= l.Time {
				currentIdx = i
			} else {
				break
			}
		}

		visibleLines := (mainH - 4) / 2
		if visibleLines < 1 { visibleLines = 1 }
		var linesToShow []string
		endIdx := currentIdx + (visibleLines - visibleLines/2)
		for i := currentIdx - visibleLines/2; i <= endIdx; i++ {
			if i < 0 || i >= len(lines) {
				linesToShow = append(linesToShow, " ")
			} else {
				txt := lines[i].Text
				if strings.TrimSpace(txt) == "" { txt = "✦ ✦ ✦" }
				if i == currentIdx {
					linesToShow = append(linesToShow, lipgloss.NewStyle().Bold(true).Italic(true).Foreground(lipgloss.Color("13")).Render("▶  "+txt))
				} else if i == currentIdx-1 || i == currentIdx+1 {
					linesToShow = append(linesToShow, lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Render("   "+txt))
				} else {
					linesToShow = append(linesToShow, lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render("   "+txt))
				}
			}
			if i != endIdx {
				linesToShow = append(linesToShow, " ") // Double spacing
			}
		}
		lyrText = strings.Join(linesToShow, "\n")
	} else {
		lyrText = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Render(entry.Lyrics.Plain)
	}
	lyrView := boxStyle.Copy().Width(m.width - awWidth - 10).Height(mainH - 2).Render(lyrText)
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, artView, lyrView)

	// PROGRESS
	formatTime := func(s float64) string {
		if s < 0 { s = 0 }
		m := int(s / 60)
		sec := int(math.Mod(s, 60))
		ms := int(math.Mod(s, 1) * 10)
		return fmt.Sprintf("%d:%02d.%d", m, sec, ms)
	}
	posStr := formatTime(pos)
	durStr := formatTime(dur)
	ratio := 0.0
	if dur > 0 { ratio = pos / dur }
	if ratio < 0 { ratio = 0 }
	if ratio > 1 { ratio = 1 }

	barW := m.width - 30
	if barW < 10 { barW = 10 }
	
	exactFilled := float64(barW) * ratio
	filled := int(exactFilled)
	remainder := exactFilled - float64(filled)

	fractions := []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	fracIndex := int(remainder * 8)
	if fracIndex < 0 { fracIndex = 0 }
	if fracIndex > 7 { fracIndex = 7 }

	barText := lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Render(strings.Repeat("█", filled))
	
	emptyStart := filled
	if fracIndex > 0 && filled < barW {
		barText += lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Render(fractions[fracIndex])
		emptyStart++
	}

	empty := barW - emptyStart
	if empty > 0 {
		barText += lipgloss.NewStyle().Foreground(lipgloss.Color("239")).Render(strings.Repeat("░", empty))
	}
	
	bar := barText + lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Render(fmt.Sprintf("  %s / %s", posStr, durStr))
	progView := boxStyle.Copy().Width(m.width - 2).Height(progH - 2).Render(bar)

	// FOOTER
	footText := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("press Q to quit")
	footView := boxStyle.Copy().Width(m.width - 2).Height(footerH - 2).Render(footText)

	return lipgloss.JoinVertical(lipgloss.Left, headerView, mainView, progView, footView)
}

func main() {
	go appleMusicPollThread()

	// Try to load figlet font, but fallback gracefully if missing
	fontURL := "https://raw.githubusercontent.com/xero/figlet-fonts/master/Small.flf"
	var font *figletlib.Font
	resp, err := http.Get(fontURL)
	if err == nil && resp.StatusCode == 200 {
		b, _ := io.ReadAll(resp.Body)
		f, err := figletlib.ReadFontFromBytes(b)
		if err == nil {
			font = f
		}
		resp.Body.Close()
	}

	m := model{figletFont: font}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}
