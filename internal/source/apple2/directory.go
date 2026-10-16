// Package apple2 contains clean-room, evidence-preserving readers for the
// Apple II source material used by this remake.
package apple2

import (
	"encoding/binary"
	"fmt"
)

// EvidenceLevel is deliberately part of the data model. A parsed byte layout
// is not automatically a proven filesystem or game consumer.
type EvidenceLevel string

const (
	EvidenceProven          EvidenceLevel = "proven"
	EvidenceStrongInference EvidenceLevel = "strong-inference"
	EvidenceHypothesis      EvidenceLevel = "hypothesis"
	EvidenceUnknown         EvidenceLevel = "unknown"
)

const DirectoryEntrySize = 26

// Directory is the typed form of a Pascal-like directory candidate. RawOffset
// and every entry offset remain available so later consumer work can trace a
// field back to the source image.
type Directory struct {
	DiskName        string        `json:"disk"`
	DiskSHA256      string        `json:"diskSha256,omitempty"`
	RawOffset       int           `json:"rawOffset"`
	EntrySize       int           `json:"entrySize"`
	Header          Header        `json:"header"`
	Entries         []Entry       `json:"entries"`
	FilesystemLevel EvidenceLevel `json:"filesystemEvidence"`
	ConsumerLevel   EvidenceLevel `json:"consumerEvidence"`
}

type Header struct {
	RawOffset        int    `json:"rawOffset"`
	SystemAreaStart  uint16 `json:"systemAreaStartBlock"`
	NextBlock        uint16 `json:"nextBlock"`
	FileType         uint16 `json:"fileType"`
	VolumeName       string `json:"volumeName"`
	BlocksInVolume   uint16 `json:"blocksInVolume"`
	FilesInDirectory uint16 `json:"filesInDirectory"`
	LastAccess       uint16 `json:"lastAccess"`
	Date             uint16 `json:"date"`
	ReservedHex      string `json:"reservedHex"`
}

type Entry struct {
	Index              int    `json:"index"`
	RawOffset          int    `json:"rawOffset"`
	StartBlock         uint16 `json:"startBlock"`
	NextBlock          uint16 `json:"nextBlock"`
	FileType           uint16 `json:"fileType"`
	Name               string `json:"name"`
	BytesUsedLastBlock uint16 `json:"bytesUsedLastBlock"`
	Date               uint16 `json:"date"`
}

// ParseDirectory parses only a caller-selected raw candidate. It validates
// the internal shape but intentionally does not decide whether the candidate
// is the real Apple Pascal filesystem or whether the boot loader consumes it.
func ParseDirectory(data []byte, diskName string, rawOffset, entryCount int) (Directory, error) {
	if rawOffset < 0 || rawOffset+DirectoryEntrySize > len(data) {
		return Directory{}, fmt.Errorf("directory header is outside image: offset 0x%x", rawOffset)
	}
	if entryCount <= 0 {
		return Directory{}, fmt.Errorf("entry count must be positive")
	}
	header, err := parseHeader(data, rawOffset)
	if err != nil {
		return Directory{}, err
	}
	if int(header.FilesInDirectory) != entryCount {
		return Directory{}, fmt.Errorf("entry count %d differs from header count %d", entryCount, header.FilesInDirectory)
	}
	end := rawOffset + DirectoryEntrySize*(entryCount+1)
	if end > len(data) {
		return Directory{}, fmt.Errorf("directory entries end outside image: 0x%x", end)
	}
	entries := make([]Entry, 0, entryCount)
	var previousNext uint16
	for index := 0; index < entryCount; index++ {
		entry, err := parseEntry(data, rawOffset+DirectoryEntrySize*(index+1), index+1)
		if err != nil {
			return Directory{}, err
		}
		if entry.StartBlock >= entry.NextBlock {
			return Directory{}, fmt.Errorf("entry %q has non-positive block span", entry.Name)
		}
		if index > 0 && entry.StartBlock != previousNext {
			return Directory{}, fmt.Errorf("entry %q is not contiguous with previous entry", entry.Name)
		}
		if entry.NextBlock > header.BlocksInVolume {
			return Directory{}, fmt.Errorf("entry %q ends after volume", entry.Name)
		}
		if entry.BytesUsedLastBlock == 0 || entry.BytesUsedLastBlock > 512 {
			return Directory{}, fmt.Errorf("entry %q has invalid bytes-used value %d", entry.Name, entry.BytesUsedLastBlock)
		}
		entries = append(entries, entry)
		previousNext = entry.NextBlock
	}
	if previousNext != header.BlocksInVolume {
		return Directory{}, fmt.Errorf("last entry ends at block %d, volume ends at %d", previousNext, header.BlocksInVolume)
	}
	return Directory{
		DiskName:        diskName,
		RawOffset:       rawOffset,
		EntrySize:       DirectoryEntrySize,
		Header:          header,
		Entries:         entries,
		FilesystemLevel: EvidenceStrongInference,
		ConsumerLevel:   EvidenceUnknown,
	}, nil
}

