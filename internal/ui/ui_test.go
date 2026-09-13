package ui

import "testing"

func TestArtworkWidthUsesAvailableHeight(t *testing.T) {
	tests := []struct {
		name       string
		termWidth  int
		mainHeight int
		want       int
	}{
		{name: "normal terminal", termWidth: 120, mainHeight: 40, want: 30},
		{name: "caps wide terminal", termWidth: 400, mainHeight: 100, want: 60},
		{name: "short terminal", termWidth: 120, mainHeight: 10, want: 16},
		{name: "tiny height remains drawable", termWidth: 80, mainHeight: 2, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := artworkWidth(tt.termWidth, tt.mainHeight); got != tt.want {
				t.Fatalf("artworkWidth(%d, %d) = %d, want %d", tt.termWidth, tt.mainHeight, got, tt.want)
			}
		})
	}
}
