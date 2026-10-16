package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/catalog"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/localization"
	"github.com/wicanr2/sundog-remake/internal/prefs"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/theme"
)

func newOptionsTestApp(t *testing.T) *app {
	t.Helper()
	reducer, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	return &app{reducer: reducer, state: game.NewState(), settingsPath: filepath.Join(t.TempDir(), "settings.json")}
}

func TestLanguageCyclesImmediatelyAndPersistsOutsideSave(t *testing.T) {
	a := newOptionsTestApp(t)
	a.languages = map[localization.Locale]languagePack{}
	for _, locale := range localization.Locales() {
		a.languages[locale] = languagePack{labels: catalog.Catalog{
			Locale: string(locale),
			Entries: map[string]catalog.Entry{
				"probe": {Key: "probe", Translation: string(locale)},
			},
		}}
	}
	if !a.applyLocale(localization.TraditionalChinese) {
		t.Fatal("cannot apply Traditional Chinese fixture")
	}
	schema := a.state.Schema
	a.cycleLocale(1)
	if a.locale != localization.SimplifiedChinese || a.lookup("probe") != "zh-Hans" {
		t.Fatalf("locale=%q, probe=%q", a.locale, a.lookup("probe"))
	}
	if a.state.Schema != schema {
		t.Fatalf("language switch changed save schema %d -> %d", schema, a.state.Schema)
	}
	saved, err := prefs.Load(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Locale != "zh-Hans" {
		t.Fatalf("settings locale = %q", saved.Locale)
	}
	for _, want := range []localization.Locale{localization.Japanese, localization.Korean, localization.English, localization.TraditionalChinese} {
		before := a.state
		a.cycleLocale(1)
		if a.locale != want || a.lookup("probe") != string(want) {
			t.Fatalf("language cycle = %q, want %q", a.locale, want)
		}
		if !reflect.DeepEqual(before, a.state) {
			t.Fatalf("language %q changed game state", want)
		}
		saved, err := prefs.Load(a.settingsPath)
		if err != nil || saved.Locale != string(want) {
			t.Fatalf("language %q not persisted: %+v, %v", want, saved, err)
		}
	}
}

// TestFirstRunHasNoLookYet: 設定檔什麼都沒說，就是還沒問過——套用偏好之後落在
// 預設那一套，而且 lookChosen 是 false，開場那張卡才會跳出來。只寫了一部分也算沒問過。
func TestFirstRunHasNoLookYet(t *testing.T) {
	a := newOptionsTestApp(t)
	applyPreferences(a, prefs.Preferences{})
	if a.lookChosen {
		t.Fatal("沒記過呈現卻當成已經問過")
	}
	if got := game.PresentationOf(a.state); got.ID != theme.DefaultPresentation().ID {
		t.Fatalf("第一次啟動落在 %q", got.ID)
	}
	// 只寫了配色、沒寫圖塊與版面：說不清楚是哪一套，所以還是要問。
	b := newOptionsTestApp(t)
	applyPreferences(b, prefs.Preferences{Theme: string(theme.AtariSTOriginal)})
	if b.lookChosen {
		t.Fatal("只寫了一個欄位就被當成選過呈現")
	}
}

// TestRememberedLookWins: 記過的預設直接套用，而且不再問。
func TestRememberedLookWins(t *testing.T) {
	a := newOptionsTestApp(t)
	applyPreferences(a, prefs.Preferences{Presentation: string(theme.AppleIIPresentation)})
	if !a.lookChosen {
		t.Fatal("記過呈現還是要再問一次")
	}
	if got := game.PresentationOf(a.state); got.ID != theme.AppleIIPresentation {
		t.Fatalf("記的是Apple II，套出來是 %q", got.ID)
	}
	if a.state.Layout != theme.RemakeLayout || a.state.Tileset != theme.AppleIITiles {
		t.Fatalf("三個欄位沒有跟著預設走：%s/%s/%s", a.state.Theme, a.state.Tileset, a.state.Layout)
	}
}

// TestOldSettingsSnapToAPreset: 預設集之前寫的設定檔沒有 presentation 欄位，
// 三個舊欄位要被吸附到最近的預設——尤其是會畫壞的那一組（ST 版面配現代圖塊），
// 它必須落回 Atari ST 原版風，而不是照著畫。
func TestOldSettingsSnapToAPreset(t *testing.T) {
	a := newOptionsTestApp(t)
	applyPreferences(a, prefs.Preferences{
		Theme:   string(theme.AtariSTOriginal),
		Tileset: string(theme.ModernTiles),
		Layout:  string(theme.AtariSTLayout),
	})
	if got := game.PresentationOf(a.state); got.ID != theme.AtariSTPresentation {
		t.Fatalf("會畫壞的那一組被吸到 %q", got.ID)
	}
	if a.state.Tileset == theme.ModernTiles {
		t.Fatal("Atari ST 版面還留著 Remake Modern 圖塊")
	}
	if !a.lookChosen {
		t.Fatal("設定檔已經寫齊三個欄位，不該再問一次")
	}
}

// TestOptionsCardCyclesAndRemembers: 卡片的左右鍵兩個方向都走得動，
// 而且每一次變更都立刻落進設定檔。
func TestOptionsCardCyclesAndRemembers(t *testing.T) {
	a := newOptionsTestApp(t)
	a.lookChosen = true
	a.openOptions()
	start := game.PresentationOf(a.state).ID
	a.cyclePresentation(1)
	if game.PresentationOf(a.state).ID == start {
		t.Fatal("往前循環沒有換呈現")
	}
	a.cyclePresentation(-1)
	if got := game.PresentationOf(a.state).ID; got != start {
		t.Fatalf("往前再往後停在 %q，不是 %q", got, start)
	}
	a.cyclePresentation(-1)
	presets := theme.Presentations()
	if got := game.PresentationOf(a.state).ID; got != presets[len(presets)-1].ID {
		t.Fatalf("從第一個往回應該繞到最後一個，得到 %q", got)
	}
	saved, err := prefs.Load(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Presentation != string(game.PresentationOf(a.state).ID) {
		t.Fatalf("設定檔記的是 %q，程式在 %q", saved.Presentation, game.PresentationOf(a.state).ID)
	}
	if saved.Theme != string(a.state.Theme) || saved.Tileset != string(a.state.Tileset) || saved.Layout != string(a.state.Layout) {
		t.Fatalf("設定檔的三個欄位與程式不一致：%s/%s/%s", saved.Theme, saved.Tileset, saved.Layout)
	}
	if !a.showOptions {
		t.Fatal("循環把卡片關掉了")
	}
	a.originalDisk = &originalmedia.Disk{}
	a.closeOptions()
	if a.showOptions {
		t.Fatal("關閉之後卡片還在")
	}
}

// TestLookBootstrapChoosesAndRemembers: 開場那張卡選完就記住，而且不會再跳。
func TestLookBootstrapChoosesAndRemembers(t *testing.T) {
	a := newOptionsTestApp(t)
	a.openLookBootstrap()
	if !a.showOptions || !a.options.lookOnly {
		t.Fatal("開場的呈現卡沒有開起來")
	}
	a.chooseLook(theme.Presentations()[1])
	if a.showOptions {
		t.Fatal("選完之後卡片還在")
	}
	if !a.lookChosen {
		t.Fatal("選完之後還是當作沒問過")
	}
	saved, err := prefs.Load(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Presentation != string(theme.Presentations()[1].ID) {
		t.Fatalf("設定檔記成 %q", saved.Presentation)
	}
}

// TestOptionsRefusesAWrongDisk: a path that is not a SunDog disk is reported
// and changes nothing; an empty path puts the disk away.
func TestOptionsRefusesAWrongDisk(t *testing.T) {
	a := newOptionsTestApp(t)
	a.openOptions()
	if a.applyDisk(filepath.Join(t.TempDir(), "missing.st")) {
		t.Fatal("a missing file was accepted as a disk")
	}
	if a.options.statusKey != "ui.options.disk_failed" || a.originalDisk != nil {
		t.Fatalf("status %q, disk %v", a.options.statusKey, a.originalDisk)
	}
	a.originalDisk = &originalmedia.Disk{}
	a.atariDiskPath = "/somewhere/sundog.st"
	if !a.applyDisk("") {
		t.Fatal("an empty path was refused")
	}
	if a.originalDisk != nil || a.atariDiskPath != "" {
		t.Fatal("an empty path did not put the disk away")
	}
	saved, _ := prefs.Load(a.settingsPath)
	if saved.AtariDisk != "" {
		t.Fatalf("settings still name a disk: %q", saved.AtariDisk)
	}
}

func TestDiskBootstrapFailsClosed(t *testing.T) {
	a := newOptionsTestApp(t)
	a.openDiskBootstrap()
	if !a.showOptions || !a.options.diskOnly || a.options.row != optionsRowDisk {
		t.Fatalf("bootstrap state = shown %v, disk-only %v, row %d", a.showOptions, a.options.diskOnly, a.options.row)
	}
	a.closeOptions()
	if !a.showOptions || !a.options.diskOnly {
		t.Fatal("bootstrap closed without a disk")
	}
	if a.applyDisk("") {
		t.Fatal("bootstrap accepted an empty disk path")
	}
	if a.options.statusKey != "ui.options.disk_none" {
		t.Fatalf("empty bootstrap status = %q", a.options.statusKey)
	}
}

func TestDiskBootstrapMouseRowsMatchItsLayout(t *testing.T) {
	a := newOptionsTestApp(t)
	a.openDiskBootstrap()
	for _, probe := range []struct {
		y    int
		want int
	}{{82, optionsRowDisk}, {104, optionsRowDetect}} {
		got, ok := a.optionsRowAt(20*displayScale, probe.y*displayScale)
		if !ok || got != probe.want {
			t.Fatalf("y=%d gives row %d/%v, want %d", probe.y, got, ok, probe.want)
		}
	}
	if _, ok := a.optionsRowAt(20*displayScale, optionsRowY(optionsRowDisk)*displayScale); ok {
		t.Fatal("bootstrap accepted the hidden normal-card disk row")
	}
}

// TestTitleOpensOptions: the fourth title row, its footer quarter and the O
// key all open the card, and the card is presentation — the state's screen
// stays the title.
func TestTitleOpensOptions(t *testing.T) {
	a := newOptionsTestApp(t)
	if action := a.mouseActionAt(100, 120+4); action != game.ActionOpenOptions {
		t.Fatalf("fourth title row gives %q", action)
	}
	if action := footerMouseAction(game.ScreenTitle, logicalWidth-10); action != game.ActionOpenOptions {
		t.Fatalf("footer's last quarter gives %q", action)
	}
	a.step(game.ActionOpenOptions)
	if !a.showOptions || a.state.Screen != game.ScreenTitle {
		t.Fatalf("card %v, screen %s", a.showOptions, a.state.Screen)
	}
}

// TestRetiredLookNamesMigrate: 設定檔還寫著已退役的 Modern Apple 配色與圖塊時，
// 會先換成接手的名字，再吸附到預設集，而不是被忽略。
func TestRetiredLookNamesMigrate(t *testing.T) {
	a := newOptionsTestApp(t)
	applyPreferences(a, prefs.Preferences{Theme: "modern-apple-dark", Tileset: "modern-apple-port-tiles"})
	if got := game.PresentationOf(a.state); got.ID != theme.AtariSTPresentation {
		t.Fatalf("退役名字吸到 %q", got.ID)
	}
	for _, tc := range []struct {
		layout theme.LayoutID
		want   theme.PresentationID
	}{
		{theme.AtariSTLayout, theme.AtariSTPresentation},
		{theme.RemakeLayout, theme.AtariSTPresentation},
	} {
		applyPreferences(a, prefs.Preferences{Theme: "modern-apple", Tileset: "modern-apple-tiles", Layout: string(tc.layout)})
		if got := game.PresentationOf(a.state); got.ID != tc.want {
			t.Fatalf("退役設定版面 %s 給了 %s", tc.layout, got.ID)
		}
	}
}

func TestRetiredHDPresetReturnsToAtariWithoutChangingGame(t *testing.T) {
	a := newOptionsTestApp(t)
	a.state.Cash, a.state.Fuel, a.state.CombatRolls = 1234, 72, 987
	before := a.state
	applyPreferences(a, prefs.Preferences{Presentation: string(theme.ModernPresentation)})
	if !a.lookChosen || game.PresentationOf(a.state).ID != theme.AtariSTPresentation {
		t.Fatal("退役HD偏好未回到已選定的Atari ST")
	}
	if a.state.Cash != before.Cash || a.state.Fuel != before.Fuel || a.state.CombatRolls != before.CombatRolls || a.state.Schema != before.Schema {
		t.Fatal("退役呈現改變遊戲狀態")
	}
	for _, preset := range theme.Presentations() {
		if preset.ID == theme.ModernPresentation {
			t.Fatal("退役HD仍在正式選單")
		}
	}
}
