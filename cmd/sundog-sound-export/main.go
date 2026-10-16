// Command sundog-sound-export turns the sounds on an original Atari ST disk
// into audio files the remake can play.
//
// The disk holds register commands for the machine's sound chip, not audio, so
// the export renders them through a model of that chip. Nothing from the disk
// is copied into the remake's source tree; the files land wherever you point
// this, and the remake reads them from there at run time.
//
//	sundog-sound-export -disk sundog.st -out ~/.config/sundog-remake/audio
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/wicanr2/sundog-remake/internal/audio"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
)

const (
	sampleRate = 44100
	// Frames to keep the chip running after a list ends, so an envelope can
	// finish rather than being cut off.
	trailingFrames = 15
)

func main() {
	disk := flag.String("disk", "", "path to an original Atari ST disk image")
	out := flag.String("out", "", "directory to write the audio files into (default: the remake's own)")
	format := flag.String("format", "ogg", "ogg or wav; ogg needs ffmpeg on PATH")
	quality := flag.String("quality", "5", "vorbis quality, -1 to 10")
	clock := flag.Int("clock", audio.AtariSTClock, "sound chip clock; the default is the Atari ST's")
	flag.Parse()

	if err := run(*disk, *out, *format, *quality, *clock); err != nil {
		fmt.Fprintln(os.Stderr, "sundog-sound-export:", err)
		os.Exit(1)
	}
}

func run(diskPath, outDir, format, quality string, clock int) error {
	if diskPath == "" {
		return fmt.Errorf("-disk is required")
	}
	if format != "ogg" && format != "wav" {
		return fmt.Errorf("format %q is neither ogg nor wav", format)
	}
	if outDir == "" {
		outDir = audio.DefaultDir()
	}

	disk, err := originalmedia.Open(diskPath)
	if err != nil {
		return err
	}

	bank, err := disk.SoundBank()
	if err != nil {
		return err
	}
	warps, err := disk.WarpSounds()
	if err != nil {
		return err
	}

	sounds := make([]originalmedia.NamedSound, 0, bank.Count()+len(warps))
	for index := 0; index < bank.Count(); index++ {
		list, _ := bank.Sound(index)
		sounds = append(sounds, originalmedia.NamedSound{
			Name: originalmedia.SoundName(index),
			List: list,
		})
	}
	sounds = append(sounds, warps...)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	encoder := ""
	if format == "ogg" {
		encoder, err = exec.LookPath("ffmpeg")
		if err != nil {
			return fmt.Errorf("ogg output needs ffmpeg on PATH; use -format wav instead")
		}
	}

	for _, sound := range sounds {
		samples, err := audio.RenderWithClock(sound.List, sampleRate, trailingFrames, clock)
		if err != nil {
			return fmt.Errorf("rendering %s: %w", sound.Name, err)
		}
		wavPath := filepath.Join(outDir, sound.Name+".wav")
		if err := writeWAV(wavPath, samples, sampleRate); err != nil {
			return err
		}
		if encoder == "" {
			fmt.Printf("%-24s %6.2fs\n", filepath.Base(wavPath), seconds(len(samples)))
			continue
		}
		oggPath := filepath.Join(outDir, sound.Name+".ogg")
		command := exec.Command(encoder, "-hide_banner", "-loglevel", "error", "-y",
			"-i", wavPath, "-c:a", "libvorbis", "-q:a", quality, oggPath)
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("encoding %s: %v: %s", sound.Name, err, output)
		}
		if err := os.Remove(wavPath); err != nil {
			return err
		}
		fmt.Printf("%-24s %6.2fs\n", filepath.Base(oggPath), seconds(len(samples)))
	}
	fmt.Printf("wrote %d sounds to %s\n", len(sounds), outDir)
	return nil
}

func seconds(sampleCount int) float64 {
	return float64(sampleCount) / sampleRate
}

func writeWAV(path string, samples []int16, rate int) error {
	body := make([]byte, len(samples)*2)
	for index, value := range samples {
		binary.LittleEndian.PutUint16(body[index*2:], uint16(value))
	}

	header := make([]byte, 44)
	copy(header[0:], "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(body)))
	copy(header[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1) // PCM
	binary.LittleEndian.PutUint16(header[22:], 1) // mono
	binary.LittleEndian.PutUint32(header[24:], uint32(rate))
	binary.LittleEndian.PutUint32(header[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(header[32:], 2)
	binary.LittleEndian.PutUint16(header[34:], 16)
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(len(body)))

	return os.WriteFile(path, append(header, body...), 0o644)
}
