package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/wicanr2/sundog-remake/internal/game"
)

type musicTrack struct {
	ID, Title, File, SHA256 string
	Frames                  int
}
type musicPlayer interface {
	Play()
	Pause()
	Close() error
	SetVolume(float64)
}
type musicVoice struct {
	cue          string
	player       musicPlayer
	gain, target float64
}
type musicSystem struct {
	enabled bool
	dir     string
	sounds  *soundboard
	tracks  map[string]musicTrack
	decoded map[string][]byte
	failed  map[string]bool
	voices  []musicVoice
	last    time.Time
	open    func(string) (musicPlayer, error)
}

func newMusicSystem(sounds *soundboard, dir string) *musicSystem {
	m := &musicSystem{enabled: true, dir: dir, sounds: sounds, tracks: map[string]musicTrack{}, decoded: map[string][]byte{}, failed: map[string]bool{}}
	var manifest struct {
		Schema, SampleRate, Channels int
		Tracks                       []musicTrack
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err == nil {
		err = json.Unmarshal(raw, &manifest)
	}
	if err == nil && (manifest.Schema != 1 || manifest.SampleRate != soundSampleRate || manifest.Channels != soundChannels) {
		err = fmt.Errorf("unsupported music format")
	}
	if err == nil {
		for _, t := range manifest.Tracks {
			if t.ID == "" || t.Frames < 1 || filepath.Base(t.File) != t.File || filepath.Ext(t.File) != ".ogg" || len(t.SHA256) != 64 || m.tracks[t.ID].ID != "" {
				err = fmt.Errorf("invalid music track %q", t.ID)
				break
			}
			m.tracks[t.ID] = t
		}
	}
	if err != nil {
		log.Printf("背景配樂無法載入，繼續遊戲：%v", err)
		m.tracks = map[string]musicTrack{}
	}
	m.open = m.openTrack
	return m
}

func (m *musicSystem) openTrack(cue string) (musicPlayer, error) {
	t, ok := m.tracks[cue]
	if !ok {
		return nil, fmt.Errorf("missing music cue %s", cue)
	}
	pcm, ok := m.decoded[cue]
	if !ok {
		raw, err := os.ReadFile(filepath.Join(m.dir, t.File))
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(raw)
		if hex.EncodeToString(hash[:]) != t.SHA256 {
			return nil, fmt.Errorf("music hash mismatch: %s", cue)
		}
		pcm, err = decodeOgg(raw)
		if err != nil {
			return nil, err
		}
		if len(pcm) != t.Frames*soundChannels*2 {
			return nil, fmt.Errorf("music length mismatch: %s", cue)
		}
		m.decoded[cue] = pcm
	}
	if m.sounds == nil || !m.sounds.device() {
		return nil, fmt.Errorf("audio device unavailable")
	}
	return m.sounds.context.NewPlayer(&loopPCMReader{data: pcm}), nil
}

// update is presentation-only: elapsed audio time never advances game clocks/RNG.
// There are at most two live players, including during rapid scene changes.
func (m *musicSystem) update(cue string, now time.Time) {
	if m == nil {
		return
	}
	dt := 1.0 / 60
	if !m.last.IsZero() {
		dt = math.Max(0, math.Min(.1, now.Sub(m.last).Seconds()))
	}
	m.last = now
	found := -1
	for i := range m.voices {
		m.voices[i].target = 0
		if m.voices[i].cue == cue {
			found = i
		}
	}
	if m.enabled {
		if found < 0 && !m.failed[cue] {
			player, err := m.open(cue)
			if err != nil {
				m.failed[cue] = true
				log.Printf("背景配樂 %s 無法播放：%v", cue, err)
			} else {
				if len(m.voices) == 2 {
					drop := 0
					if m.voices[1].gain < m.voices[0].gain {
						drop = 1
					}
					m.voices[drop].player.Close()
					m.voices = append(m.voices[:drop], m.voices[drop+1:]...)
				}
				player.SetVolume(0)
				player.Play()
				m.voices = append(m.voices, musicVoice{cue: cue, player: player})
				found = len(m.voices) - 1
				log.Printf("HD配樂：%s", cue)
			}
		}
		if found >= 0 {
			m.voices[found].target = 1
			m.voices[found].player.Play()
		}
	}
	master := .28
	if m.sounds != nil {
		for _, p := range m.sounds.playing {
			if p.IsPlaying() {
				master = .14
				break
			}
		}
	}
	live := m.voices[:0]
	for _, voice := range m.voices {
		step := dt / .9
		if voice.gain < voice.target {
			voice.gain = math.Min(voice.target, voice.gain+step)
		} else {
			voice.gain = math.Max(voice.target, voice.gain-step)
		}
		voice.player.SetVolume(voice.gain * master)
		if voice.gain == 0 && voice.target == 0 {
			if !m.enabled && voice.cue == cue {
				voice.player.Pause()
				live = append(live, voice)
			} else {
				voice.player.Close()
			}
		} else {
			live = append(live, voice)
		}
	}
	m.voices = live
}
func (m *musicSystem) close() {
	if m != nil {
		for _, v := range m.voices {
			v.player.Close()
		}
		m.voices = nil
	}
}

// Infinite PCM reader: no silence insertion or decoder restart at the seam.
type loopPCMReader struct {
	data []byte
	at   int
}

func (r *loopPCMReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := len(p)
	for len(p) > 0 {
		copied := copy(p, r.data[r.at:])
		p = p[copied:]
		r.at = (r.at + copied) % len(r.data)
	}
	return n, nil
}

// Only visible scenes may select danger music. Hidden CombatActive during sleep
// never reveals an encounter before the player is shown the tactical screen.
func sceneMusic(s game.State) string {
	screen := s.Screen
	if screen == game.ScreenCharacterLibrary && s.LibraryReturn != "" {
		screen = s.LibraryReturn
	}
	if screen == game.ScreenCharacterStatus {
		screen = s.StatusReturn
	}
	if screen == game.ScreenOutcome {
		if s.OutcomeKey == "ui.outcome.failure" {
			return "defeat"
		}
		return "ending"
	}
	if screen == game.ScreenScore {
		return "ending"
	}
	switch screen {
	case game.ScreenTitle, game.ScreenCharacterLibrary, game.ScreenCharacterCreation, game.ScreenAttributeAllocation:
		return "title"
	case game.ScreenTactical:
		if s.CombatActive {
			return "combat"
		}
		return "flight"
	case game.ScreenArena:
		return "combat"
	case game.ScreenNavigation, game.ScreenFlight:
		return "flight"
	case game.ScreenGround:
		if s.GroundWorld {
			return "surface"
		}
		return "port"
	case game.ScreenInterior, game.ScreenTrade, game.ScreenUniteller, game.ScreenDialog, game.ScreenSlots, game.ScreenTubeStation, game.ScreenParts, game.ScreenDepot, game.ScreenFuelPurchase, game.ScreenFTLBoost:
		return "port"
	default:
		return "ship"
	}
}
func (a *app) toggleMusic() {
	if a.music == nil {
		return
	}
	a.music.enabled = !a.music.enabled
	a.options.statusKey = "ui.music.on"
	if !a.music.enabled {
		a.options.statusKey = "ui.music.off"
	}
	a.options.statusDetail = ""
	a.savePreferences()
}
func (a *app) musicValue() string {
	if a.music != nil && a.music.enabled {
		return a.lookup("ui.music.on")
	}
	return a.lookup("ui.music.off")
}
