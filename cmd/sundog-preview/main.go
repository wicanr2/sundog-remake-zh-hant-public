package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/render"
)

func main() {
	outPath := flag.String("out", "workplace/preview/sundog-themes.png", "PNG preview output")
	atariAtlasManifest := flag.String("atari-atlas-manifest", "", "optional normalized Atari ST PNG atlas manifest")
	spaceScene := flag.Bool("space-scene", false, "輸出 Apple II／Atari ST／Modern Apple 同狀態太空場景 prototype")
	flag.Parse()

	reducer, err := game.NewReducer()
	if err != nil {
		fatal(err)
	}
	if *atariAtlasManifest != "" {
		registry, err := reducer.Themes.WithAtariSTAtlas(*atariAtlasManifest)
		if err != nil {
			fatal(fmt.Errorf("load Atari ST atlas: %w", err))
		}
		reducer.Themes = registry
	}
	state := game.NewState()
	var img image.Image
	if *spaceScene {
		state, err = reducer.Step(state, game.ActionPlay)
		if err != nil {
			fatal(err)
		}
		img, err = render.RenderSpaceComparison(state, reducer.Themes)
	} else {
		for _, action := range []game.Action{game.ActionPlay, game.ActionOpenGround, game.ActionMoveEast} {
			state, err = reducer.Step(state, action)
			if err != nil {
				fatal(err)
			}
		}
		img, err = render.RenderComparison(state, reducer.Themes)
	}
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		fatal(err)
	}
	file, err := os.Create(*outPath)
	if err != nil {
		fatal(err)
	}
	if err := render.WritePNG(file, img); err != nil {
		_ = file.Close()
		fatal(err)
	}
	if err := file.Close(); err != nil {
		fatal(err)
	}
	fmt.Println(*outPath)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
