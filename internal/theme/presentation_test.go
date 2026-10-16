package theme

import "testing"

// TestPresentationsAreDistinctAndComplete: 每個預設都要指到註冊表裡真的存在的
// 配色與圖塊，而且三個預設不能有兩個是同一組三元組——否則循環時會有一步看不出變化。
func TestPresentationsAreDistinctAndComplete(t *testing.T) {
	registry, err := NewPrototypeRegistry()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[[3]string]PresentationID{}
	for _, preset := range Presentations() {
		if _, ok := registry.byChrome[preset.Chrome]; !ok {
			t.Fatalf("%s 的配色 %q 不在註冊表裡", preset.ID, preset.Chrome)
		}
		if _, ok := registry.byTileset[preset.Tileset]; !ok {
			t.Fatalf("%s 的圖塊 %q 不在註冊表裡", preset.ID, preset.Tileset)
		}
		if !KnownLayout(preset.Layout) {
			t.Fatalf("%s 的版面 %q 不是已知版面", preset.ID, preset.Layout)
		}
		key := [3]string{string(preset.Chrome), string(preset.Tileset), string(preset.Layout)}
		if other, clash := seen[key]; clash {
			t.Fatalf("%s 與 %s 是同一組三元組", preset.ID, other)
		}
		seen[key] = preset.ID
	}
}

// TestNoPresetPutsTheModernRasterUnderTheAtariSTLayout: 這是 2026-09-03 量到會畫壞的
// 那一格——重製版的高解析地圖會浮貼在原版滿版城市上。預設集存在的理由就是它。
func TestNoPresetPutsTheModernRasterUnderTheAtariSTLayout(t *testing.T) {
	for _, preset := range Presentations() {
		if preset.Layout == AtariSTLayout && preset.Tileset == ModernTiles {
			t.Fatalf("%s 把 Remake Modern 圖塊放進 Atari ST 版面", preset.ID)
		}
	}
}

// TestSnapRefusesCombinationsNobodyDesigned: 舊設定檔或手改的檔案不能落在沒人設計過的
// 組合上，尤其不能落在會畫壞的那一格。
func TestSnapRefusesCombinationsNobodyDesigned(t *testing.T) {
	cases := []struct {
		name    string
		chrome  ID
		tileset TilesetID
		layout  LayoutID
		want    PresentationID
	}{
		{"會畫壞的那一格", AtariSTOriginal, ModernTiles, AtariSTLayout, AtariSTPresentation},
		{"ST 版面配 Apple II 圖塊", AppleIIOriginal, AppleIITiles, AtariSTLayout, AtariSTPresentation},
		{"退役HD圖塊回到Atari ST", AtariSTOriginal, ModernTiles, RemakeLayout, AtariSTPresentation},
		{"重製版版面配 Apple II 圖塊", AtariSTOriginal, AppleIITiles, RemakeLayout, AppleIIPresentation},
		{"空值", "", "", "", AtariSTPresentation},
	}
	for _, testCase := range cases {
		got := SnapPresentation(testCase.chrome, testCase.tileset, testCase.layout)
		if got.ID != testCase.want {
			t.Errorf("%s：snap 得到 %s，預期 %s", testCase.name, got.ID, testCase.want)
		}
	}
}

// TestSnapKeepsAnExactTriple: 三個預設自己 snap 回自己，設定檔往返不會漂移。
func TestSnapKeepsAnExactTriple(t *testing.T) {
	for _, preset := range Presentations() {
		got := SnapPresentation(preset.Chrome, preset.Tileset, preset.Layout)
		if got.ID != preset.ID {
			t.Fatalf("%s snap 成 %s", preset.ID, got.ID)
		}
	}
}

// TestNextPresentationWrapsAndRecovers: 循環會繞回來；不認得的值回到預設，
// 免得壞掉的設定把玩家卡在循環外面。
func TestNextPresentationWrapsAndRecovers(t *testing.T) {
	presets := Presentations()
	id := presets[0].ID
	for range presets {
		id = NextPresentation(id).ID
	}
	if id != presets[0].ID {
		t.Fatalf("循環一圈之後停在 %s", id)
	}
	if got := NextPresentation("nonsense").ID; got != DefaultPresentation().ID {
		t.Fatalf("不認得的值給了 %s", got)
	}
}
