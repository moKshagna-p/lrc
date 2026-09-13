package api

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/image/draw"
)

type LyricLine struct {
	Time float64
	Text string
}

type LyricsData struct {
	Found  bool
	Synced []LyricLine
	Plain  string
}

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

func FetchLyrics(artist, track, album string) *LyricsData {
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

type quadrantCell struct {
	mask       uint8
	foreground color.RGBA
	background color.RGBA
}

var quadrantRunes = [16]rune{
	' ', '▘', '▝', '▀',
	'▖', '▌', '▞', '▛',
	'▗', '▚', '▐', '▜',
	'▄', '▙', '▟', '█',
}

// bestQuadrant reduces four source pixels to the two colors a terminal cell
// can display. Trying every unique two-color partition retains diagonal and
// vertical detail that an upper-half-block renderer necessarily discards.
func bestQuadrant(pixels [4]color.RGBA) quadrantCell {
	best := quadrantCell{mask: 1}
	bestError := int64(^uint64(0) >> 1)

	// A mask and its inverse describe the same cell with foreground and
	// background swapped, so masks 1..7 cover every unique partition.
	for mask := uint8(1); mask < 8; mask++ {
		foreground := averagePixels(pixels, mask, true)
		background := averagePixels(pixels, mask, false)
		errorValue := colorError(pixels, mask, foreground, background)
		if errorValue < bestError {
			bestError = errorValue
			best = quadrantCell{mask: mask, foreground: foreground, background: background}
		}
	}
	return best
}

func averagePixels(pixels [4]color.RGBA, mask uint8, foreground bool) color.RGBA {
	var r, g, b, count int
	for i, pixel := range pixels {
		selected := mask&(1<<i) != 0
		if selected != foreground {
			continue
		}
		r += int(pixel.R)
		g += int(pixel.G)
		b += int(pixel.B)
		count++
	}
	return color.RGBA{R: uint8(r / count), G: uint8(g / count), B: uint8(b / count), A: 255}
}

func colorError(pixels [4]color.RGBA, mask uint8, foreground, background color.RGBA) int64 {
	var total int64
	for i, pixel := range pixels {
		candidate := background
		if mask&(1<<i) != 0 {
			candidate = foreground
		}
		dr := int64(pixel.R) - int64(candidate.R)
		dg := int64(pixel.G) - int64(candidate.G)
		db := int64(pixel.B) - int64(candidate.B)
		total += dr*dr + dg*dg + db*db
	}
	return total
}

func FetchArtwork(artist, album, track string) image.Image {
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

func RenderArtwork(img image.Image, width int) string {
	if img == nil || width <= 0 {
		return ""
	}

	cellHeight := (width + 1) / 2
	dst := image.NewRGBA(image.Rect(0, 0, width*2, cellHeight*2))
	draw.CatmullRom.Scale(dst, dst.Rect, img, img.Bounds(), draw.Over, nil)

	lines := make([]string, 0, cellHeight)
	for cellY := 0; cellY < cellHeight; cellY++ {
		var line strings.Builder
		for cellX := 0; cellX < width; cellX++ {
			x := cellX * 2
			y := cellY * 2
			pixels := [4]color.RGBA{
				color.RGBAModel.Convert(dst.At(x, y)).(color.RGBA),
				color.RGBAModel.Convert(dst.At(x+1, y)).(color.RGBA),
				color.RGBAModel.Convert(dst.At(x, y+1)).(color.RGBA),
				color.RGBAModel.Convert(dst.At(x+1, y+1)).(color.RGBA),
			}
			cell := bestQuadrant(pixels)
			style := lipgloss.NewStyle().
				Foreground(lipgloss.Color(rgbToHex(cell.foreground))).
				Background(lipgloss.Color(rgbToHex(cell.background)))
			line.WriteString(style.Render(string(quadrantRunes[cell.mask])))
		}
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}
