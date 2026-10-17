package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ebitengine/oto/v3"
	"github.com/jfreymuth/oggvorbis"

	gameaudio "github.com/wicanr2/sundog-remake/internal/audio"
	"github.com/wicanr2/sundog-remake/internal/game"
)

// The sound effects are the original's, so they are not part of the remake.
// sundog-sound-export renders them from a disk the player owns into a
// directory the game reads at startup; with nothing there, the game runs
// silently and says so.
//
// Playback goes straight to the audio backend rather than through Ebiten's
// audio package. A machine with no output device — a headless build machine,
// a container, a desktop with sound switched off — reports that failure
// through Ebiten as a game error, which would end the game over something the
// player never asked for. Owning the device means a missing one is a line in
// the log and nothing more.

const (
	soundSampleRate = 44100
	soundChannels   = 2
)

// soundboard holds the exported sounds and plays them.
type soundboard struct {
	library *gameaudio.Library
	decoded map[string][]byte

	once      sync.Once
	context   *oto.Context
	available bool

	playing []*oto.Player
	muted   bool
}

// newSoundboard reads whatever the export left in dir. A directory that is
// missing or empty is not a failure: the game is playable without sound.
func newSoundboard(dir string) *soundboard {
	library, err := gameaudio.Load(dir)
	if err != nil {
		log.Printf("sounds in %s could not be read: %v", dir, err)
		return &soundboard{decoded: map[string][]byte{}}
	}
	return &soundboard{library: library, decoded: map[string][]byte{}}
}

// found reports whether the export left anything to play.
func (s *soundboard) found() bool {
	return s != nil && !s.library.Empty()
}

// count is how many sounds were found.
func (s *soundboard) count() int {
	if s == nil || s.library == nil {
		return 0
	}
	return len(s.library.Names())
}

// device opens the audio output the first time a sound is played, and reports
// whether there is one. A machine without one plays nothing and says so once.
func (s *soundboard) device() bool {
	s.once.Do(func() {
		context, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate:   soundSampleRate,
			ChannelCount: soundChannels,
			Format:       oto.FormatSignedInt16LE,
		})
		if err != nil {
			log.Printf("沒有可用的音效輸出，遊戲改用靜音：%v", err)
			return
		}
		<-ready
		s.context = context
		s.available = true
	})
	return s.available
}

// toggleMute turns the sound off and on, and reports the new setting.
func (s *soundboard) toggleMute() bool {
	if s == nil {
		return true
	}
	s.muted = !s.muted
	if s.muted {
		for _, player := range s.playing {
			player.Pause()
			player.Close()
		}
		s.playing = nil
	}
	return s.muted
}

// play starts one sound. Sounds overlap: a shot that lands while the last one
// is still ringing does not cut it off.
func (s *soundboard) play(name string) {
	if !s.found() || s.muted || name == "" {
		return
	}
	pcm, err := s.samples(name)
	if err != nil {
		log.Printf("sound %s could not be decoded: %v", name, err)
		return
	}
	if pcm == nil || !s.device() {
		return
	}
	player := s.context.NewPlayer(bytes.NewReader(pcm))
	player.Play()

	// Finished players are dropped so the list cannot grow without limit.
	live := s.playing[:0]
	for _, existing := range s.playing {
		if existing.IsPlaying() {
			live = append(live, existing)
			continue
		}
		existing.Close()
	}
	s.playing = append(live, player)
}

