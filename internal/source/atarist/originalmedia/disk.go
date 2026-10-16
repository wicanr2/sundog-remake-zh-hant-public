// Package originalmedia reads the player's own Atari ST SunDog disk at run
// time.
//
// Nothing in this package ships with the remake. The repository carries the
// decoder and the layout it decodes, never the bytes it decodes: the tile
// pixels and the sound data stay on the disk the player already owns. A build
// without a disk runs on the remake's own art and stays fully playable, which
// is why every entry point here is optional and returns a plain error rather
// than failing the program.
//
// The layout is the one documented in docs/atari-st/: logical block L is at
// raw offset 0x04800 + L * 512.
package originalmedia

import (
	"encoding/binary"
	"fmt"
	"os"
)

const (
	blockSize         = 512
	logicalBaseOffset = 0x04800
	// The disk is a 720-sector single-sided image.
	expectedDiskBytes = 368640
)

// Disk is a read-only view of the player's raw .st image.
type Disk struct {
	bytes []byte
	path  string
}

// Open reads a raw .st image. The file is opened read-only and never written
// back; the caller owns whatever copy it points at.
func Open(path string) (*Disk, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Atari ST disk: %w", err)
	}
	if len(data) != expectedDiskBytes {
		return nil, fmt.Errorf("Atari ST disk is %d bytes, want %d", len(data), expectedDiskBytes)
	}
	disk := &Disk{bytes: data, path: path}
	if err := disk.verify(); err != nil {
		return nil, err
	}
	return disk, nil
}

// Path is the file the disk was read from, for reporting.
func (d *Disk) Path() string { return d.path }

// verify checks the disk carries the structures this package decodes. A wrong
// image of the right size would otherwise be found out much later, as garbled
// tiles rather than as a bad file.
func (d *Disk) verify() error {
	images, err := d.Directory()
	if err != nil {
		return fmt.Errorf("Atari ST disk does not carry a SunDog image directory: %w", err)
	}
	if len(images) < 32 {
		return fmt.Errorf("Atari ST disk lists only %d images", len(images))
	}
	return nil
}

func (d *Disk) at(offset, length int) ([]byte, error) {
	if offset < 0 || length < 0 || offset+length > len(d.bytes) {
		return nil, fmt.Errorf("read of %d bytes at 0x%05x is outside the disk", length, offset)
	}
	return d.bytes[offset : offset+length], nil
}

func logicalOffset(block int) int { return logicalBaseOffset + block*blockSize }

// ---- image directory ----

const (
	directoryBlock   = 0x47
	directoryEntries = 88
	imageBaseBlock   = 0x48
	imageHeaderBytes = 4
	maxImageWidth    = 512
	maxImageHeight   = 256
)

// Image is one entry of the directory: where the picture is and how big it
// says it is.
type Image struct {
	Index     int
	RawOffset int
	ByteLen   int
	Width     int
	Height    int
}

// Directory decodes the 88-slot image directory. Entries are contiguous, which
// is what makes a mis-read directory obvious rather than subtly wrong.
func (d *Disk) Directory() ([]Image, error) {
	raw, err := d.at(logicalOffset(directoryBlock), directoryEntries*4)
	if err != nil {
		return nil, err
	}
	stream := logicalOffset(imageBaseBlock)
	images := make([]Image, 0, directoryEntries)
	expected := 0
	for index := 0; index < directoryEntries; index++ {
		offsetWords := int(binary.BigEndian.Uint16(raw[index*4:]))
		lengthWords := int(binary.BigEndian.Uint16(raw[index*4+2:]))
		if lengthWords == 0 {
			if offsetWords != 0 {
				return nil, fmt.Errorf("directory entry %d has length 0 but offset %d", index, offsetWords)
			}
			continue
		}
		if offsetWords != expected {
			return nil, fmt.Errorf("directory entry %d starts at word %d, previous ended at %d",
				index, offsetWords, expected)
		}
		rawOffset := stream + offsetWords*2
		header, err := d.at(rawOffset, imageHeaderBytes)
		if err != nil {
			return nil, err
		}
		width := int(binary.BigEndian.Uint16(header))
		height := int(binary.BigEndian.Uint16(header[2:]))
		if width <= 0 || width > maxImageWidth || height <= 0 || height > maxImageHeight {
			return nil, fmt.Errorf("directory entry %d declares %dx%d", index, width, height)
		}
		images = append(images, Image{
			Index: index, RawOffset: rawOffset, ByteLen: lengthWords * 2,
			Width: width, Height: height,
		})
		expected = offsetWords + lengthWords
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("image directory is empty")
	}
	return images, nil
}

