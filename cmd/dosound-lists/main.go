// Command dosound-lists prints the original's sound lists as hex, one per
// line, so another tool can render the same input.
//
//	dosound-lists -disk sundog.st
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
)

func main() {
	disk := flag.String("disk", "", "path to an original Atari ST disk image")
	flag.Parse()

	if err := run(*disk); err != nil {
		fmt.Fprintln(os.Stderr, "dosound-lists:", err)
		os.Exit(1)
	}
}

func run(path string) error {
	if path == "" {
		return fmt.Errorf("-disk is required")
	}
	image, err := originalmedia.Open(path)
	if err != nil {
		return err
	}
	bank, err := image.SoundBank()
	if err != nil {
		return err
	}
	for index := 0; index < bank.Count(); index++ {
		list, _ := bank.Sound(index)
		fmt.Printf("%s %s\n", originalmedia.SoundName(index), hex.EncodeToString(list))
	}
	warps, err := image.WarpSounds()
	if err != nil {
		return err
	}
	for _, warp := range warps {
		fmt.Printf("%s %s\n", warp.Name, hex.EncodeToString(warp.List))
	}
	return nil
}
