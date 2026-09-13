package api

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBestQuadrantPreservesVerticalColorBoundary(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}

	cell := bestQuadrant([4]color.RGBA{red, blue, red, blue})

	if cell.mask != 0b0101 {
		t.Fatalf("bestQuadrant mask = %04b, want 0101", cell.mask)
	}
	if cell.foreground != red {
		t.Fatalf("foreground = %#v, want %#v", cell.foreground, red)
	}
	if cell.background != blue {
		t.Fatalf("background = %#v, want %#v", cell.background, blue)
	}
}

func TestRenderArtworkUsesRequestedCellDimensions(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 60), A: 255})
		}
	}

	rendered := RenderArtwork(img, 5)
	lines := strings.Split(rendered, "\n")
	if len(lines) != 3 {
		t.Fatalf("RenderArtwork line count = %d, want 3", len(lines))
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != 5 {
			t.Fatalf("line %d width = %d, want 5", i, got)
		}
	}
}
