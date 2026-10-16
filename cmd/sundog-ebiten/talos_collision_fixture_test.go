package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
)

// 此測試只匯出現行場景及判定供 Talos 執行原版指令比較；不以自行算出的
// Pass 欄位作單元測試期望值。完整入口見 tools/verify_talos_ground_collision.py。
func TestExportTalosGroundCollisionFixtures(t *testing.T) {
	diskPath, output := os.Getenv("SUNDOG_PARITY_DISK"), os.Getenv("SUNDOG_PARITY_OUTPUT")
	if diskPath == "" && output == "" {
		t.Skip("請由 tools/verify_talos_ground_collision.py 指定本機磁碟與輸出")
	}
	if diskPath == "" || output == "" {
		t.Fatal("原版磁碟與輸出路徑必須同時指定")
	}
	raw, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	if digest != "f7c541b3cffdc7befc5641d667f039a8b33b0dfc406d17b20844158e52155fd6" {
		t.Fatal("原版磁碟雜湊與本次原生指令契約不符")
	}
	disk, err := originalmedia.Open(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	patterns, err := disk.CollidePatterns()
	if err != nil {
		t.Fatal(err)
	}
	font, err := disk.Font()
	if err != nil {
		t.Fatal(err)
	}
	includeMarks := os.Getenv("SUNDOG_PARITY_DRAWN_MARKS") == "1"
	podPatterns := os.Getenv("SUNDOG_PARITY_POD_PATTERNS") == "1"
	patternIDs := []int{4, 6, 7, 8, 9}
	if podPatterns {
		patternIDs = []int{0, 1, 2, 3}
	}
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	type sample struct {
		Pattern, X, Y int
		Pass          bool
	}
	type sceneFixture struct {
		Name     string
		CityID   string
		PlanetID string
		World    bool
		Origin   game.Position
		Indices  []byte
		Blocked  uint16
		Samples  []sample
	}
	fixture := struct {
		Schema      int
		DiskSHA256  string
		Comparison  string
		Randomness  string
		PodPatterns bool `json:"podPatterns,omitempty"`
		Scenes      []sceneFixture
	}{Schema: 1, DiskSHA256: digest,
		PodPatterns: podPatterns,
		Comparison:  "same-input-native-pixel-collision; not normal-player-path or full-scene parity",
		Randomness:  "remake appearance fixed by scene identity/origin/rebuild=0; identical pixel input supplied to native code; collision consumes no RNG"}
	appendScene := func(name string, state game.State, origin game.Position) {
		if podPatterns {
			state.GroundMode = game.GroundInPod
		}
		a := &app{state: state, reducer: r, originalDisk: disk, originalTiles: map[int]*originalTileCache{}}
		scene := a.buildSTGroundScene(state.CityID, origin.X, origin.Y, 0)
		if scene == nil || len(scene.indices) != logicalWidth*logicalHeight {
			t.Fatalf("%s：正式場景建構失敗", name)
		}
		f := sceneFixture{Name: name, CityID: state.CityID, PlanetID: state.PlanetID,
			World: state.GroundWorld, Origin: origin, Indices: append([]byte(nil), scene.indices...), Blocked: scene.blocked}
		if !state.GroundWorld {
			for index, pass := range stGroundPassable {
				if !pass {
					f.Blocked |= 1 << index
				}
			}
		}
		if podPatterns {
			f.Blocked |= 1<<7 | 1<<8
		}
		// 同時涵蓋 16 像素群組邊界、城市／地表格線與畫面內四邊。
		for _, y := range []int{8, 39, 40, 79, 80, 119, 159, 187} {
			for _, x := range []int{8, 31, 32, 63, 64, 95, 127, 159, 191, 223, 255, 287, 310} {
				for _, p := range patternIDs {
					for _, point := range patterns[p] {
						sx, sy := x-4+point.X, y-4+point.Y
						if sx < 0 || sx >= logicalWidth || sy < 0 || sy >= logicalHeight {
							t.Fatal("夾具越過原版螢幕取樣前提")
						}
					}
					f.Samples = append(f.Samples, sample{p, x, y, scene.canStand(patterns, p, x, y)})
				}
			}
		}
		if includeMarks {
			// drawSTGroundMarks／drawSTChar 的實際字元與 pen 1；原版 collide
			// 讀取 glyph 疊完的 framebuffer，不能只比較底圖索引。
			for _, mark := range scene.marks {
				glyph := font.Glyph(mark.ch)
				for gy := 0; gy < originalmedia.GlyphHeight; gy++ {
					for gx := 0; gx < glyph.Width; gx++ {
						if !glyph.Set(gx, gy) {
							continue
						}
						sx, sy := mark.x-3+gx, mark.y-4+gy
						if sx < 0 || sx >= logicalWidth || sy < 0 || sy >= logicalHeight {
							continue
						}
						// VDI pen 1 對應硬體索引 15；不可把 pen 編號當索引。
						f.Indices[sy*logicalWidth+sx] = 15
						for _, p := range patternIDs {
							x, y := sx+4-patterns[p][0].X, sy+4-patterns[p][0].Y
							if x < 4 || x > 313 || y < 4 || y > 193 {
								continue
							}
							f.Samples = append(f.Samples, sample{p, x, y, scene.canStand(patterns, p, x, y)})
						}
					}
				}
			}
		}
		if podPatterns {
			seen := map[[3]int]bool{}
			unique := make([]sample, 0, len(f.Samples))
			for _, row := range f.Samples {
				key := [3]int{row.Pattern, row.X, row.Y}
				if !seen[key] {
					unique = append(unique, row)
					seen[key] = true
				}
			}
			f.Samples = unique
		}
		fixture.Scenes = append(fixture.Scenes, f)
		if scene.picture != nil {
			scene.picture.Deallocate()
		}
	}
	for _, city := range r.World.Destinations() {
		// 使用明示 typed 前置狀態，避免 NewState 的時鐘初始化；碰撞本身不擲骰。
		state := game.State{ShipCityID: city.CityID, ShipPosition: game.Position{X: 12, Y: 7}}
		state.Screen, state.SystemID, state.PlanetID, state.CityID = game.ScreenGround, city.SystemID, city.PlanetID, city.CityID
		state.Position = game.Position{X: 12, Y: 7}
		appendScene("city/"+city.CityID, state, game.Position{X: 8, Y: 4})
	}
	for _, planet := range r.World.Planets() {
		cities := r.World.DestinationsOnPlanet(planet.ID)
		if len(cities) == 0 {
			t.Fatalf("行星 %s 缺少可核對城市", planet.ID)
		}
		state := game.State{ShipCityID: cities[0].CityID, ShipPosition: game.Position{X: 12, Y: 7}}
		state.Screen, state.SystemID, state.PlanetID, state.CityID = game.ScreenGround, planet.SystemID, planet.ID, cities[0].CityID
		state.GroundWorld = true
		state.Position = game.Position{X: cities[0].MapColumn, Y: cities[0].MapRow}
		origin := game.GroundWrap(state, game.Position{X: state.Position.X - 5, Y: state.Position.Y - 3})
		appendScene("surface/"+planet.ID, state, origin)
	}
	// 原始 60 城市槽含兩個保留槽；typed 世界保留 58 個可用城市。
	if len(fixture.Scenes) != 76 {
		t.Fatalf("場景 %d 組，不符來源 58 城市＋18 行星", len(fixture.Scenes))
	}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "collision-fixtures.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("匯出 %d 場景，每場景至少 416 案，含後繪物件=%v；原版磁碟 %s", len(fixture.Scenes), includeMarks, digest)
}

// 比較原始模式初始化後的collide，正式場景仍由建構入口建立。
func TestExportTalosPodCollisionColors(t *testing.T) {
	diskPath, output := os.Getenv("SUNDOG_PARITY_DISK"), os.Getenv("SUNDOG_PARITY_OUTPUT")
	if output == "" {
		t.Skip("requires native pod color receipt")
	}
	disk, err := originalmedia.Open(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	patterns, err := disk.CollidePatterns()
	if err != nil {
		t.Fatal(err)
	}
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	var native struct {
		Rows []struct {
			Foot           bool
			Before         [16]byte
			Pattern, Color int
			Pass           bool
		}
	}
	raw, err := os.ReadFile(filepath.Join(output, "native-pod-collision-colors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &native); err != nil {
		t.Fatal(err)
	}
	if len(native.Rows) != 64 {
		t.Fatal("incomplete native pod color inputs")
	}
	report := struct{ Cases, Differences int }{}
	for _, row := range native.Rows {
		state := game.State{Screen: game.ScreenGround, CityID: "drahew", ShipCityID: "drahew", ShipPosition: game.Position{X: 1, Y: 0}, GroundMode: game.GroundOnFoot}
		if !row.Foot {
			state.GroundMode = game.GroundInPod
		}
		a := &app{state: state, reducer: r, originalDisk: disk, originalTiles: map[int]*originalTileCache{}}
		scene := a.buildSTGroundScene("drahew", -3, -2, 0)
		if scene == nil {
			t.Fatal("real scene constructor failed")
		}
		// 僅控制待比較像素與初始遮罩。場景的模式設定由正式建構器決定。
		scene.world = true
		for i, v := range row.Before {
			if v != 0 {
				scene.blocked |= 1 << i
			}
		}
		for i := range scene.indices {
			scene.indices[i] = byte(row.Color)
		}
		pass := scene.canStand(patterns, row.Pattern, 100, 100)
		report.Cases++
		if pass != row.Pass {
			report.Differences++
		}
		scene.picture.Deallocate()
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "remake-pod-collision-colors.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if report.Differences != 0 {
		t.Fatalf("pod color cases%d differences%d", report.Cases, report.Differences)
	}
}
