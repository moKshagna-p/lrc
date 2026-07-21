package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/moKshagna-p/lrc/internal/player"
	"github.com/moKshagna-p/lrc/internal/ui"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.BoolVar(showVersion, "v", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: lyrics [--version]")
		fmt.Fprintln(os.Stderr, "Shows synced lyrics for the currently playing Apple Music or Spotify track.")
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	go player.PlayerPollThread()
	ui.Start()
}
