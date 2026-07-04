package main

import (
	"github.com/moKshagna-p/lrc/internal/player"
	"github.com/moKshagna-p/lrc/internal/ui"
)

func main() {
	go player.AppleMusicPollThread()
	ui.Start()
}
