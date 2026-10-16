package originalmedia

import (
	"encoding/binary"
	"testing"
)

// The sound table and the lists behind it are laid out here the way the disk
// lays them out, so the decoder is checked without any of the player's media.

func buildSoundDisk(t *testing.T, lists [][]byte) []byte {
	t.Helper()
	// The directory has to look like the real one before anything else on the
	// disk is read, so the synthetic disk carries a full set of small images.
	const images = 40
	entries := make([][2]int, images)
	bodies := make([][]byte, images)
	word := 0
	for index := range entries {
		body := solidImage(2, 2, byte(index%16))
		bodies[index] = body
		entries[index] = [2]int{word, (len(body) + 1) / 2}
		word += (len(body) + 1) / 2
	}
	disk := buildDisk(t, entries, bodies)

	offset := 0
	for index, list := range lists {
		binary.BigEndian.PutUint16(disk[soundTableOffset+index*2:], uint16(offset))
		copy(disk[soundDataOffset+offset:], list)
		offset += len(list)
	}
	// The word after the last list closes it, and the rest of the table is
	// zero, which is how the disk marks the end.
	binary.BigEndian.PutUint16(disk[soundTableOffset+len(lists)*2:], uint16(offset))
	return disk
}

// A list of ten well-formed sounds comes back as ten sounds, in order.
func TestSoundBankReadsEveryList(t *testing.T) {
	lists := make([][]byte, 10)
	for index := range lists {
		lists[index] = []byte{0x08, byte(index), 0xFF, 0x00}
	}
	disk, err := Open(writeDisk(t, buildSoundDisk(t, lists)))
	if err != nil {
		t.Fatal(err)
	}
	bank, err := disk.SoundBank()
	if err != nil {
		t.Fatal(err)
	}
	if bank.Count() != len(lists) {
		t.Fatalf("got %d sounds, want %d", bank.Count(), len(lists))
	}
	for index := range lists {
		got, ok := bank.Sound(index)
		if !ok {
			t.Fatalf("sound %d is missing", index)
		}
		if got[1] != byte(index) {
			t.Errorf("sound %d holds %x", index, got)
		}
	}
}

// A table that does not start at zero is a table read from the wrong place.
func TestSoundTableMustStartAtZero(t *testing.T) {
	lists := make([][]byte, 10)
	for index := range lists {
		lists[index] = []byte{0xFF, 0x00}
	}
	disk := buildSoundDisk(t, lists)
	binary.BigEndian.PutUint16(disk[soundTableOffset:], 4)

	image, err := Open(writeDisk(t, disk))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := image.SoundBank(); err == nil {
		t.Error("a table that starts anywhere but zero should be rejected")
	}
}

// So is one holding bytes that are not Dosound commands.
func TestSoundBankRejectsBytesThatAreNotCommands(t *testing.T) {
	lists := make([][]byte, 10)
	for index := range lists {
		lists[index] = []byte{0xFF, 0x00}
	}
	// 0x40 is in the gap between the register writes and the waits.
	lists[4] = []byte{0x40, 0x01, 0xFF, 0x00}

	image, err := Open(writeDisk(t, buildSoundDisk(t, lists)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := image.SoundBank(); err == nil {
		t.Error("a list holding a command that does not exist should be rejected")
	}
}

// A table too short to be the real one is rejected rather than half-read.
func TestSoundBankRejectsAShortTable(t *testing.T) {
	lists := [][]byte{{0xFF, 0x00}, {0xFF, 0x00}}
	image, err := Open(writeDisk(t, buildSoundDisk(t, lists)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := image.SoundBank(); err == nil {
		t.Error("a table of two entries should be rejected")
	}
}

// Every sound the export can produce has a name, and no name is used twice.
func TestSoundNamesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range SoundNames() {
		if name == "" {
			t.Error("a sound has no name")
		}
		if seen[name] {
			t.Errorf("%s is used twice", name)
		}
		seen[name] = true
	}
	if len(seen) != 33 {
		t.Errorf("got %d names, want 33", len(seen))
	}
}

// The numbered name keeps the table order in the file name and the plain name
// after it, which is what the game looks up.
func TestSoundNameCarriesItsNumber(t *testing.T) {
	if got := SoundName(4); got != "04-menu_click" {
		t.Errorf("got %q", got)
	}
	if got := SoundName(len(soundNames) + 5); got != "33-sound" {
		t.Errorf("got %q", got)
	}
}
