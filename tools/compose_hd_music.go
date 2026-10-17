//go:build ignore

// Project-authored score, MIDI writer and owned synthesis. See assets/music/README.md.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

const rate = 44100

type event struct {
	Beat, Length  float64
	Note          int
	Velocity, Pan float64
	Voice         string
	Layer         int
}
type cue struct {
	ID, Title string
	BPM, Bars int
	Density   float64
	Harmony   [][]int
}
type track struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	File    string  `json:"file"`
	SHA256  string  `json:"sha256"`
	Frames  int     `json:"frames"`
	BPM     int     `json:"bpm"`
	Bars    int     `json:"bars"`
	Seconds float64 `json:"seconds"`
}

var harmony = [][]int{{50, 57, 60, 64}, {53, 57, 60, 64}, {48, 55, 59, 64}, {43, 55, 59, 64}}
var motif = []int{62, 69, 72, 76, 71}

func main() {
	out := flag.String("out", "assets/music", "formal assets")
	work := flag.String("work", "workplace/hd-music-render", "intermediates")
	flag.Parse()
	if _, err := os.Stat("/.dockerenv"); err != nil {
		panic("Docker only")
	}
	must(os.MkdirAll(*out, 0755))
	must(os.MkdirAll(*work, 0755))
	cues := []cue{{"title", "自由的契約", 86, 24, .6, harmony}, {"ship", "老船仍在呼吸", 72, 24, .2, harmony}, {"port", "貨單與霓虹", 100, 32, .8, harmony}, {"surface", "異星的風", 76, 24, .15, harmony}, {"flight", "星圖之外", 90, 32, .6, harmony}, {"combat", "不要失去這艘船", 120, 32, 1, [][]int{{50, 57, 60, 64}, {46, 53, 58, 62}, {48, 55, 60, 63}, {45, 52, 57, 60}}}, {"ending", "下一趟航行", 84, 16, .4, [][]int{{53, 57, 60, 64}, {48, 55, 59, 64}, {55, 59, 62, 64}, {50, 57, 60, 64}}}, {"defeat", "未償的契約", 64, 16, .05, harmony}}
	var tracks []track
	for _, c := range cues {
		events := compose(c)
		frames := int(math.Round(float64(c.Bars*4) * 60 / float64(c.BPM) * rate))
		layers := make([][]float32, 4)
		for l := range layers {
			layers[l] = make([]float32, frames*2)
		}
		for i, e := range events {
			render(layers[e.Layer], frames, c.BPM, e, uint32(20261017+i*7919))
		}
		// Cyclic stereo delays retain their tails at every loop boundary.
		delays := []float64{.173, .317, .461}
		gains := []float32{.13, .08, .04}
		for _, layer := range layers {
			original := append([]float32(nil), layer...)
			for d, seconds := range delays {
				offset := int(seconds * rate)
				for n := 0; n < frames; n++ {
					from := (n - offset + frames) % frames
					layer[n*2] += original[from*2+1] * gains[d]
					layer[n*2+1] += original[from*2] * gains[d]
				}
			}
		}
		mix := make([]float32, frames*2)
		peak := 0.0
		for i := range mix {
			for l := range layers {
				mix[i] += layers[l][i]
			}
			peak = math.Max(peak, math.Abs(float64(mix[i])))
		}
		gain := .66 / math.Max(peak, .001)
		for i := range mix {
			mix[i] *= float32(gain)
		}
		wav := filepath.Join(*work, c.ID+".wav")
		writeWAV(wav, mix)
		for l, layer := range layers {
			for i := range layer {
				layer[i] *= float32(gain)
			}
			writeWAV(filepath.Join(*work, fmt.Sprintf("%s-stem-%d.wav", c.ID, l)), layer)
		}
		ogg := filepath.Join(*out, c.ID+".ogg")
		cmd := exec.Command("ffmpeg", "-v", "error", "-y", "-i", wav, "-c:a", "libvorbis", "-q:a", "5", ogg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		must(cmd.Run())
		writeMIDI(filepath.Join(*out, c.ID+".mid"), c, events)
		saveJSON(filepath.Join(*out, c.ID+"-score.json"), map[string]any{"identity": "new remake composition; no original melody/sample", "cue": c, "motifMIDI": motif, "events": events, "layers": []string{"harmony", "melody", "bass", "pulse"}})
		raw, err := os.ReadFile(ogg)
		must(err)
		h := sha256.Sum256(raw)
		tracks = append(tracks, track{c.ID, c.Title, c.ID + ".ogg", hex.EncodeToString(h[:]), frames, c.BPM, c.Bars, float64(frames) / rate})
		fmt.Printf("%s: %.3fs, %d events\n", c.ID, float64(frames)/rate, len(events))
	}
	source, err := os.ReadFile("tools/compose_hd_music.go")
	must(err)
	hash := sha256.Sum256(source)
	saveJSON(filepath.Join(*out, "manifest.json"), map[string]any{"schema": 1, "identity": "entirely new SunDog remake music", "license": "RRSAL-1.0", "source": "tools/compose_hd_music.go", "sourceSHA256": hex.EncodeToString(hash[:]), "sourceInputs": "authored notes and owned oscillator/noise synthesis; no external samples, SoundFont or music service", "originalBGM": "unknown; not investigated", "sampleRate": rate, "channels": 2, "codec": "Ogg Vorbis quality5", "loop": "whole decoded track, gapless wrap; cyclic reverberation", "date": "2026-10-17", "tracks": tracks, "listeningReview": "pending; technical analysis is not listening approval"})
}
func compose(c cue) []event {
	var es []event
	add := func(b, d float64, n int, v, p float64, voice string, l int) {
		es = append(es, event{b, d, n, v, p, voice, l})
	}
	for bar := 0; bar < c.Bars; bar++ {
		chord := c.Harmony[(bar/2)%len(c.Harmony)]
		b := float64(bar * 4)
		section := (bar / 8) % 3
		if bar%2 == 0 {
			for i, n := range chord {
				add(b, 8.5, n, .17, -.65+float64(i)*.43, "pad", 0)
			}
		}
		if c.ID != "surface" && c.ID != "defeat" {
			add(b, 1.6, chord[0]-12, .23, 0, "bass", 2)
			if c.Density > .5 {
				add(b+2.5, 1.25, chord[0]-12, .18, 0, "bass", 2)
			}
		}
		if c.ID == "port" {
			for _, offset := range []float64{.5, 1.75, 3} {
				for i, n := range chord {
					add(b+offset, .8, n+12, .11, float64(i-2)*.2, "keys", 0)
				}
			}
		}
		if c.ID == "flight" || c.ID == "combat" {
			for step := 0; step < 8; step++ {
				n := chord[step%len(chord)] + 12
				add(b+float64(step)*.5, .4, n, .08, -.55+float64(step%3)*.55, "pluck", 3)
			}
		}
		if bar%4 == 0 || (c.ID == "combat" && bar%2 == 0) {
			for i, n := range motif {
				if c.ID == "defeat" && i == 4 {
					continue
				}
				if c.ID == "combat" && n == 71 {
					n = 70
				}
				if section == 1 && i < 3 {
					n -= 12
				}
				length := 1.0
				offset := float64(i) * 1.25
				if c.ID == "ship" || c.ID == "surface" || c.ID == "defeat" {
					length = 2.3
					offset = float64(i) * 1.5
					n -= 12
				}
				if c.ID == "combat" {
					length = .4
					offset = float64(i) * .5
				}
				voice := "keys"
				if c.ID == "surface" || c.ID == "flight" {
					voice = "bell"
				}
				add(b+offset, length, n, .2, -.3+float64(i%3)*.3, voice, 1)
			}
		}
		if c.Density >= .4 && (section != 0 || c.ID == "port" || c.ID == "combat") {
			for _, offset := range []float64{0, 2} {
				add(b+offset, .3, 36, .22, 0, "kick", 3)
			}
			if c.Density > .7 {
				for _, offset := range []float64{1, 3} {
					add(b+offset, .18, 38, .12, .1, "snare", 3)
				}
			}
			for k := 0; k < 8; k++ {
				add(b+float64(k)*.5, .07, 42, .045, -.4+float64(k%2)*.8, "hat", 3)
			}
		}
	}
	return es
}
func render(dst []float32, frames, bpm int, e event, seed uint32) {
	start := int(math.Round(e.Beat * 60 / float64(bpm) * rate))
	length := e.Length * 60 / float64(bpm)
	tail := .35
	if e.Voice == "pad" {
		tail = 1.4
	}
	count := int((length + tail) * rate)
	frequency := 440 * math.Pow(2, float64(e.Note-69)/12)
	left := math.Sqrt((1 - e.Pan) / 2)
	right := math.Sqrt((1 + e.Pan) / 2)
	previous := 0.0
	for i := 0; i < count; i++ {
		t := float64(i) / rate
		phase := 2 * math.Pi * frequency * t
		attack := math.Min(1, t/.012)
		release := math.Max(0, math.Min(1, (length+tail-t)/tail))
		env := attack * release
		v := 0.0
		switch e.Voice {
		case "pad":
			env = math.Min(1, t/.8) * release
			v = (math.Sin(phase*.998)+math.Sin(phase*1.002))*.34 + math.Sin(phase*2)*.12 + math.Sin(phase*3)*.05
		case "keys":
			v = math.Sin(phase+1.4*math.Exp(-t*3)*math.Sin(2*phase))*.7*math.Exp(-t*1.3) + math.Sin(phase*3)*.08*math.Exp(-t*5)
		case "bell":
			v = (math.Sin(phase) + .25*math.Sin(phase*2.01) + .1*math.Sin(phase*3.99)) * .65 * math.Exp(-t*1.1)
		case "bass":
			v = (math.Sin(phase) + .2*math.Sin(phase*2) + .07*math.Sin(phase*3)) * .7 * math.Exp(-t*.7)
		case "pluck":
			v = (math.Sin(phase) + .3*math.Sin(2*phase) + .15*math.Sin(3*phase)) * .6 * math.Exp(-t*6)
		case "kick":
			v = math.Sin(2*math.Pi*(52*t+2.2*(1-math.Exp(-40*t)))) * math.Exp(-t*19)
		case "snare", "hat":
			seed ^= seed << 13
			seed ^= seed >> 17
			seed ^= seed << 5
			noise := float64(seed)/2147483648 - 1
			high := noise - previous
			previous = noise
			if e.Voice == "hat" {
				v = high * .3 * math.Exp(-t*70)
			} else {
				v = (high*.35 + math.Sin(phase)*.2) * math.Exp(-t*28)
			}
		}
		value := v * env * e.Velocity
		at := (start + i) % frames
		dst[at*2] += float32(value * left)
		dst[at*2+1] += float32(value * right)
	}
}
func writeWAV(path string, samples []float32) {
	b := new(bytes.Buffer)
	b.WriteString("RIFF")
	binary.Write(b, binary.LittleEndian, uint32(36+len(samples)*2))
	b.WriteString("WAVEfmt ")
	binary.Write(b, binary.LittleEndian, uint32(16))
	for _, v := range []any{uint16(1), uint16(2), uint32(rate), uint32(rate * 4), uint16(4), uint16(16)} {
		binary.Write(b, binary.LittleEndian, v)
	}
	b.WriteString("data")
	binary.Write(b, binary.LittleEndian, uint32(len(samples)*2))
	for _, s := range samples {
		v := int(math.Round(float64(s) * 32767))
		if v > 32767 || v < -32768 {
			panic("clipping")
		}
		binary.Write(b, binary.LittleEndian, int16(v))
	}
	must(os.WriteFile(path, b.Bytes(), 0644))
}
func vlq(n int) []byte {
	r := []byte{byte(n & 127)}
	for n >>= 7; n > 0; n >>= 7 {
		r = append([]byte{byte(n&127) | 128}, r...)
	}
	return r
}
func writeMIDI(path string, c cue, events []event) {
	type msg struct {
		tick int
		data []byte
	}
	tracks := [][]byte{}
	tempo := 60000000 / c.BPM
	head := []byte{0, 255, 81, 3, byte(tempo >> 16), byte(tempo >> 8), byte(tempo), 0, 255, 88, 4, 4, 2, 24, 8}
	head = append(head, vlq(c.Bars*4*480)...)
	head = append(head, 255, 47, 0)
	tracks = append(tracks, head)
	for layer := 0; layer < 4; layer++ {
		var msgs []msg
		for _, e := range events {
			if e.Layer != layer {
				continue
			}
			channel := byte(layer)
			if e.Voice == "kick" || e.Voice == "snare" || e.Voice == "hat" {
				channel = 9
			}
			msgs = append(msgs, msg{int(e.Beat * 480), []byte{144 | channel, byte(e.Note), byte(e.Velocity * 127 * 2)}}, msg{int((e.Beat + e.Length) * 480), []byte{128 | channel, byte(e.Note), 0}})
		}
		sort.SliceStable(msgs, func(i, j int) bool { return msgs[i].tick < msgs[j].tick })
		b := new(bytes.Buffer)
		b.Write([]byte{0, 192 | byte(layer), []byte{89, 4, 38, 81}[layer]})
		last := 0
		for _, m := range msgs {
			b.Write(vlq(m.tick - last))
			b.Write(m.data)
			last = m.tick
		}
		end := c.Bars * 4 * 480
		if end < last {
			end = last
		}
		b.Write(vlq(end - last))
		b.Write([]byte{255, 47, 0})
		tracks = append(tracks, b.Bytes())
	}
	b := new(bytes.Buffer)
	b.WriteString("MThd")
	binary.Write(b, binary.BigEndian, uint32(6))
	binary.Write(b, binary.BigEndian, uint16(1))
	binary.Write(b, binary.BigEndian, uint16(len(tracks)))
	binary.Write(b, binary.BigEndian, uint16(480))
	for _, t := range tracks {
		b.WriteString("MTrk")
		binary.Write(b, binary.BigEndian, uint32(len(t)))
		b.Write(t)
	}
	must(os.WriteFile(path, b.Bytes(), 0644))
}
func saveJSON(path string, v any) {
	b, e := json.MarshalIndent(v, "", "  ")
	must(e)
	must(os.WriteFile(path, append(b, '\n'), 0644))
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