// samples decodes one sound the first time it is asked for and keeps it.
func (s *soundboard) samples(name string) ([]byte, error) {
	if pcm, ok := s.decoded[name]; ok {
		return pcm, nil
	}
	path, ok := s.library.Path(name)
	if !ok {
		// Not every sound the original has is wired to something here, and a
		// player may have exported only some. Remember the miss.
		s.decoded[name] = nil
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pcm []byte
	if strings.EqualFold(filepath.Ext(path), ".wav") {
		pcm, err = decodeWAV(raw)
	} else {
		pcm, err = decodeOgg(raw)
	}
	if err != nil {
		return nil, err
	}
	s.decoded[name] = pcm
	return pcm, nil
}

// decodeOgg turns one exported file into the frames the output wants: signed
// 16-bit, little endian, two channels.
func decodeOgg(raw []byte) ([]byte, error) {
	samples, format, err := oggvorbis.ReadAll(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if format.SampleRate != soundSampleRate {
		return nil, fmt.Errorf("sampled at %d Hz, want %d", format.SampleRate, soundSampleRate)
	}
	if format.Channels != 1 && format.Channels != soundChannels {
		return nil, fmt.Errorf("unsupported audio channels: %d", format.Channels)
	}
	frames := len(samples) / format.Channels
	out := make([]byte, frames*soundChannels*2)
	for frame := 0; frame < frames; frame++ {
		for channel := 0; channel < soundChannels; channel++ {
			sourceChannel := channel
			if format.Channels == 1 {
				sourceChannel = 0
			}
			value := clampSample(samples[frame*format.Channels+sourceChannel])
			binary.LittleEndian.PutUint16(out[(frame*soundChannels+channel)*2:], uint16(value))
		}
	}
	return out, nil
}

// decodeWAV reads what the export writes when there is no encoder to hand:
// one channel of signed 16-bit samples.
func decodeWAV(raw []byte) ([]byte, error) {
	if len(raw) < 44 || !bytes.Equal(raw[:4], []byte("RIFF")) || !bytes.Equal(raw[8:12], []byte("WAVE")) {
		return nil, fmt.Errorf("not a WAV file")
	}
	cursor := 12
	channels, rate, bits := 0, 0, 0
	for cursor+8 <= len(raw) {
		name := raw[cursor : cursor+4]
		size := int(binary.LittleEndian.Uint32(raw[cursor+4:]))
		if cursor+8+size > len(raw) {
			return nil, fmt.Errorf("chunk %q runs past the end of the file", name)
		}
		body := raw[cursor+8 : cursor+8+size]
		switch {
		case bytes.Equal(name, []byte("fmt ")):
			if len(body) < 16 {
				return nil, fmt.Errorf("format chunk is %d bytes", len(body))
			}
			channels = int(binary.LittleEndian.Uint16(body[2:]))
			rate = int(binary.LittleEndian.Uint32(body[4:]))
			bits = int(binary.LittleEndian.Uint16(body[14:]))
		case bytes.Equal(name, []byte("data")):
			if channels == 0 {
				return nil, fmt.Errorf("the data chunk comes before the format chunk")
			}
			if rate != soundSampleRate || bits != 16 {
				return nil, fmt.Errorf("%d Hz, %d bits; want %d Hz, 16 bits", rate, bits, soundSampleRate)
			}
			frames := len(body) / (channels * 2)
			out := make([]byte, frames*soundChannels*2)
			for frame := 0; frame < frames; frame++ {
				sample := body[frame*channels*2 : frame*channels*2+2]
				for channel := 0; channel < soundChannels; channel++ {
					copy(out[(frame*soundChannels+channel)*2:], sample)
				}
			}
			return out, nil
		}
		cursor += 8 + size + size%2
	}
	return nil, fmt.Errorf("no data chunk")
}

func clampSample(value float32) int16 {
	scaled := int(value * 32767)
	if scaled > 32767 {
		scaled = 32767
	}
	if scaled < -32768 {
		scaled = -32768
	}
	return int16(scaled)
}

// messageSounds ties the messages the reducer produces to the original's
// sounds.
//
// Which sound goes with which event is the remake's decision. The sounds are
// the original's and so are their names; where the original's own call sites
// have been read out of the p-code (docs/atari-st/sound-events.json), the
// remake follows them. fire_it plays sound 9 (cannon) for the wide gun and
// sound 8 (laser) for the tight one, and those are the two fire actions here.
var messageSounds = map[string]string{
	// Gunnery, on both sides. The enemy ship's gun is pirate_laser; nme_laser
	// is XDOCOMBA's, the gun a thug points at you on the ground, and using it
	// out here was this remake's own mistake. The two hit cues are the
	// original's own split: hit_shield when the shields hold, hit_hull when
	// they do not (SHIPLIB:update_attack).
	"ui.message.fired":               "cannon",
	"ui.message.laser_fired":         "laser",
	"ui.message.laser_missed":        "laser",
	"ui.message.enemy_destroyed":     "explosion",
	"ui.message.defeat":              "explosion",
	"ui.message.enemy_fired_hull":    "hit_hull",
	"ui.message.enemy_fired_shields": "hit_shield",
	"ui.message.enemy_fired_system":  "pirate_laser",
	"ui.message.enemy_missed":        "pirate_laser",
	// An encounter beginning is another ship turning up in the system.
	"ui.message.tactical": "ship_arrives",

	// The three tactical commands beside the guns. The original plays a
	// background tone while the tractor beam is engaged and a one-shot for the
	// bolts and the hail; docs/atari-st/sound-events.md maps all three.
	"ui.message.cargo_loaded": "tractor",
	"ui.message.bolts_armed":  "jettison",
	"ui.message.hailing":      "hail",

	// Ship systems giving out and being put right.
	"ui.message.guns_blown":         "part_dies",
	"ui.message.subc_engines_blown": "part_dies",
	"ui.message.warp_engines_blown": "part_dies",
	"ui.message.repaired":           "repair_bay",
	"ui.message.system_repaired":    "repair_bay",
	"ui.message.shields_repaired":   "repair_bay",
	"ui.message.weapon_repaired":    "repair_bay",
	"ui.message.part_installed":     "repair_bay",

	// Fuel and flight.
	"ui.message.refuel_eta":          "distress",
	"ui.message.refueled":            "fuel_here",
	"ui.message.aux_fuel_drained":    "fuel_here",
	"ui.message.under_way":           "warp2",
	"ui.message.navigated":           "warp3",
	"ui.message.out_of_fuel":         "warp_fail",
	"ui.message.insufficient_fuel":   "warp_fail",
	"ui.message.beyond_maximum_jump": "warp_fail",

	// Moving between the ship and the ground. The landing is XLANDFX, so the
	// cue plays as the horizon starts moving rather than on arrival.
	"ui.message.boarded": "teleport_up",
	"ui.message.landing": "teleport_down",
	"ui.message.landed":  "teleport_down",

	// Stepping inside a building. pod is an ambient loop in the original, but
	// for the ship bay (interaction kind 19) and for walking the ground —
	// sound-events.json has no entry tying it to a bar, an equipment shop, a
	// parts shop or a hotel. Reusing it for those four doors is this shell's
	// own choice: it has one cue per event and no background layer, so an
	// ambient loop has nowhere else to live.
	"ui.interior.kind.bar":       "pod",
	"ui.interior.kind.equipment": "pod",
	"ui.interior.kind.parts":     "pod",
	"ui.interior.kind.hotel":     "pod",

	// The trade counter works its demand out before it quotes (XDOTRADI:
	// set_demand), and says so with a tone.
	"ui.message.bought": "phase_done",
	"ui.message.sold":   "phase_done",

	// City life.
	"ui.message.street_mugger": "stinger",
	"ui.message.street_beggar": "hail",
	"ui.message.street_dealer": "hail",
	"ui.message.dialog_opened": "hail",
	// Every line of a frame steps with a click in the original
	// (XDOINTER:read_frame). The screen never changes during a conversation,
	// so without this the whole exchange after the greeting is silent.
	"ui.message.dialog_said":      "menu_click",
	"ui.message.mission_complete": "jackpot",
	"ui.message.cargo_unloaded":   "jettison",

	// Inside a building, and the machine in the corner.
	"ui.message.slots_jackpot": "jackpot",
	"ui.message.slots_pair":    "menu_click",
	"ui.message.slots_lost":    "menu_click",
	"ui.message.no_credit":     "disk_warn",

	// Ground combat: the same weapon and death cues the arena uses.
	"ui.message.arena_exchange": "nme_laser",
	"ui.message.arena_won":      "explosion",
	"ui.message.arena_lost":     "part_dies",
	// Taking a hit and running dry are the original's own two cues for this
	// fight: XDOCOMBA:its_ahit plays stinger when a shot lands on the player,
	// and fire_gun plays part_dies when the scattergun is spent.
	"ui.message.arena_hit":      "stinger",
	"ui.message.arena_ammo_out": "part_dies",

	// Trouble with the disk the game keeps its library on.
	"ui.message.library_missing":  "disk_warn",
	"ui.message.save_failed":      "disk_warn",
	"ui.message.failure_terminal": "disk_dead",
}

// messageFollowUpSounds are the second cue for the few events the original
// answers with two. Firing is the one that matters: XDOFIGHT:0x06 fire_it plays
// the gun going off and then, if the shot connects, the hit landing on the
// other ship (sound event hit_pirate). A laser that misses fires without the
// follow-up, which is also the original's split.
var messageFollowUpSounds = map[string]string{
	"ui.message.fired":       "hit_pirate",
	"ui.message.laser_fired": "hit_pirate",
}

// soundCue picks what to play for one step.
//
// A fight starting speaks over anything else; then whatever the step's message
// stands for; and failing both, a click whenever the screen changes, which is
// what the original's menus did.
func soundCue(before, after game.State) string {
	if !before.CombatActive && after.CombatActive {
		return "attack_siren"
	}
	if sound, ok := messageSounds[after.MessageKey]; ok {
		return sound
	}
	if before.Screen != after.Screen {
		return "menu_click"
	}
	return ""
}

// soundFollowUp is the cue that plays a beat after soundCue, or empty.
func soundFollowUp(before, after game.State) string {
	if !before.CombatActive && after.CombatActive {
		return ""
	}
	return messageFollowUpSounds[after.MessageKey]
}
