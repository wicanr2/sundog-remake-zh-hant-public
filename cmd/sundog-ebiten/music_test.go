package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/prefs"
)

type fakeMusicPlayer struct {
	playing, closed bool
	volume          float64
}

func (p *fakeMusicPlayer) Play()               { p.playing = true }
func (p *fakeMusicPlayer) Pause()              { p.playing = false }
func (p *fakeMusicPlayer) Close() error        { p.closed = true; p.playing = false; return nil }
func (p *fakeMusicPlayer) SetVolume(v float64) { p.volume = v }

func TestMusicLoopHasNoInsertedSilence(t *testing.T) {
	r := &loopPCMReader{data: []byte{1, 2, 3, 4, 5, 6, 7, 8}}
	out := make([]byte, 35)
	if _, err := io.ReadFull(r, out); err != nil {
		t.Fatal(err)
	}
	for i, v := range out {
		if v != byte(i%8+1) {
			t.Fatal("loop discontinuity", i)
		}
	}
	if n, err := (&loopPCMReader{}).Read(out); n != 0 || err != io.EOF {
		t.Fatal("empty PCM did not terminate")
	}
}
func TestMusicTransitionsBoundedAndMuteResumes(t *testing.T) {
	var created []*fakeMusicPlayer
	m := &musicSystem{enabled: true, failed: map[string]bool{}, open: func(string) (musicPlayer, error) {
		p := &fakeMusicPlayer{}
		created = append(created, p)
		return p, nil
	}}
	now := time.Unix(1, 0)
	for i := 0; i < 300; i++ {
		cue := []string{"ship", "flight", "combat"}[i%3]
		m.update(cue, now)
		now = now.Add(time.Second / 60)
		live := 0
		for _, p := range created {
			if !p.closed {
				live++
			}
		}
		if live > 2 {
			t.Fatal("unbounded music players", live)
		}
	}
	for i := 0; i < 120; i++ {
		m.update("ship", now)
		now = now.Add(time.Second / 60)
	}
	if len(m.voices) != 1 {
		t.Fatal("fade did not dispose old player")
	}
	saved := m.voices[0].player
	m.enabled = false
	for i := 0; i < 120; i++ {
		m.update("ship", now)
		now = now.Add(time.Second / 60)
	}
	if len(m.voices) != 1 || m.voices[0].gain != 0 || saved.(*fakeMusicPlayer).playing {
		t.Fatal("mute did not finish silently")
	}
	m.enabled = true
	m.update("ship", now)
	if m.voices[0].player != saved || !saved.(*fakeMusicPlayer).playing {
		t.Fatal("same cue did not resume")
	}
	m.close()
	for _, p := range created {
		if !p.closed {
			t.Fatal("player leaked")
		}
	}
}
func TestMissingMusicIsNonFatalAndNotRetriedEachFrame(t *testing.T) {
	count := 0
	m := &musicSystem{enabled: true, failed: map[string]bool{}, open: func(string) (musicPlayer, error) { count++; return nil, fmt.Errorf("missing") }}
	for i := 0; i < 60; i++ {
		m.update("ship", time.Unix(1, int64(i)*16666666))
	}
	if count != 1 || len(m.voices) != 0 {
		t.Fatal("failed source repeatedly opened")
	}
	if got := newMusicSystem(nil, t.TempDir()); len(got.tracks) != 0 {
		t.Fatal("missing catalog was populated")
	}
}
func TestVisibleMusicScenesAndHiddenEncounterIsolation(t *testing.T) {
	cases := []struct {
		s    game.State
		want string
	}{
		{game.State{Screen: game.ScreenTitle}, "title"},
		{game.State{Screen: game.ScreenShipboard}, "ship"},
		{game.State{Screen: game.ScreenEngineering}, "ship"},
		{game.State{Screen: game.ScreenGround}, "port"},
		{game.State{Screen: game.ScreenGround, GroundWorld: true}, "surface"},
		{game.State{Screen: game.ScreenTrade}, "port"},
		{game.State{Screen: game.ScreenFuelPurchase}, "port"},
		{game.State{Screen: game.ScreenFTLBoost}, "port"},
		{game.State{Screen: game.ScreenNavigation}, "flight"},
		{game.State{Screen: game.ScreenFlight}, "flight"},
		{game.State{Screen: game.ScreenTactical}, "flight"},
		{game.State{Screen: game.ScreenTactical, CombatActive: true}, "combat"},
		{game.State{Screen: game.ScreenArena}, "combat"},
		{game.State{Screen: game.ScreenOutcome, OutcomeKey: "ui.outcome.failure"}, "defeat"},
		{game.State{Screen: game.ScreenOutcome, OutcomeKey: "ui.outcome.success"}, "ending"},
		{game.State{Screen: game.ScreenScore}, "ending"},
		{game.State{Screen: game.ScreenCharacterLibrary, LibraryReturn: game.ScreenEngineering}, "ship"},
		{game.State{Screen: game.ScreenCharacterStatus, StatusReturn: game.ScreenFlight, Sleep: game.SleepState{Active: true}, CombatActive: true}, "flight"},
	}
	for _, tc := range cases {
		before, _ := json.Marshal(tc.s)
		if got := sceneMusic(tc.s); got != tc.want {
			t.Errorf("%s: %s want %s", tc.s.Screen, got, tc.want)
		}
		after, _ := json.Marshal(tc.s)
		if !bytes.Equal(before, after) {
			t.Fatal("routing changed state")
		}
	}
}
func TestMusicPreferencePersistsWithoutTouchingGameOrSFX(t *testing.T) {
	a := newOptionsTestApp(t)
	a.music = &musicSystem{enabled: true}
	a.sounds = &soundboard{muted: true}
	before := a.state
	a.toggleMusic()
	if a.music.enabled || !reflect.DeepEqual(before, a.state) || !a.sounds.muted {
		t.Fatal("music changed gameplay or SFX")
	}
	saved, err := prefs.Load(a.settingsPath)
	if err != nil || saved.MusicEnabled == nil || *saved.MusicEnabled {
		t.Fatal("off not persisted", err)
	}
	b := newOptionsTestApp(t)
	b.music = &musicSystem{enabled: true}
	applyPreferences(b, saved)
	if b.music.enabled {
		t.Fatal("off did not survive reopen")
	}
	b.toggleMusic()
	saved, err = prefs.Load(b.settingsPath)
	if err != nil || saved.MusicEnabled == nil || !*saved.MusicEnabled {
		t.Fatal("on not persisted")
	}
	applyPreferences(b, prefs.Preferences{})
	if !b.music.enabled {
		t.Fatal("old settings lost default")
	}
}
func TestShippedMusicPCMAndStereo(t *testing.T) {
	dir := filepath.Join("..", "..", "assets", "music")
	m := newMusicSystem(nil, dir)
	if len(m.tracks) != 8 {
		t.Fatal("music set incomplete", len(m.tracks))
	}
	for id, track := range m.tracks {
		raw, err := os.ReadFile(filepath.Join(dir, track.File))
		if err != nil {
			t.Fatal(err)
		}
		pcm, err := decodeOgg(raw)
		if err != nil {
			t.Fatal(id, err)
		}
		if len(pcm) != track.Frames*4 {
			t.Fatal(id, "decoded frames", len(pcm)/4, "want", track.Frames)
		}
		var peak int
		stereo := false
		for i := 0; i < len(pcm); i += 4 {
			left := int(int16(binary.LittleEndian.Uint16(pcm[i:])))
			right := int(int16(binary.LittleEndian.Uint16(pcm[i+2:])))
			if left != right {
				stereo = true
			}
			if left < 0 {
				left = -left
			}
			if right < 0 {
				right = -right
			}
			if left > peak {
				peak = left
			}
			if right > peak {
				peak = right
			}
		}
		if peak < 100 || peak >= 32767 || !stereo {
			t.Fatal(id, "silent/clipped/dual mono", peak, stereo)
		}
		for channel := 0; channel < 2; channel++ {
			first := int(int16(binary.LittleEndian.Uint16(pcm[channel*2:])))
			last := int(int16(binary.LittleEndian.Uint16(pcm[len(pcm)-4+channel*2:])))
			diff := first - last
			if diff < 0 {
				diff = -diff
			}
			if diff > 1000 {
				t.Error(id, "loop seam step", diff)
			}
		}
	}
}
