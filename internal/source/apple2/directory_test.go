package apple2

import (
	"encoding/binary"
	"testing"
)

func TestParseDirectoryKeepsRawLocationsAndEvidence(t *testing.T) {
	data := syntheticDirectory()
	directory, err := ParseDirectory(data, "SUNDOG1.DSK", 0x100, 2)
	if err != nil {
		t.Fatal(err)
	}
	if directory.RawOffset != 0x100 || directory.Header.RawOffset != 0x100 {
		t.Fatalf("raw offset was not preserved: %+v", directory)
	}
	if directory.FilesystemLevel != EvidenceStrongInference || directory.ConsumerLevel != EvidenceUnknown {
		t.Fatalf("unexpected evidence levels: %+v", directory)
	}
	entry, ok := directory.Find("SYSTEM.DATA")
	if !ok || entry.RawOffset != 0x11a || entry.StartBlock != 3 || entry.NextBlock != 5 {
		t.Fatalf("unexpected entry: %+v, found=%v", entry, ok)
	}
}

func TestReadDeclaredSpanRequiresExplicitMapping(t *testing.T) {
	data := syntheticDirectory()
	directory, err := ParseDirectory(data, "SUNDOG1.DSK", 0x100, 2)
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := directory.Find("SYSTEM.DATA")
	span, err := ReadDeclaredSpan(data, entry, 0, 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(span) != 32 || span[0] != 0x31 || span[31] != 0x4f {
		t.Fatalf("unexpected raw span: len=%d first=%x last=%x", len(span), span[0], span[len(span)-1])
	}
}

func TestParseDirectoryRejectsNonContiguousEntries(t *testing.T) {
	data := syntheticDirectory()
	binary.LittleEndian.PutUint16(data[0x100+DirectoryEntrySize+DirectoryEntrySize:], 6)
	if _, err := ParseDirectory(data, "SUNDOG1.DSK", 0x100, 2); err == nil {
		t.Fatal("expected non-contiguous entry error")
	}
}

func syntheticDirectory() []byte {
	data := make([]byte, 16*32)
	const offset = 0x100
	binary.LittleEndian.PutUint16(data[offset:], 0)
	binary.LittleEndian.PutUint16(data[offset+2:], 3)
	binary.LittleEndian.PutUint16(data[offset+4:], 0)
	putPascalName(data[offset+6:], 8, "SUNDOG")
	binary.LittleEndian.PutUint16(data[offset+0x0e:], 7)
	binary.LittleEndian.PutUint16(data[offset+0x10:], 2)

	putEntry := func(entryOffset int, start, end uint16, name string, last uint16) {
		binary.LittleEndian.PutUint16(data[entryOffset:], start)
		binary.LittleEndian.PutUint16(data[entryOffset+2:], end)
		binary.LittleEndian.PutUint16(data[entryOffset+4:], 5)
		putPascalName(data[entryOffset+6:], 16, name)
		binary.LittleEndian.PutUint16(data[entryOffset+0x16:], last)
	}
	putEntry(offset+26, 3, 5, "SYSTEM.DATA", 16)
	putEntry(offset+52, 5, 7, "SYSTEM.APPLE", 16)
	for i := 3 * 16; i < 5*16; i++ {
		data[i] = byte(0x31 + (i % 8))
	}
	data[4*16+15] = 0x4f
	return data
}

func putPascalName(destination []byte, fieldSize int, value string) {
	destination[0] = byte(len(value))
	copy(destination[1:fieldSize], value)
}
