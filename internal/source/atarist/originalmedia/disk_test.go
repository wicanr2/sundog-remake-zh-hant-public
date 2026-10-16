package originalmedia

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// Every test here runs on a synthetic disk. The real one is the player's own
// media and is never part of the repository, so the decoder is checked against
// bytes the test itself laid out to the documented layout.

func buildDisk(t *testing.T, entries [][2]int, bodies [][]byte) []byte {
	t.Helper()
	disk := make([]byte, expectedDiskBytes)
	base := logicalOffset(directoryBlock)
	for index, entry := range entries {
		binary.BigEndian.PutUint16(disk[base+index*4:], uint16(entry[0]))
		binary.BigEndian.PutUint16(disk[base+index*4+2:], uint16(entry[1]))
	}
	stream := logicalOffset(imageBaseBlock)
	for index, entry := range entries {
		copy(disk[stream+entry[0]*2:], bodies[index])
	}
	return disk
}

// solidImage is a picture of one repeated colour, in the simplest encoding the
// stream has: short runs of a literal nibble. The short form keeps the top bit
// clear, so a run is at most eight pixels.
func solidImage(width, height int, index byte) []byte {
	body := make([]byte, 4)
	binary.BigEndian.PutUint16(body, uint16(width))
	binary.BigEndian.PutUint16(body[2:], uint16(height))
	remaining := width * height
	for remaining > 0 {
		run := remaining
		if run > 8 {
			run = 8
		}
		body = append(body, byte((run-1)<<4)|flip4(index))
		remaining -= run
	}
	return body
}

func writeDisk(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "disk.st")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDirectoryDecodesContiguousEntries(t *testing.T) {
	body := solidImage(32, 32, 3)
	words := (len(body) + 1) / 2
	entries := [][2]int{{0, words}, {words, words}}
	disk := buildDisk(t, entries, [][]byte{body, body})
	// A directory this short would be rejected by Open's sanity check, so the
	// decoder is exercised directly.
	d := &Disk{bytes: disk}
	images, err := d.Directory()
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 {
		t.Fatalf("%d images, want 2", len(images))
	}
	if images[0].Width != 32 || images[0].Height != 32 {
		t.Fatalf("image 0 is %dx%d", images[0].Width, images[0].Height)
	}
}

func TestAGapInTheDirectoryIsRejected(t *testing.T) {
	body := solidImage(32, 32, 3)
	words := (len(body) + 1) / 2
	disk := buildDisk(t, [][2]int{{0, words}, {words + 1, words}}, [][]byte{body, body})
	d := &Disk{bytes: disk}
	if _, err := d.Directory(); err == nil {
		t.Fatal("a gap between entries was accepted")
	}
}

func TestDecodedPixelsAreTheColourTheStreamNames(t *testing.T) {
	body := solidImage(8, 4, 6)
	words := (len(body) + 1) / 2
	disk := buildDisk(t, [][2]int{{0, words}}, [][]byte{body})
	d := &Disk{bytes: disk}
	picture, err := d.DecodeImage(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(picture.Pixels) != 8*4 {
		t.Fatalf("%d pixels, want 32", len(picture.Pixels))
	}
	for offset, value := range picture.Pixels {
		if value != 6 {
			t.Fatalf("pixel %d is %d, want 6", offset, value)
		}
	}
}

func TestAStreamThatEndsEarlyIsRejected(t *testing.T) {
	body := solidImage(32, 32, 3)
	body = body[:8] // cut the pixel data short
	disk := buildDisk(t, [][2]int{{0, 64}}, [][]byte{body})
	d := &Disk{bytes: disk}
	if _, err := d.DecodeImage(0); err == nil {
		t.Fatal("a truncated stream decoded")
	}
}

func TestOpenRejectsAFileOfTheWrongSize(t *testing.T) {
	path := writeDisk(t, make([]byte, 1024))
	if _, err := Open(path); err == nil {
		t.Fatal("a short file was accepted as a disk")
	}
}

func TestOpenRejectsAnImageWithoutASunDogDirectory(t *testing.T) {
	path := writeDisk(t, make([]byte, expectedDiskBytes))
	if _, err := Open(path); err == nil {
		t.Fatal("a blank disk was accepted")
	}
}

func TestFlipFourIsItsOwnInverse(t *testing.T) {
	for value := byte(0); value < 16; value++ {
		if got := flip4(flip4(value)); got != value {
			t.Fatalf("flip4(flip4(%d)) = %d", value, got)
		}
	}
}
