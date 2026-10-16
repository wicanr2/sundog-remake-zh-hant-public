package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
)

// Every sound the cues ask for has to be one the export produces, or the cue
// is silent and nobody notices.
func TestEveryCuedSoundIsExported(t *testing.T) {
	exported := map[string]bool{}
	for _, name := range originalmedia.SoundNames() {
		exported[name] = true
	}
	cued := append([]string{"attack_siren", "menu_click"}, values(messageSounds)...)
	for _, name := range cued {
		if !exported[name] {
			t.Errorf("%s is cued but is not one of the original's sounds", name)
		}
	}
}

// Every message a cue names has to be one the reducer can actually produce.
func TestEveryCuedMessageExists(t *testing.T) {
	// The frontend sets these itself when storage fails, so they are not in
	// the reducer's own set.
	frontend := map[string]bool{
		"ui.message.library_missing": true,
		"ui.message.save_failed":     true,
	}
	produced := gameMessageKeys(t)
	for key := range messageSounds {
		if frontend[key] || produced[key] {
			continue
		}
		t.Errorf("%s is cued but nothing produces it", key)
	}
}

// A fight starting is heard over whatever else the step did.
func TestCombatStartingSpeaksFirst(t *testing.T) {
	before := game.State{Screen: game.ScreenShipboard}
	after := game.State{Screen: game.ScreenTactical, CombatActive: true, MessageKey: "ui.message.fired"}
	if got := soundCue(before, after); got != "attack_siren" {
		t.Errorf("got %q, want attack_siren", got)
	}
}

// Otherwise the step's message decides.
func TestMessageDecidesTheSound(t *testing.T) {
	before := game.State{Screen: game.ScreenTactical, CombatActive: true}
	after := game.State{Screen: game.ScreenTactical, CombatActive: true, MessageKey: "ui.message.fired"}
	if got := soundCue(before, after); got != "cannon" {
		t.Errorf("got %q, want cannon", got)
	}
	after.MessageKey = "ui.message.laser_fired"
	if got := soundCue(before, after); got != "laser" {
		t.Errorf("got %q, want laser", got)
	}
}

// A step that only changes the screen clicks, the way the original's menus did.
func TestScreenChangeClicks(t *testing.T) {
	before := game.State{Screen: game.ScreenShipboard}
	after := game.State{Screen: game.ScreenCargo, MessageKey: "ui.message.cargo"}
	if got := soundCue(before, after); got != "menu_click" {
		t.Errorf("got %q, want menu_click", got)
	}
}

// A step that changes nothing worth hearing is silent.
func TestAnUnremarkableStepIsSilent(t *testing.T) {
	before := game.State{Screen: game.ScreenGround, MessageKey: "ui.message.moved"}
	after := game.State{Screen: game.ScreenGround, MessageKey: "ui.message.moved"}
	if got := soundCue(before, after); got != "" {
		t.Errorf("got %q, want silence", got)
	}
}

func values(table map[string]string) []string {
	out := make([]string, 0, len(table))
	for _, value := range table {
		out = append(out, value)
	}
	return out
}

// gameMessageKeys is every message key the reducer's own source can set.
func gameMessageKeys(t *testing.T) map[string]bool {
	t.Helper()
	// Most of what the reducer puts in MessageKey is a ui.message.* key, but not
	// all of it: InteriorLabelKey answers with ui.interior.kind.*, and a pattern
	// that only knew about the first prefix would report those as unproduced.
	pattern := regexp.MustCompile(`ui\.(?:message|interior\.kind)\.[a-z_]+`)
	keys := map[string]bool{}
	entries, err := os.ReadDir(filepath.Join("..", "..", "internal", "game"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		body, err := os.ReadFile(filepath.Join("..", "..", "internal", "game", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range pattern.FindAllString(string(body), -1) {
			keys[key] = true
		}
	}
	if len(keys) == 0 {
		t.Fatal("no message keys were found in the reducer's source")
	}
	return keys
}

// A machine with no audio output has to keep playing the game. The build
// machines have none, so this runs the whole path — decode, open the device,
// fail — and asks only that it comes back.
func TestPlayingWithoutADeviceIsSurvivable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "04-menu_click.wav"), tone(), 0o644); err != nil {
		t.Fatal(err)
	}
	board := newSoundboard(dir)
	if !board.found() {
		t.Fatal("the sound should have been found")
	}
	board.play("menu_click")
	board.play("menu_click")
	board.play("nothing-of-that-name")
}

// The exported files are decoded the way the export writes them.
func TestDecodeWAVMakesStereoFrames(t *testing.T) {
	pcm, err := decodeWAV(tone())
	if err != nil {
		t.Fatal(err)
	}
	if want := toneFrames * soundChannels * 2; len(pcm) != want {
		t.Errorf("got %d bytes, want %d", len(pcm), want)
	}
	// One mono sample becomes the same value on both channels.
	if !bytes.Equal(pcm[0:2], pcm[2:4]) {
		t.Error("the two channels should carry the same sample")
	}
}

// A file that is not one of ours is refused rather than played as noise.
func TestDecodeWAVRejectsWhatItCannotPlay(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("RIFF"), append([]byte("RIFFxxxxWAVE"), make([]byte, 40)...)} {
		if _, err := decodeWAV(raw); err == nil {
			t.Errorf("%q should have been refused", raw)
		}
	}
}

const toneFrames = 512

// tone is a mono 16-bit file in the shape the export writes. The bytes are the
// test's own; nothing here comes from the original.
func tone() []byte {
	body := make([]byte, toneFrames*2)
	for frame := 0; frame < toneFrames; frame++ {
		value := int16(3000)
		if frame%16 < 8 {
			value = -3000
		}
		binary.LittleEndian.PutUint16(body[frame*2:], uint16(value))
	}
	header := make([]byte, 44)
	copy(header[0:], "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(body)))
	copy(header[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1)
	binary.LittleEndian.PutUint16(header[22:], 1)
	binary.LittleEndian.PutUint32(header[24:], 44100)
	binary.LittleEndian.PutUint32(header[28:], 44100*2)
	binary.LittleEndian.PutUint16(header[32:], 2)
	binary.LittleEndian.PutUint16(header[34:], 16)
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(len(body)))
	return append(header, body...)
}

// Decoding a real export is checked against a directory the player points at,
// because nothing from the original disk is in the repository. Set
// SUNDOG_AUDIO_DIR to what sundog-sound-export wrote to run it.
func TestExportedSoundsDecode(t *testing.T) {
	dir := os.Getenv("SUNDOG_AUDIO_DIR")
	if dir == "" {
		t.Skip("SUNDOG_AUDIO_DIR names an export to check; it is not set")
	}
	board := newSoundboard(dir)
	if !board.found() {
		t.Fatalf("%s holds no sounds", dir)
	}
	for _, name := range board.library.Names() {
		pcm, err := board.samples(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(pcm) == 0 {
			t.Errorf("%s decoded to nothing", name)
		}
		if len(pcm)%(soundChannels*2) != 0 {
			t.Errorf("%s decoded to %d bytes, which is not whole frames", name, len(pcm))
		}
	}
}