// Picture is a decoded image: one palette index per pixel.
type Picture struct {
	Width  int
	Height int
	Pixels []byte
}

// DecodeImage decompresses the picture in the given directory slot.
func (d *Disk) DecodeImage(index int) (Picture, error) {
	images, err := d.Directory()
	if err != nil {
		return Picture{}, err
	}
	for _, image := range images {
		if image.Index != index {
			continue
		}
		body, err := d.at(image.RawOffset+imageHeaderBytes, image.ByteLen-imageHeaderBytes)
		if err != nil {
			return Picture{}, err
		}
		pixels, err := decompress(body, image.Width, image.Height)
		if err != nil {
			return Picture{}, fmt.Errorf("image %d: %w", index, err)
		}
		return Picture{Width: image.Width, Height: image.Height, Pixels: pixels}, nil
	}
	return Picture{}, fmt.Errorf("image %d is not in the directory", index)
}

// flip4 reverses a nibble. The compressed stream stores its literal pixels
// bit-reversed.
func flip4(value byte) byte {
	return ((value & 0x1) << 3) | ((value & 0x2) << 1) | ((value & 0x4) >> 1) | ((value & 0x8) >> 3)
}

// decompress expands the 4-bit pixel stream. The opcode layout is the one the
// public secondary oracle documents and tools/decode_atari_st_images.py
// implements; this is the same routine in Go.
func decompress(source []byte, width, height int) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid dimensions %dx%d", width, height)
	}
	total := width * height
	pixels := make([]byte, 0, total)
	cursor := 0
	readByte := func() (byte, error) {
		if cursor >= len(source) {
			return 0, fmt.Errorf("stream ends at 0x%x", cursor)
		}
		value := source[cursor]
		cursor++
		return value, nil
	}
	appendPixels := func(values ...byte) error {
		if len(pixels)+len(values) > total {
			return fmt.Errorf("stream overshoots %d pixels", total)
		}
		pixels = append(pixels, values...)
		return nil
	}

	for len(pixels) < total {
		opcode, err := readByte()
		if err != nil {
			return nil, err
		}
		if opcode&0x80 == 0 {
			count := int(opcode>>4) + 1
			literal := flip4(opcode & 0x0F)
			for i := 0; i < count; i++ {
				if err := appendPixels(literal); err != nil {
					return nil, err
				}
			}
			continue
		}
		countByte, err := readByte()
		if err != nil {
			return nil, err
		}
		count := int(countByte)
		if opcode&0x40 != 0 {
			low, err := readByte()
			if err != nil {
				return nil, err
			}
			count = count<<8 | int(low)
		}
		count++
		switch {
		case opcode&0x10 == 0:
			literal := flip4(opcode & 0x0F)
			for i := 0; i < count; i++ {
				if err := appendPixels(literal); err != nil {
					return nil, err
				}
			}
		case opcode&0x20 != 0:
			// Copy from the line above, one pixel at a time: past a scanline
			// the copy reads pixels it has just written.
			if len(pixels) < width {
				return nil, fmt.Errorf("previous-line command before the first row")
			}
			for i := 0; i < count; i++ {
				if len(pixels) >= total {
					return nil, fmt.Errorf("previous-line command overshoots the image")
				}
				pixels = append(pixels, pixels[len(pixels)-width])
			}
			if err := appendPixels(flip4(opcode & 0x0F)); err != nil {
				return nil, err
			}
		default:
			if count&1 == 1 {
				if err := appendPixels(flip4(opcode & 0x0F)); err != nil {
					return nil, err
				}
			}
			for i := 0; i < count/2; i++ {
				packed, err := readByte()
				if err != nil {
					return nil, err
				}
				if err := appendPixels(flip4(packed>>4), flip4(packed&0x0F)); err != nil {
					return nil, err
				}
			}
		}
	}
	return pixels, nil
}