func parseHeader(data []byte, offset int) (Header, error) {
	volumeName, err := parsePascalName(data, offset+6, 8)
	if err != nil {
		return Header{}, fmt.Errorf("parse volume name at 0x%x: %w", offset+6, err)
	}
	return Header{
		RawOffset:        offset,
		SystemAreaStart:  binary.LittleEndian.Uint16(data[offset:]),
		NextBlock:        binary.LittleEndian.Uint16(data[offset+2:]),
		FileType:         binary.LittleEndian.Uint16(data[offset+4:]),
		VolumeName:       volumeName,
		BlocksInVolume:   binary.LittleEndian.Uint16(data[offset+0x0e:]),
		FilesInDirectory: binary.LittleEndian.Uint16(data[offset+0x10:]),
		LastAccess:       binary.LittleEndian.Uint16(data[offset+0x12:]),
		Date:             binary.LittleEndian.Uint16(data[offset+0x14:]),
		ReservedHex:      fmt.Sprintf("%x", data[offset+0x16:offset+DirectoryEntrySize]),
	}, nil
}

func parseEntry(data []byte, offset, index int) (Entry, error) {
	name, err := parsePascalName(data, offset+6, 16)
	if err != nil {
		return Entry{}, fmt.Errorf("parse entry %d name at 0x%x: %w", index, offset+6, err)
	}
	return Entry{
		Index:              index,
		RawOffset:          offset,
		StartBlock:         binary.LittleEndian.Uint16(data[offset:]),
		NextBlock:          binary.LittleEndian.Uint16(data[offset+2:]),
		FileType:           binary.LittleEndian.Uint16(data[offset+4:]),
		Name:               name,
		BytesUsedLastBlock: binary.LittleEndian.Uint16(data[offset+0x16:]),
		Date:               binary.LittleEndian.Uint16(data[offset+0x18:]),
	}, nil
}

func parsePascalName(data []byte, offset, fieldSize int) (string, error) {
	if offset < 0 || offset+fieldSize > len(data) {
		return "", fmt.Errorf("name field is outside image")
	}
	length := int(data[offset])
	if length > fieldSize-1 {
		return "", fmt.Errorf("length %d exceeds field capacity %d", length, fieldSize-1)
	}
	raw := data[offset+1 : offset+1+length]
	for _, value := range raw {
		if value < 0x20 || value > 0x7e {
			return "", fmt.Errorf("name contains non-printable byte 0x%02x", value)
		}
	}
	return string(raw), nil
}

// Find returns an entry by its source name without changing the evidence
// status or inventing a logical-block mapping.
func (d Directory) Find(name string) (Entry, bool) {
	for _, entry := range d.Entries {
		if entry.Name == name {
			return entry, true
		}
	}
	return Entry{}, false
}

// ReadDeclaredSpan reads an entry only when the caller supplies an explicit
// logical-block-to-raw mapping. The mapping is not inferred from the
// directory's raw offset because the SunDog candidate's relationship remains
// unknown. The returned bytes are a raw span, not a proven decoded file.
func ReadDeclaredSpan(data []byte, entry Entry, baseOffset, blockSize int) ([]byte, error) {
	if baseOffset < 0 || blockSize <= 0 {
		return nil, fmt.Errorf("base offset and block size must be non-negative/positive")
	}
	start := baseOffset + int(entry.StartBlock)*blockSize
	end := baseOffset + int(entry.NextBlock)*blockSize
	if start < baseOffset || end < start || end > len(data) {
		return nil, fmt.Errorf("declared span 0x%x..0x%x is outside image", start, end)
	}
	if entry.BytesUsedLastBlock < uint16(blockSize) {
		end -= blockSize - int(entry.BytesUsedLastBlock)
	}
	return append([]byte(nil), data[start:end]...), nil
}
