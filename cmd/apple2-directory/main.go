package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/wicanr2/sundog-remake/internal/source/apple2"
)

func main() {
	diskPath := flag.String("disk", "SUNDOG1.DSK", "Apple II disk image supplied outside the repository")
	diskName := flag.String("disk-name", "SUNDOG1.DSK", "source disk name recorded in the manifest")
	offset := flag.Int("offset", 0x9100, "raw candidate offset")
	entries := flag.Int("entries", 6, "directory entry count")
	flag.Parse()

	data, err := os.ReadFile(*diskPath)
	if err != nil {
		fatal(err)
	}
	directory, err := apple2.ParseDirectory(data, *diskName, *offset, *entries)
	if err != nil {
		fatal(err)
	}
	directory.DiskSHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(directory); err != nil {
		fatal(err)
	}
	for _, entry := range directory.Entries {
		fmt.Fprintf(os.Stderr, "%s raw=0x%x blocks=%d..%d\n", entry.Name, entry.RawOffset, entry.StartBlock, entry.NextBlock)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
