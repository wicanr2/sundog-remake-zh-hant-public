package main

import (
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
)

func main() {
	disk, err := originalmedia.Open(os.Args[1])
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	images, err := disk.Directory()
	if err != nil {
		fmt.Println("dir:", err)
		os.Exit(1)
	}
	palette := originalmedia.PaletteRGBA()
	for _, meta := range images {
		if meta.Height != 32 {
			continue
		}
		picture, err := disk.DecodeImage(meta.Index)
		if err != nil {
			fmt.Printf("image %d %dx%d: %v\n", meta.Index, meta.Width, meta.Height, err)
			continue
		}
		out := image.NewRGBA(image.Rect(0, 0, picture.Width, picture.Height))
		for y := 0; y < picture.Height; y++ {
			for x := 0; x < picture.Width; x++ {
				out.SetRGBA(x, y, palette[picture.Pixels[y*picture.Width+x]&0x0F])
			}
		}
		path := fmt.Sprintf("%s/strip-%02d.png", os.Args[2], meta.Index)
		file, _ := os.Create(path)
		png.Encode(file, out)
		file.Close()
		fmt.Printf("wrote %s (%dx%d)\n", path, picture.Width, picture.Height)
	}
}
