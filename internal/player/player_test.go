package player

import (
	"fmt"
	"testing"
)

func TestPollOnceFallsBackToPlayingSpotifyWhenMusicIsPaused(t *testing.T) {
	spotifyResult := "Spotify||PLAYING||Song||Artist||Album||12.5||180"
	run := func(script string) (string, error) {
		switch script {
		case processCheckScript:
			return "BOTH", nil
		case musicScript:
			return "Apple Music||NOT_PLAYING", nil
		case spotifyScript:
			return spotifyResult, nil
		default:
			return "", fmt.Errorf("unexpected script")
		}
	}

	got, err := pollOnce(run)
	if err != nil {
		t.Fatalf("pollOnce returned an error: %v", err)
	}
	if got != spotifyResult {
		t.Fatalf("pollOnce() = %q, want %q", got, spotifyResult)
	}
}
