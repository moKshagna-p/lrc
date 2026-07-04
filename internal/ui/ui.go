package ui

import (
	"fmt"
	"image"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/moKshagna-p/lrc/internal/api"
	"github.com/moKshagna-p/lrc/internal/player"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lukesampson/figlet/figletlib"
)

type CacheEntry struct {
	Lyrics       *api.LyricsData
	ArtworkText  string
	ArtworkWidth int
	RawArtwork   image.Image
}

var (
	cache   = make(map[string]*CacheEntry)
	cacheMu sync.Mutex
)

// Bubble Tea Model
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/20, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type dataFetchedMsg struct {
	Key        string
	Lyrics     *api.LyricsData
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
		if awWidth < 20 {
			awWidth = 20
		}
		if awWidth > 60 {
			awWidth = 60
		}
		cacheMu.Lock()
		for _, entry := range cache {
			if entry.RawArtwork != nil && entry.ArtworkWidth != awWidth {
				entry.ArtworkText = api.RenderArtwork(entry.RawArtwork, awWidth)
				entry.ArtworkWidth = awWidth
			}
		}
		cacheMu.Unlock()
	case tickMsg:
		var fetchCmd tea.Cmd
		status, track, artist, album, _ := player.SharedState.GetState()

		if status == "PLAYING" {
			key := artist + "-" + track
			if key != m.currentSongKey {
				m.currentSongKey = key
				cacheMu.Lock()
				if _, ok := cache[key]; !ok {
					cache[key] = &CacheEntry{
						Lyrics: &api.LyricsData{Found: true, Plain: "Loading lyrics..."},
					}
					// Background fetch
					fetchCmd = func() tea.Msg {
						lyr := api.FetchLyrics(artist, track, album)
						art := api.FetchArtwork(artist, album, track)
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
				if awWidth < 20 {
					awWidth = 20
				}
				if awWidth > 60 {
					awWidth = 60
				}
				entry.ArtworkText = api.RenderArtwork(msg.RawArtwork, awWidth)
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

	status, track, artist, album, dur := player.SharedState.GetState()
	pos := player.SharedState.GetSmoothPosition()

	// Styles matching the Python version
	boxStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("237")).Align(lipgloss.Center)

	// Layout dimensions
	headerH := 8
	progH := 3
	footerH := 3
	mainH := m.height - headerH - progH - footerH
	if mainH < 10 {
		mainH = 10
	}

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
		if fWidth < 20 {
			fWidth = 20
		}
		fStr := figletlib.SprintMsg(track, m.figletFont, fWidth, m.figletFont.Settings(), "left")
		// Clean up empty lines
		lines := strings.Split(fStr, "\n")
		var cl []string
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				cl = append(cl, l)
			}
		}
		headerText += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Render(strings.Join(cl, "\n")) + "\n"
	} else {
		headerText += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Render("♫ " + track + " \n")
	}
	headerText += lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("14")).Render("✦ " + artist + " ✦")
	if album != "" {
		headerText += "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Render(album)
	}
	headerView := boxStyle.Copy().Width(m.width - 2).Height(headerH - 2).Render(headerText)

	// MAIN (Artwork + Lyrics)
	cacheMu.Lock()
	entry := cache[m.currentSongKey]
	cacheMu.Unlock()

	awWidth := m.width / 4
	if awWidth < 20 {
		awWidth = 20
	}
	if awWidth > 60 {
		awWidth = 60
	}
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
		if visibleLines < 1 {
			visibleLines = 1
		}
		var linesToShow []string
		endIdx := currentIdx + (visibleLines - visibleLines/2)
		for i := currentIdx - visibleLines/2; i <= endIdx; i++ {
			if i < 0 || i >= len(lines) {
				linesToShow = append(linesToShow, " ")
			} else {
				txt := lines[i].Text
				if strings.TrimSpace(txt) == "" {
					txt = "✦ ✦ ✦"
				}
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
		if s < 0 {
			s = 0
		}
		m := int(s / 60)
		sec := int(math.Mod(s, 60))
		ms := int(math.Mod(s, 1) * 10)
		return fmt.Sprintf("%d:%02d.%d", m, sec, ms)
	}
	posStr := formatTime(pos)
	durStr := formatTime(dur)
	ratio := 0.0
	if dur > 0 {
		ratio = pos / dur
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	barW := m.width - 30
	if barW < 10 {
		barW = 10
	}

	exactFilled := float64(barW) * ratio
	filled := int(exactFilled)
	remainder := exactFilled - float64(filled)

	fractions := []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	fracIndex := int(remainder * 8)
	if fracIndex < 0 {
		fracIndex = 0
	}
	if fracIndex > 7 {
		fracIndex = 7
	}

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

func Start() {
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
