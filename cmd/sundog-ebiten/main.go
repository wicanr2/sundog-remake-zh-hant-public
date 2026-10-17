// Command sundog-ebiten is the first interactive presentation shell for the
// framework-neutral remake core. It is intentionally small: the reducer owns
// rules and this command owns input, pixel layout, and a replaceable font.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	ebitentext "github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"

	"github.com/wicanr2/sundog-remake/internal/appdir"
	gameaudio "github.com/wicanr2/sundog-remake/internal/audio"
	"github.com/wicanr2/sundog-remake/internal/catalog"
	"github.com/wicanr2/sundog-remake/internal/game"
	semanticinput "github.com/wicanr2/sundog-remake/internal/input"
	"github.com/wicanr2/sundog-remake/internal/localization"
	"github.com/wicanr2/sundog-remake/internal/prefs"
	"github.com/wicanr2/sundog-remake/internal/render"
	apple2source "github.com/wicanr2/sundog-remake/internal/source/apple2"
	"github.com/wicanr2/sundog-remake/internal/source/atarist"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/storage"
	"github.com/wicanr2/sundog-remake/internal/theme"
	"github.com/wicanr2/sundog-remake/internal/world"
)

// buildVersion is injected by tools/build_release.sh. Development builds keep
// an explicit value so a diagnostic never masquerades as a release artifact.
var buildVersion = "development"

const (
	logicalWidth          = 320
	logicalHeight         = 200
	defaultDisplayScale   = 4
	tileScale             = 2
	footerHelpY           = 178
	footerHelpWidth       = 310
	footerHelpMaxLines    = 2
	equipmentRowStep      = 9
	equipmentNoteWidth    = panelTextW
	equipmentNoteMaxLines = 2

	// statusStripY is where the shared status-message strip starts. It is drawn
	// over whatever the panel put there, and its own two lines of text sit at
	// 138 and 147, so anything a panel draws below panelLastRowY collides with
	// the current gameplay message rather than with empty space.
	statusStripY = frameMessageY + 4
	// 訊息框的七列與折行的列距。八像素的墨加一像素間隔，七列剛好填滿 67 高的框。
	messageRowStep = 9
	wrappedRowStep = 9
	panelLastRowY  = frameLeftY + frameLeftH - 9
	equipmentNoteY = panelLastRowY - 10

	// The generic panel list: one row per line, from just under the title bar
	// down to the last row that clears the strip.
	panelRowY0   = 45
	panelRowStep = 9

	// The ground panel is six actions and nothing else. The location and the
	// facilities are lists of names, which the 68-pixel column can only show as
	// an ellipsis, so both are in the wide message box.
	groundRowY0   = panelRowY0
	groundRowStep = panelRowStep

	// The F1 card covers the frame, so its rows are bounded by the card itself
	// rather than by the status strip.
	helpRowY0   = 43
	helpRowStep = 8

	// The menu screens carry their own message row inside the panel instead of
	// the shared status strip. Same arithmetic: a row is eight pixels of ink
	// starting one below its y, so a row above the message has to end before it.
	menuMessageY = 137

	// The engineering panel is the other crowded one: four system meters, the
	// selected system with its own meter, the shunt row and the source note.
	// Seven rows have to clear the strip, so the meters sit closer together
	// than the 16 pixels they used when the note was still running under it.
	engineeringRowY0     = 45
	engineeringRowStep   = 11
	engineeringSelectedY = 89
	engineeringShuntY    = 100
	engineeringRowsY     = 109
	// 標籤與量表要一起塞進 82 寬的左方框：標籤五個中文字 40 像素，量表用剩下的。
	engineeringLabelWidth = 40
	engineeringMeterX     = panelTextX + engineeringLabelWidth + 2
	engineeringMeterWidth = panelX + panelW - 4 - engineeringMeterX

	// 高解析地表圖也畫進原版介面框的中央視埠，所以原點就是視埠內緣，
	// 一格用 tilePixels 而不是圖集自己的 20——20 的話 10×7 格會撐破 165×116 的框。
	modernGroundOriginX = sceneX
	modernGroundOriginY = sceneY
	// 原版介面框的七塊。座標量在 docs/ui/original-layout.json，來源是原版磁碟影像目錄
	// 的 raw 0x14670。重製版照這七塊排版，但文字仍畫在視窗解析度的圖層上——排版以 8px
	// 字面量測，所以一個中文字佔 8 個邏輯像素，左方框 74 的可寫寬度放得下九個字。
	frameViewportX = 94
	frameViewportY = 4
	frameViewportW = 165
	frameViewportH = 116
	frameLeftX     = 4
	frameLeftY     = 20
	frameLeftW     = 82
	frameLeftH     = 106
	frameGaugeX    = 267
	frameGaugeY    = 20
	frameGaugeW    = 48
	frameGaugeH    = 106
	frameMessageX  = 4
	frameMessageY  = 131
	frameMessageW  = 269
	frameMessageH  = 67
	frameMenuX     = 273
	frameMenuY     = 131
	frameMenuW     = 46
	frameMenuH     = 67
	frameTopLeftX  = 16
	frameTopLeftY  = 5
	frameTopLeftW  = 58
	frameTopLeftH  = 13
	// 浮動視窗：蓋在中央視埠上的不透明小視窗，給放不進 82 寬左方框的清單用
	//（艙位十二格、設備十六項、各畫面的讀數）。原版的選單也是這樣蓋在畫面上的：
	// 它只有那份清單那麼大，底下的場景照樣看得見。
	//
	// 下面四個數字是它能長到的最大範圍——視埠的內緣。實際的位置與大小由內容決定，
	// 見 fitFloatWindow。
	floatX = frameViewportX + 2
	floatY = frameViewportY + 2
	floatW = frameViewportW - 4
	floatH = frameViewportH - 4
	// floatPadding 是框線與第一個字之間的留白。
	floatPadding = 5
	// 再小就只剩一個框，看不出是視窗。
	floatMinW = 96
	floatMinH = 32
	// floatTextW 是一列最寬能有多少——折行用它，視窗再依折完的實際寬度縮。
	// 反過來（先縮視窗再折行）會變成兩者互相決定。
	floatTextW = floatW - 2*floatPadding
	// floatColumnGap 是兩欄之間的空隙。
	floatColumnGap = 8
	// floatDescent 是最後一列底下要多留的墨。
	floatDescent = 4
	// 設備清單排兩欄。
	equipmentColumns = 2

	floatRowStep     = 12
	carriedIconStep  = ItemIconStep
	shipboardRowStep = 14
	// 指南針與圖示列都排在視窗底下那一條；指南針佔右下角，圖示列排到它左邊為止。
	floatBottomRowInset = 20
	floatCompassInset   = 30

	frameTopRightX = 273
	frameTopRightY = 5
	frameTopRightW = 42
	frameTopRightH = 13

	// The shared panel's rectangle. Every screen's panel and every click band
	// comes from these four numbers.
	//
	// It sits beside the scene, not on it. The panel used to be drawn at
	// (5,27)-(138,153) with an 82%-opaque black fill, and the city map was
	// drawn at (6,28)-(134,124) -- the same rectangle. The map showed through
	// the panel and the panel's text sat on the map, so neither was readable.
	// TestThePanelNeverCoversTheScene keeps them apart.
	panelX = frameLeftX
	panelY = frameLeftY
	panelW = frameLeftW
	panelH = frameLeftH
	// panelTextX and panelTextW are the inset the panel's rows are drawn at.
	panelTextX = panelX + 7
	panelTextW = panelW - 14

	// The scene's top-left corner. The city map's tiles are laid out from here,
	// and the panel has to start to the right of where the last column ends.
	sceneX = frameViewportX + 2
	sceneY = frameViewportY + 2
	// tilePixels is one map cell in the 320x200 buffer.
	tilePixels = 16
)

// displayScale is how many screen pixels one of the original's pixels takes.
// The whole frame is drawn into a 320x200 buffer and blown up by this with no
// filtering, so any whole number keeps the pixels square and hard-edged.
//
// These three only fix the size the window opens at; -scale 6 opens 1920x1200
// because a screenshot for a page that will be scaled again wants a larger
// multiple to be resampled from. Once the window is up the player can drag it
// to any size and the running scale comes from viewportFor instead.
var (
	displayScale  = defaultDisplayScale
	displayWidth  = logicalWidth * displayScale
	displayHeight = logicalHeight * displayScale
)

// setDisplayScale fixes the size the window opens at.
func setDisplayScale(scale int) {
	if scale < 1 {
		scale = defaultDisplayScale
	}
	displayScale = scale
	displayWidth = logicalWidth * scale
	displayHeight = logicalHeight * scale
}

// viewportFor fits the 320x200 frame into a window of any size: the largest
// whole-number multiple that still fits, centred, with the leftover as border.
//
// Whole numbers are the point. A fractional multiple resamples every tile edge
// into a grey smear, which on pixel art is the one artefact that shows from
// across the room. Losing a few pixels to a border is the cheaper trade.
func viewportFor(width, height int) (scale, originX, originY int) {
	scale = width / logicalWidth
	if vertical := height / logicalHeight; vertical < scale {
		scale = vertical
	}
	if scale < 1 {
		scale = 1
	}
	originX = (width - logicalWidth*scale) / 2
	originY = (height - logicalHeight*scale) / 2
	if originX < 0 {
		originX = 0
	}
	if originY < 0 {
		originY = 0
	}
	return scale, originX, originY
}

type app struct {
	reducer        game.Reducer
	state          game.State
	labels         catalog.Catalog
	locale         localization.Locale
	languages      map[localization.Locale]languagePack
	face           font.Face
	savePath       string
	libraryPath    string
	librarySlot    string
	frameLimit     int
	frames         int
	sleepWakeArmed bool
	spaceKey       string
	spaceImage     *ebiten.Image
	uiLayer        *ebiten.Image
	// textLayer carries every string at the window's own resolution. The rest
	// of the frame is composed in a 320x200 buffer and blown up, which is what
	// keeps the tiles hard-edged; putting Chinese through that same path would
	// render each glyph into about eight pixels and then magnify the mush.
	textLayer *ebiten.Image
	// The text face is rebuilt whenever the window changes size, so a glyph is
	// always drawn with as many pixels as the window actually has. newFace is
	// how one is made; faces keeps the ones already made.
	newFace func(size int) font.Face
	faces   map[int]font.Face
	hdFaces map[int]font.Face
	// scale, originX and originY are the current fit of the 320x200 frame into
	// the window. Layout refreshes them every frame; everything that converts
	// between layout coordinates and window pixels reads them.
	scale   int
	originX int
	originY int
	viewW   int
	viewH   int
	// showHelp puts the key card over the frame. It is presentation only: it
	// never reaches the reducer or the save file.
	showHelp bool
	// showOptions puts the options card over the frame; options is what it
	// is showing. settingsPath is where the choices are written, and
	// atariDiskPath is the disk the choices name (see options.go).
	showOptions bool
	// newGameArmed is set by the first New Game press when there is an unstored
	// game to lose; the second press goes through. Not saved -- it belongs to
	// this sitting.
	newGameArmed bool
	// lookChosen records that the player has answered the first-run look
	// question. The settings file is where it persists; false means the card
	// is still owed, which is what a settings file without a preset means.
	lookChosen bool
	// stMessageFrame is the frame the last message arrived on; the zoom
	// window shows a fresh message on its own for a moment (stlayout.go).
	stMessageFrame int
	// stSceneMessageFrame is the same clock for the full-screen city and
	// shipboard message windows. It is a separate field because those two also
	// have to show the message that comes with a screen change -- landing, or
	// boarding -- while the zoom window deliberately does not.
	stSceneMessageFrame int
	// shipWalker is the little figure walking the corridors (stwalker.go).
	shipWalker stShipWalker
	// groundWalker is the same figure on the city ground, at the original's
	// resolution (stground.go): a pixel walk over a drawn window whose palette
	// indices decide what it can stand on.
	groundWalker           stGroundWalker
	stGroundScene          *stGroundScene
	stGroundPatternIndices []byte
	collidePatternData     [originalmedia.CollidePatternCount][]originalmedia.CollidePoint
	collideRead            bool
	collideOK              bool
	// cabinFloor is the corridor picture's passability, one bool per pixel, and
	// cabinLegal caches which anchor positions the walker's collide pattern
	// allows. Both are built once the picture is decoded.
	cabinFloor          []bool
	cabinWidth          int
	cabinHeight         int
	cabinLegal          []bool
	cabinComponent      []int32
	cabinPattern        []originalmedia.CollidePoint
	cabinPatternRead    bool
	cabinField          []int32
	cabinFieldTargetX   int
	cabinFieldTargetY   int
	cabinFieldHasTarget bool
	// starScrollX/Y is how far the tactical starfield has slid under the
	// player's turning (replot_screen's G11/G10).
	starScrollX, starScrollY int
	roomArrivals             stRoomArrivals
	options                  optionsCard
	// showCredits puts the original's credits picture over the frame.
	showCredits bool
	// originalTitleArt is the title decoded from the disk (originaltitle.go).
	originalTitleArt  *originalTitleArt
	originalTitleRead bool
	// stFrameImage is the frame picture decoded from the disk (stlayout.go).
	stFrameImage        *ebiten.Image
	stFrameRead         bool
	stFontStrip         *originalmedia.Font
	stFontRead          bool
	stBackdrops         map[int]*ebiten.Image
	stGroundPattern     *ebiten.Image
	stGroundPatternRead bool
	settingsPath        string
	atariDiskPath       string
	modernAtlas         *ebiten.Image
	modernAtlasHash     string
	// originalDisk is the player's own Atari ST disk, when one was given. The
	// ground then draws the original's tiles instead of the remake's art. The
	// disk is never shipped, so this is nil in every default build.
	originalDisk  *originalmedia.Disk
	originalTiles map[int]*originalTileCache
	// itemIcons are the original's own 16x16 item pictures, decoded once from
	// the same disk. itemIconsRead records that the attempt was made, so a disk
	// without them is not re-decoded every frame.
	itemIcons     map[int]*ebiten.Image
	itemIconsRead bool
	// shipInterior holds the original corridor picture in its two light
	// states (palette 5 black / red); read lazily like the item icons.
	shipInterior     [2]*ebiten.Image
	shipInteriorRead bool
	// groundSprites are the original walker patterns, read lazily.
	groundSprites     *[originalmedia.GroundSpriteCount][originalmedia.GroundSpriteRows]byte
	groundSpritesRead bool
	// interiorBackdrops caches the original's 160x100 room pictures by picture
	// index, decoded once from the same disk.
	interiorBackdrops map[int]*ebiten.Image
	// sounds are the original's sound effects, rendered from the player's own
	// disk by sundog-sound-export. Nothing is shipped, so this is silent in
	// every default build.
	sounds *soundboard
	music  *musicSystem
}

type languagePack struct {
	labels catalog.Catalog
	apple2 apple2source.TextCatalog
	dialog atarist.DialogData
}

// originalTileCache holds one city's decoded tiles. Decoding is not free and a
// city is walked for many frames, so each tile is decoded once.
type originalTileCache struct {
	set    *originalmedia.TileSet
	images map[int]*ebiten.Image
}

var prototypeLibrarySlots = []string{"slot-1", "slot-2", "slot-3", "slot-4", "slot-5", "slot-6", "slot-7", "slot-8"}

func main() {
	fontPath := flag.String("font", "assets/fonts/NotoSansCJKtc-Regular.otf", "繁體中文字型資產")
	sourcePath := flag.String("catalog", "docs/i18n/apple2-text-catalog.json", "Apple II 來源文字 catalog")
	uiPath := flag.String("ui-catalog", "docs/i18n/sundog-ui-zh-Hant.json", "繁中 UI catalog")
	saveName := fmt.Sprintf("sundog-ebiten-v%d-slot-1.json", game.CurrentSchema)
	libraryName := fmt.Sprintf("sundog-ebiten-v%d-library.json", game.CurrentSchema)
	savePath := flag.String("save", "workplace/save/"+saveName, "可寫入的重製版存檔槽")
	libraryPath := flag.String("library", "workplace/save/"+libraryName, "可寫入的重製版保存庫替代檔")
	loadSave := flag.Bool("load", false, "啟動前載入可寫入的重製版存檔槽")
	startScreen := flag.String("start", "title", "起始畫面：title、shipboard、status、ground、trade、uniteller、tube-station、tube-broke、navigation、tactical、arena、engineering、equipment、cargo、dialog、mission 或 outcome")
	frames := flag.Int("frames", 0, "在指定影格數後退出；供 Docker 截圖 smoke 使用")
	atariAtlasManifest := flag.String("atari-atlas-manifest", "", "選用的 Atari ST normalized PNG atlas manifest")
	atariDisk := flag.String("atari-disk", "", "選用的原版 Atari ST raw .st；給了就用原版的地表圖塊。沒給就讀設定檔，再到慣用位置偵測")
	settingsPath := flag.String("settings", "", "設定檔（介面配色、圖塊集、磁碟路徑）；預設在使用者設定目錄")
	localeFlag := flag.String("locale", "", "啟動語系：zh-Hant、zh-Hans、ja、ko、en；指定後也會保存到設定檔")
	audioDir := flag.String("audio", "", "音效目錄；預設是 sundog-sound-export 的輸出位置")
	scale := flag.Int("scale", defaultDisplayScale, "每個原版像素放大成幾個螢幕像素；截圖要再被縮放時給大一點")
	showVersion := flag.Bool("version", false, "顯示完整發行版號後結束")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildVersion)
		return
	}
	setDisplayScale(*scale)

	// A packaged build starts from whatever directory the launcher happened to
	// use, so the read-only assets are looked for next to the executable as well,
	// and the writable files move under the user's own directory when the working
	// directory cannot take them.
	*fontPath = appdir.Asset(*fontPath)
	*sourcePath = appdir.Asset(*sourcePath)
	*uiPath = appdir.Asset(*uiPath)
	// 只有玩家沒指定路徑時才改寫。明確給了 -save／-library 的呼叫端
	// （replay 腳本、測試、錄影流程）拿到的永遠是它自己給的那個路徑。
	given := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if !given["save"] {
		*savePath = appdir.UserData(saveName, *savePath)
	}
	if !given["library"] {
		*libraryPath = appdir.UserData(libraryName, *libraryPath)
	}
	if *settingsPath == "" {
		*settingsPath = prefs.DefaultPath()
	}
	settings, err := prefs.Load(*settingsPath)
	if err != nil {
		// A broken settings file is the player's to see in the log, not a
		// reason to refuse to start: this run just has no remembered wishes.
		log.Printf("設定檔讀不進來，這次不套用：%v", err)
		settings = prefs.Preferences{}
	}
	if *localeFlag != "" {
		if !localization.KnownLocale(localization.Locale(*localeFlag)) {
			log.Fatalf("不支援的語系 %q；可用 zh-Hant、zh-Hans、ja、en", *localeFlag)
		}
		settings.Locale = *localeFlag
	}

	reducer, err := game.NewReducer()
	if err != nil {
		log.Fatal(err)
	}
	if *atariAtlasManifest != "" {
		registry, err := reducer.Themes.WithAtariSTAtlas(*atariAtlasManifest)
		if err != nil {
			log.Fatal(fmt.Errorf("load Atari ST atlas: %w", err))
		}
		reducer.Themes = registry
	}
	languages, err := loadLanguagePacks(*sourcePath, *uiPath)
	if err != nil {
		log.Fatal(err)
	}
	fontBytes, err := os.ReadFile(*fontPath)
	if err != nil {
		log.Fatal(fmt.Errorf("read font %s: %w", *fontPath, err))
	}
	fontData, err := opentype.Parse(fontBytes)
	if err != nil {
		log.Fatal(fmt.Errorf("parse font %s: %w", *fontPath, err))
	}
	face, err := opentype.NewFace(fontData, &opentype.FaceOptions{Size: 8, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		log.Fatal(fmt.Errorf("create font face: %w", err))
	}

	// The same font at whatever size the window currently calls for. Layout is
	// still measured on the 8-pixel face, so this only changes how many pixels
	// a glyph is drawn with, never what fits on a line.
	newFace := func(size int) font.Face {
		display, err := opentype.NewFace(fontData, &opentype.FaceOptions{
			Size: float64(size), DPI: 72, Hinting: font.HintingFull,
		})
		if err != nil {
			log.Printf("build %dpx text face: %v", size, err)
			return face
		}
		return display
	}

	ebiten.SetWindowSize(displayWidth, displayHeight)
	ebiten.SetWindowTitle("SunDog Remake")
	ebiten.SetVsyncEnabled(false)
	// The player can drag the window to any size; the frame follows by picking
	// the largest whole-number multiple that fits. Below one multiple there is
	// nothing left to show, so that is the floor.
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowSizeLimits(logicalWidth, logicalHeight, -1, -1)
	// The disk is opened before the state exists because the default look
	// depends on it: a game that starts with an Atari ST disk in hand starts
	// in the Atari ST look (applyPreferences).
	disk := openAtariDisk(*atariDisk, settings.AtariDisk)
	state := game.NewState()
	if *loadSave {
		state, err = storage.Load(*savePath)
		if err != nil {
			log.Fatal(err)
		}
		state = resumeState(state)
	}
	state, err = startState(state, reducer, *startScreen)
	if err != nil {
		log.Fatal(err)
	}
	gameApp := &app{
		reducer:     reducer,
		state:       state,
		languages:   languages,
		face:        face,
		newFace:     newFace,
		savePath:    *savePath,
		libraryPath: *libraryPath,
		librarySlot: prototypeLibrarySlots[0],
		frameLimit:  *frames,
	}
	gameApp.applyLocale(localization.NormalizeLocale(settings.Locale))
	soundDir := *audioDir
	if soundDir == "" {
		soundDir = gameaudio.DefaultDir()
	}
	gameApp.sounds = newSoundboard(soundDir)
	gameApp.music = newMusicSystem(gameApp.sounds, appdir.Asset("assets/music"))
	defer gameApp.music.close()
	if gameApp.sounds.found() {
		log.Printf("音效讀到 %d 個檔案：%s", gameApp.sounds.count(), soundDir)
	} else {
		log.Printf("沒有音效可播；用 sundog-sound-export 從你自己的原版磁碟轉出到 %s", soundDir)
	}
	gameApp.settingsPath = *settingsPath
	gameApp.useDisk(disk)
	if disk == nil && !given["atari-disk"] && settings.AtariDisk == "" {
		// First run with nothing named: look in the usual places once, so a
		// player who dropped their disk next to the game sees it without
		// ever opening the options.
		gameApp.detectDisk()
	}
	if gameApp.originalDisk == nil && gameApp.state.Screen == game.ScreenTitle {
		gameApp.openDiskBootstrap()
	}
	applyPreferences(gameApp, settings)
	gameApp.savePreferences()
	if err := ebiten.RunGame(gameApp); err != nil && err != ebiten.Termination {
		log.Fatal(err)
	}
}

// openAtariDisk tries the command line first, then the settings file. A bad
// disk is reported and leaves the title bootstrap in charge; it never falls
// through to replacement title art.
func openAtariDisk(flagPath, settingsPath string) *originalmedia.Disk {
	for _, path := range []string{flagPath, settingsPath} {
		if path == "" {
			continue
		}
		disk, err := originalmedia.Open(path)
		if err != nil {
			log.Printf("原版磁碟讀不進來：%v", err)
			continue
		}
		return disk
	}
	return nil
}

// applyPreferences is the rule for what look the game opens in. A wish
// written in the settings wins over everything, including the theme a loaded
// save carries — the settings are the player's current choice, the save is
// where they were when they wrote it. With no wish expressed, a new game
// opens in the Atari ST look when an Atari ST disk is in hand, and a loaded
// save keeps its own look.
func applyPreferences(a *app, settings prefs.Preferences) {
	if a.music != nil && settings.MusicEnabled != nil {
		a.music.enabled = *settings.MusicEnabled
	}
	a.applyLocale(localization.NormalizeLocale(settings.Locale))
	// The Apple II HD direction retired on 2026-10-14. A remembered choice
	// returns to the Atari ST default without asking the player again.
	if theme.PresentationID(settings.Presentation) == theme.ModernPresentation {
		a.state = game.WithPresentation(a.state, theme.DefaultPresentation())
		a.lookChosen = true
		return
	}
	// The preset the file names is what the player chose. Without one this is a
	// first run — or a file written before the presets existed — so the game
	// asks rather than guesses; the old three fields still decide what the card
	// starts on, which pre-answers the question with the look they had.
	if preset, ok := theme.PresentationByID(theme.PresentationID(settings.Presentation)); ok {
		a.state = game.WithPresentation(a.state, preset)
		a.lookChosen = true
		return
	}
	if settings.Theme == "" && settings.Tileset == "" && settings.Layout == "" {
		a.state = game.WithPresentation(a.state, theme.DefaultPresentation())
		return
	}
	// A retired name (the Modern Apple chromes, the two Modern Apple tilesets)
	// stands for what replaced it before the triple is snapped to a preset.
	a.state = game.WithPresentation(a.state, theme.SnapPresentation(
		theme.MigrateChrome(theme.ID(settings.Theme)),
		theme.MigrateTileset(theme.TilesetID(settings.Tileset)),
		theme.LayoutID(settings.Layout)))
	// A file that names the whole triple is a settled look, even without the
	// preset field: a player who already had one should not be asked again,
	// and neither should the capture scripts, which write all three.
	if settings.Theme != "" && settings.Tileset != "" && settings.Layout != "" {
		a.lookChosen = true
	}
}

func (a *app) applyLocale(locale localization.Locale) bool {
	pack, ok := a.languages[locale]
	if !ok {
		return false
	}
	a.locale = locale
	a.labels = pack.labels
	a.reducer = a.reducer.WithAppleIIText(pack.apple2).WithDialogText(pack.dialog)
	a.reducer = a.reducer.WithLocalizer(func(key, fallback string) string {
		return localization.Lookup(a.labels, key, fallback)
	})
	return true
}

func startState(state game.State, reducer game.Reducer, startScreen string) (game.State, error) {
	// 正式預設入口必須保留新局資源；以下補給只供明確選用的診斷場景。
	if startScreen == "title" {
		return state, nil
	}
	sequences := map[string][]game.Action{
		"title":              nil,
		"character-library":  {game.ActionSelectGame},
		"character-creation": {game.ActionSelectGame, game.ActionNewGame},
		"shipboard":          {game.ActionPlay},
		"status":             {game.ActionPlay, game.ActionOpenStatus},
		// Direct-entry screenshots name a steady screen, not the transition used
		// to reach it. Normal ActionOpenGround still runs the complete landing;
		// only the diagnostic -start path clears that overlay explicitly.
		"ground":      {game.ActionPlay, game.ActionOpenGround, game.ActionSkipLanding},
		"trade":       {game.ActionPlay, game.ActionOpenGround, game.ActionSkipLanding, game.ActionOpenTrade},
		"uniteller":   {game.ActionPlay, game.ActionOpenGround, game.ActionSkipLanding, game.ActionOpenUniteller},
		"navigation":  {game.ActionPlay, game.ActionOpenNavigation},
		"tactical":    {game.ActionPlay, game.ActionOpenTactical},
		"engineering": {game.ActionPlay, game.ActionOpenEngineering},
		"equipment":   {game.ActionPlay, game.ActionOpenEngineering, game.ActionOpenEquipment},
		"cargo":       {game.ActionPlay, game.ActionOpenCargo},
		"dialog":      {game.ActionPlay, game.ActionOpenGround, game.ActionSkipLanding, game.ActionMoveEast, game.ActionTalk},
		"mission": {
			game.ActionPlay,
			game.ActionOpenGround,
			game.ActionSkipLanding,
			game.ActionLoadSupplies,
			game.ActionLoadCryogens,
			// 回船要走進船那一格；`-start` 只是診斷用的重播序列，不是玩家路徑。
			game.ActionMoveEast,
			game.ActionMoveWest,
			game.ActionOpenNavigation,
			game.ActionNavigate,
			game.ActionOpenGround,
			game.ActionSkipLanding,
		},
		"outcome": {
			game.ActionPlay,
			game.ActionOpenGround,
			game.ActionSkipLanding,
			game.ActionLoadSupplies,
			game.ActionLoadCryogens,
			// 回船要走進船那一格；`-start` 只是診斷用的重播序列，不是玩家路徑。
			game.ActionMoveEast,
			game.ActionMoveWest,
			game.ActionOpenNavigation,
			game.ActionNavigate,
			game.ActionOpenGround,
			game.ActionSkipLanding,
			game.ActionDeliverMission,
		},
	}
	// The tube station is a building of its own, so there is no action that
	// opens it from the landing cell. Its replay walks the ground sequence and
	// then puts the walker on the station's door — the same kind of shortcut as
	// the fuel below, and for the same reason. "tube-broke" is the same station
	// with the balance emptied, which is the only way to see the refusal row.
	// The arena is only reachable through a street encounter, and that is a
	// 1/1000 roll per pass. The diagnostic walks the real path — a mugger's
	// conversation, then walking away from it — and only skips the die.
	openTheArena := startScreen == "arena"
	if openTheArena {
		startScreen = "ground"
	}
	moveToTubeStation := startScreen == "tube-station" || startScreen == "tube-broke"
	emptyTheBank := startScreen == "tube-broke"
	if moveToTubeStation {
		startScreen = "ground"
	}
	sequence, ok := sequences[startScreen]
	if !ok {
		return game.State{}, fmt.Errorf("unsupported start screen %q", startScreen)
	}
	// A new game's tank is empty, the way the original leaves it (worklist D6),
	// so the sequences that fly would stall on ActionNavigate. -start is a
	// diagnostic shortcut, not a player path — it already skips the landing —
	// so it fuels the ship up front rather than walking to a spaceport first.
	// The same goes for the ship: a new game rolls random damage (XSTARTUP:0x31)
	// which now and then blows the Sub-C engines, and then the flying sequences
	// stop at ActionNavigate with "engines blown".
	state.Fuel = game.MaxFuel()
	state.ShipSystems = game.IntactShipSystems()
	if startScreen == "mission" || startScreen == "outcome" {
		// 診斷場景補足掃描器，不改正式新局或導航門檻。
		data, err := atarist.EmbeddedShipSystemData()
		if err != nil {
			return game.State{}, err
		}
		for i, source := range data.Systems() {
			if state.ShipSystems[i].ID == game.ShipSystemPilotage && source.Alternate != nil {
				state.ShipSystems[i].Rows[0][source.Alternate.Column] = source.Alternate.ItemIndex
			}
		}
	}
	// An empty pod draws no pirate (game.PodDrawsEncounter), so -start tactical
	// would open on empty scanners. The diagnostic exists to show a fight, so it
	// loads one lot the same way it fills the tank.
	if startScreen == "tactical" {
		state.Holds[0] = game.Hold{Kind: game.HoldGoods}
	}
	for _, action := range sequence {
		if action == game.ActionDeliverMission {
			// 此入口只擷取任務畫面；正常城外步行不走這條診斷捷徑。
			position := world.LandingPosition(state.CityID)
			state.Position = game.Position{X: position.X, Y: position.Y}
			state.GroundView = game.GroundView{}
		}
		var err error
		state, err = reducer.Step(state, action)
		if err != nil {
			return game.State{}, fmt.Errorf("start action %s: %w", action, err)
		}
	}
	if openTheArena {
		state.Screen = game.ScreenDialog
		state.StreetPending = game.StreetKindMugger
		var err error
		state, err = reducer.Step(state, game.ActionBack)
		if err != nil {
			return game.State{}, fmt.Errorf("start arena: %w", err)
		}
	}
	if moveToTubeStation {
		door, ok := reducer.World.FacilityPosition(state.CityID, world.FacilityTubeStation)
		if !ok {
			return game.State{}, fmt.Errorf("%q has no tube station", state.CityID)
		}
		state.Position = game.Position{X: door.X, Y: door.Y}
		if emptyTheBank {
			state.Bank = 0
			state.BankBalances[state.SystemID] = 0
		}
		var err error
		state, err = reducer.Step(state, game.ActionOpenTubeStation)
		if err != nil {
			return game.State{}, fmt.Errorf("start action %s: %w", game.ActionOpenTubeStation, err)
		}
		if emptyTheBank {
			// 直接落在確認頁，省掉一次點擊——這只是診斷捷徑。
			if list := reducer.TubeDestinations(state); len(list) > 0 {
				state.TubePage, state.TubeTargetID = game.TubePageConfirm, list[0].ID
			}
		}
	}
	return state, nil
}

func loadCatalogs(sourcePath, uiPath string) (catalog.Catalog, apple2source.TextCatalog, error) {
	load := func(path string) (catalog.Catalog, []byte, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return catalog.Catalog{}, nil, fmt.Errorf("read catalog %s: %w", path, err)
		}
		loaded, err := catalog.Load(bytes.NewReader(data))
		if err != nil {
			return catalog.Catalog{}, nil, err
		}
		return loaded, data, nil
	}
	base, sourceData, err := load(sourcePath)
	if err != nil {
		return catalog.Catalog{}, apple2source.TextCatalog{}, err
	}
	sourceText, err := apple2source.LoadTextCatalog(bytes.NewReader(sourceData))
	if err != nil {
		return catalog.Catalog{}, apple2source.TextCatalog{}, fmt.Errorf("validate Apple II source catalog: %w", err)
	}
	ui, _, err := load(uiPath)
	if err != nil {
		return catalog.Catalog{}, apple2source.TextCatalog{}, err
	}
	merged, err := base.Merge(ui)
	if err != nil {
		return catalog.Catalog{}, apple2source.TextCatalog{}, err
	}
	return merged, sourceText, nil
}

func loadLanguagePacks(sourcePath, uiPath string) (map[localization.Locale]languagePack, error) {
	result := make(map[localization.Locale]languagePack, len(localization.Locales()))
	dialogSource, err := atarist.EmbeddedDialogData()
	if err != nil {
		return nil, err
	}
	for _, locale := range localization.Locales() {
		source := sourcePath
		ui := uiPath
		if locale != localization.TraditionalChinese {
			source = strings.Replace(sourcePath, "apple2-text-catalog.json", "apple2-text-"+string(locale)+".json", 1)
			ui = strings.Replace(uiPath, "zh-Hant", string(locale), 1)
		}
		labels, apple2, err := loadCatalogs(source, ui)
		if err != nil {
			return nil, fmt.Errorf("load %s language pack: %w", locale, err)
		}
		dialogPath := appdir.Asset("docs/i18n/atari-st-dialog-" + string(locale) + ".json")
		dialogBytes, err := os.ReadFile(dialogPath)
		if err != nil {
			return nil, fmt.Errorf("read %s dialog pack: %w", locale, err)
		}
		dialog, err := dialogSource.Translate(bytes.NewReader(dialogBytes))
		if err != nil {
			return nil, fmt.Errorf("load %s dialog pack: %w", locale, err)
		}
		result[locale] = languagePack{labels: labels, apple2: apple2, dialog: dialog}
	}
	return result, nil
}

func (a *app) Update() error {
	defer func() { a.music.update(sceneMusic(a.state), time.Now()) }()
	if inpututil.IsKeyJustPressed(ebiten.KeyF6) {
		a.toggleMusic()
		return nil
	}
	a.frames++
	if !a.state.Sleep.Active {
		a.sleepWakeArmed = false
	}
	setPanelLayout(a.state.Layout)
	if a.frameLimit > 0 && a.frames >= a.frameLimit {
		return ebiten.Termination
	}
	// The original title is a required player-owned asset. Never let a title
	// frame without a valid disk fall through to the remake menu underneath.
	if a.state.Screen == game.ScreenTitle && a.originalDisk == nil && !a.showOptions {
		a.openDiskBootstrap()
	}
	// The look question comes after the disk one: the Atari ST preset shows
	// the disk's own pictures, so asking before there is a disk would be
	// asking about something the player cannot see yet. It is a title-screen
	// question — a -start replay walks through the title, so the card is still
	// the first thing that sees the keyboard.
	if !a.lookChosen && a.originalDisk != nil && !a.showOptions &&
		a.state.Screen == game.ScreenTitle {
		a.openLookBootstrap()
	}
	// The help card is a reading mode, so it swallows the rest of the input
	// while it is up: the key pressed to close it must not also act on the game
	// behind it.
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		a.showHelp = !a.showHelp
		return nil
	}
	if a.showHelp {
		a.sleepWakeArmed = false
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
			inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			a.showHelp = false
		}
		return nil
	}
	// The options card is the same kind of mode, with its own keys.
	if a.showOptions {
		a.updateOptions()
		a.sleepWakeArmed = false
		return nil
	}
	if art := a.originalTitle(); a.usesOriginalTitle() && art != nil && a.originalTitleShowsNotice(art) {
		// The notice ends on a button, like sign_on's set_timeout loop; the
		// key that ends it is not also a menu choice.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) ||
			inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) || len(inpututil.AppendJustPressedKeys(nil)) > 0 {
			art.noticeDone = true
		}
		return nil
	}
	if a.showCredits {
		// sign_on:show_credits waits for a button; any key does here.
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) ||
			inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) || len(inpututil.AppendJustPressedKeys(nil)) > 0 {
			a.showCredits = false
		}
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) && !isFailureTerminal(a.state) {
		a.openOptions()
		return nil
	}
	// One semantic VBL per update is presentation pacing. The rules retain
	// the independent event phase and worker wait; this is not physical Hz.
	a.step(game.ActionVBLTick)
	// XLANDFX runs as an overlay over the ground rather than a screen of its
	// own, so it advances on its own frames and any key cuts it short.
	if a.state.Landing.Phase != game.LandingIdle && !a.state.Sleep.Active {
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
			a.step(game.ActionSkipLanding)
		} else {
			a.step(game.ActionLandingTick)
		}
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF10) {
		a.step(game.ActionQuit)
	}
	// F5 opens the save library over a game in progress. The remake keeps
	// eight slots where the original's disk kept one game in one place, so a
	// save is a choice of slot rather than an overwrite.
	if inpututil.IsKeyJustPressed(ebiten.KeyF5) && !isFailureTerminal(a.state) {
		a.step(game.ActionOpenLibrary)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF7) {
		if a.sounds.toggleMute() {
			a.state.MessageKey = "ui.message.sound_off"
		} else {
			a.state.MessageKey = "ui.message.sound_on"
		}
	}
	// F8 walks the presentation presets. The colours, the tile art and the
	// layout move together; see internal/theme/presentation.go for why they
	// stopped being three separate keys.
	if inpututil.IsKeyJustPressed(ebiten.KeyF8) && !isFailureTerminal(a.state) {
		a.step(game.ActionCyclePresentation)
		a.savePreferences()
	}
	if a.state.Sleep.Active && a.state.Screen == game.ScreenCharacterStatus {
		wakeHeld := ebiten.IsKeyPressed(ebiten.KeyEscape) || ebiten.IsKeyPressed(ebiten.KeySpace) ||
			ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeyR) ||
			ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) || ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight)
		// Release the input that opened sleep before accepting a new wake.
		if !wakeHeld {
			a.sleepWakeArmed = true
		}
		if wakeHeld && a.sleepWakeArmed {
			a.step(game.ActionWake)
		}
		// One logical inner-loop cycle per update; this is presentation
		// pacing, not a claim about the original's hardware wall clock.
		a.step(game.ActionSleepTick)
		if a.state.QuitRequested {
			a.autosave()
			return ebiten.Termination
		}
		return nil
	}
	// While the mouse steers (do_guns), a button ends the steering before it
	// means anything else -- the original's loop exits on the press.
	slewing := a.state.Screen == game.ScreenTactical && a.state.TacticalSlewing
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) && !isFailureTerminal(a.state) {
		if slewing {
			a.step(game.ActionTacticalStop)
		} else {
			a.step(game.ActionBack)
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		if slewing {
			a.step(game.ActionTacticalStop)
		} else if a.state.HandItem != 0 && a.state.HandOrigin.Kind != "" {
			a.step(game.ActionCancelItem)
		} else if a.state.Screen == game.ScreenAttributeAllocation {
			a.step(game.ActionConfirmAttributes)
		} else if !isFailureTerminal(a.state) {
			a.step(semanticinput.RightClickAction(a.state.Screen))
		}
	}
	if a.state.Screen == game.ScreenCharacterLibrary {
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
			a.cycleLibrarySlot(-1)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
			a.cycleLibrarySlot(1)
		}
		// The list shows every slot, so a number picks one outright.
		for index, key := range librarySlotKeys {
			if index < len(prototypeLibrarySlots) && inpututil.IsKeyJustPressed(key) {
				a.selectLibrarySlot(index)
			}
		}
	}
	if a.state.Screen == game.ScreenCharacterCreation {
		if chars := ebiten.InputChars(); len(chars) > 0 {
			a.state = game.AppendCharacterNameDraft(a.state, chars)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
			a.state = game.BackspaceCharacterNameDraft(a.state)
		}
	}
	for _, key := range polledKeys {
		if inpututil.IsKeyJustPressed(key) {
			if action, ok := keyAction(key, a.state); ok {
				a.step(action)
			}
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		if a.state.Screen == game.ScreenCharacterLibrary {
			if slot, ok := a.librarySlotAtDisplay(x, y); ok {
				a.selectLibrarySlot(slot)
				return nil
			}
		}
		if action := a.mouseActionAtDisplay(x, y); action != "" {
			// Under the pixel walk the button is the walk itself, not a cell
			// step: xmove_on_ground moves the sprite while it is held and does
			// nothing on the press (stground.go). The arena is the same, and
			// there the whole window is the floor — it has no bands at all.
			inRealTimeArena := a.usesSTArena() && a.state.Screen == game.ScreenArena
			if !(a.usesSTGroundWalker() && isGroundMove(action)) && !inRealTimeArena {
				a.step(action)
			}
		} else if a.state.Screen == game.ScreenTactical && a.state.TacticalSlewing {
			// update_fight: the left button while steering is the trigger.
			a.step(game.ActionFire)
		}
	}
	if a.state.Screen == game.ScreenTactical && a.state.TacticalSlewing &&
		a.frames%(ebiten.TPS()/game.SlewTicksPerSecond) == 0 {
		x, y := a.toLayout(ebiten.CursorPosition())
		a.state = a.reducer.Slew(a.state, x, y)
		// replot_screen: the starfield slides against the turn (G11 -= Ge,
		// G10 -= Gd).
		a.starScrollX -= a.state.SlewBearing
		a.starScrollY -= a.state.SlewPitch
	}
	// 船艙的小人與地面的連續走路都在輸入之後跑：這一幀的按鍵已經反映在 state 上。
	a.updateSTShipWalker()
	a.updateSTGroundWalker()
	a.updateSTArena()
	a.updateGroundWalkRepeat()
	if a.state.QuitRequested {
		a.autosave()
		return ebiten.Termination
	}
	return nil
}

// autosave writes the writable slot on the way out, so quitting and relaunching
// with -load resumes where the player stopped. It is the same file the S key
// writes. The menus are skipped: a title-screen state carries nothing worth
// keeping and saving it would overwrite a real game with an empty one.
func (a *app) autosave() {
	if isMenuScreen(a.state.Screen) {
		return
	}
	// Every action clears the saved flag, and quitting is an action, so the
	// state arriving here always reads unsaved even when the player pressed S
	// a moment ago. What goes to disk has been written by definition, so the
	// flag is set on the way out — otherwise the autosave would overwrite a
	// deliberate save with a copy that claims it was never saved.
	saved := a.state
	saved.Saved = true
	if err := storage.Save(a.savePath, saved); err != nil {
		log.Printf("autosave failed: %v", err)
	}
}

// warnsBeforeDiscardingTheGame holds the first New Game back when the game in
// progress has not been stored, and reports that it did. The manual's page 5
// puts it as "any current game not stored in the library will be lost
// permanently" -- the original guards that by refusing to start until the
// player deletes the old game, which is a constraint of its single save slot.
// This shell has a multi-slot library and no such constraint, so it warns once
// and lets the second press through. The armed flag is not part of the save:
// it is a property of this sitting, like the attribute-allocation warning.
func (a *app) warnsBeforeDiscardingTheGame() bool {
	if a.newGameArmed {
		a.newGameArmed = false
		return false
	}
	if a.state.Character == "" || a.state.Saved || a.state.TimeMinutes == 0 {
		// Nothing worth losing: no character, already stored, or not yet played.
		return false
	}
	a.newGameArmed = true
	a.state.MessageKey = "ui.message.new_game_discards"
	return true
}

func (a *app) step(action game.Action) {
	if isGroundMove(action) && a.state.Screen == game.ScreenGround && a.usesSTGroundWalker() {
		a.nudgeSTGroundWalker(action)
		return
	}
	before := a.state
	defer func() {
		if a.state.Sleep.Active && (!before.Sleep.Active || before.Screen != game.ScreenCharacterStatus) {
			a.sleepWakeArmed = false
		}
		a.sounds.play(soundCue(before, a.state))
		a.sounds.play(soundFollowUp(before, a.state))
	}()

	defer func() {
		if a.state.MessageKey != before.MessageKey && a.state.Screen == before.Screen {
			a.stMessageFrame = a.frames
		}
		if a.state.MessageKey != before.MessageKey || a.state.Screen != before.Screen {
			a.stSceneMessageFrame = a.frames
		}
	}()
	if action == game.ActionLoadGame {
		a.loadLibraryGame()
		return
	}
	if action == game.ActionNewGame && a.warnsBeforeDiscardingTheGame() {
		return
	}
	if action == game.ActionOpenOptions {
		a.openOptions()
		return
	}
	if action == game.ActionAboutSunDog {
		if art := a.originalTitle(); art != nil && art.credits[0] != nil {
			a.showCredits = true
		} else {
			a.state.MessageKey = "ui.message.about_needs_disk"
		}
		return
	}
	switch action {
	case game.ActionMakeLibrary:
		if err := storage.MakeLibrary(a.libraryPath); err != nil {
			a.state.MessageKey = "ui.message.library_missing"
			if errors.Is(err, storage.ErrOldSave) {
				a.state.MessageKey = "ui.message.old_save_unsupported"
			}
			return
		}
	case game.ActionStoreGame:
		if err := storage.StoreGame(a.libraryPath, a.librarySlot, a.state); err != nil {
			a.state.MessageKey = "ui.message.library_missing"
			if errors.Is(err, storage.ErrOldSave) {
				a.state.MessageKey = "ui.message.old_save_unsupported"
			}
			return
		}
	case game.ActionDeleteGame:
		if err := storage.DeleteGame(a.libraryPath, a.librarySlot); err != nil {
			a.state.MessageKey = "ui.message.library_missing"
			if errors.Is(err, storage.ErrOldSave) {
				a.state.MessageKey = "ui.message.old_save_unsupported"
			}
			return
		}
	}
	next, err := a.reducer.Step(a.state, action)
	if err != nil {
		a.state.MessageKey = "ui.message.unknown_command"
		return
	}
	a.state = next
	if before.Screen != game.ScreenTitle && next.Screen == game.ScreenTitle {
		a.enterOriginalTitle()
	}
	if action == game.ActionSave {
		if err := storage.Save(a.savePath, a.state); err != nil {
			log.Printf("save failed: %v", err)
			a.state.MessageKey = "ui.message.save_failed"
		}
	}
}

func (a *app) loadSave() {
	loaded, err := storage.Load(a.savePath)
	if err != nil {
		a.state.MessageKey = "ui.message.load_failed"
		if errors.Is(err, storage.ErrOldSave) {
			a.state.MessageKey = "ui.message.old_save_unsupported"
		}
		return
	}
	a.state = resumeState(loaded)
}

func (a *app) loadLibraryGame() {
	loaded, libraryErr := storage.LoadGame(a.libraryPath, a.librarySlot)
	if errors.Is(libraryErr, storage.ErrOldSave) {
		a.state.MessageKey = "ui.message.old_save_unsupported"
		return
	}
	if libraryErr != nil {
		// The current single save remains available before a library exists.
		var saveErr error
		loaded, saveErr = storage.Load(a.savePath)
		if saveErr != nil {
			a.state.MessageKey = "ui.message.library_missing"
			if errors.Is(saveErr, storage.ErrOldSave) {
				a.state.MessageKey = "ui.message.old_save_unsupported"
			}
			return
		}
	}
	a.state = resumeState(loaded)
}

// resumeState removes process-lifetime flags from a persisted game. In
// particular, an autosave made while handling Quit contains QuitRequested;
// carrying that flag into the next process would make -load terminate before
// it can draw a frame.
func resumeState(loaded game.State) game.State {
	loaded.MessageKey = "ui.message.loaded"
	loaded.QuitRequested = false
	loaded.Saved = false
	return loaded
}

// librarySlotKeys is the number row the slot list answers to, one key a slot.
var librarySlotKeys = []ebiten.Key{
	ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4,
	ebiten.Key5, ebiten.Key6, ebiten.Key7, ebiten.Key8,
}

// librarySlotAtDisplay is which slot cell a click landed on, in the same two
// columns drawLibrary paints.
func (a *app) librarySlotAtDisplay(displayX, displayY int) (int, bool) {
	x, y := a.toLayout(displayX, displayY)
	for index := range prototypeLibrarySlots {
		cellX := librarySlotColX
		row := index
		if index >= librarySlotRows {
			cellX = librarySlotColX2
			row = index - librarySlotRows
		}
		cellY := librarySlotsY + row*librarySlotStep
		if x >= cellX-8 && x < cellX+librarySlotColW && y >= cellY-2 && y < cellY+librarySlotStep-2 {
			return index, true
		}
	}
	return 0, false
}

func (a *app) selectLibrarySlot(index int) {
	if index < 0 || index >= len(prototypeLibrarySlots) {
		return
	}
	a.librarySlot = prototypeLibrarySlots[index]
	a.state.MessageKey = "ui.message.library_slot_selected"
}

func (a *app) cycleLibrarySlot(delta int) {
	index := 0
	for candidate, slot := range prototypeLibrarySlots {
		if slot == a.librarySlot {
			index = candidate
			break
		}
	}
	index = (index + delta + len(prototypeLibrarySlots)) % len(prototypeLibrarySlots)
	a.librarySlot = prototypeLibrarySlots[index]
	a.state.MessageKey = "ui.message.library_slot_selected"
}

// polledKeys is the set of keys Update asks inpututil about each frame. A key
// that keyAction maps but that is missing here is silently dead at runtime,
// which no keyAction unit test can catch; TestPolledKeysCoverEveryMappedKey
// holds the two lists together.
var polledKeys = []ebiten.Key{
	ebiten.KeyEnter, ebiten.KeySpace, ebiten.KeyP, ebiten.KeyC, ebiten.KeyN,
	ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4, ebiten.Key5, ebiten.Key6,
	ebiten.KeyG, ebiten.KeyT, ebiten.KeyX, ebiten.KeyE, ebiten.KeyD,
	ebiten.KeyW, ebiten.KeyA, ebiten.KeyS, ebiten.KeyB, ebiten.KeyU,
	ebiten.KeyF, ebiten.KeyR, ebiten.KeyO, ebiten.KeyL, ebiten.KeyM,
	ebiten.KeyQ, ebiten.KeyH, ebiten.KeyK, ebiten.KeyV, ebiten.KeyI,
	ebiten.KeyY, ebiten.KeyJ,
	ebiten.KeyArrowLeft, ebiten.KeyArrowRight,
	ebiten.KeyArrowUp, ebiten.KeyArrowDown,
	ebiten.KeyBracketLeft, ebiten.KeyBracketRight,
}

// separatelyPolledKeys have their own handler in Update ahead of the loop
// because they carry extra gating, such as the failure-terminal lock. They are
// live at runtime even though they are absent from polledKeys.
var separatelyPolledKeys = []ebiten.Key{
	ebiten.KeyEscape, ebiten.KeyF1, ebiten.KeyF2, ebiten.KeyF5, ebiten.KeyF6, ebiten.KeyF7, ebiten.KeyF8, ebiten.KeyF10,
}

// typesText names the screens whose keystrokes belong to the player's text
// rather than to the command set. Letters must not double as commands there.
func typesText(screen game.Screen) bool {
	return screen == game.ScreenCharacterCreation
}

func keyAction(key ebiten.Key, state game.State) (game.Action, bool) {
	screen := state.Screen
	// Q is the long-standing way out and stays live wherever a keystroke is a
	// command. Where a keystroke is a letter of what the player is typing it is
	// not: a character named 「Quinn」 would quit the game on its first letter.
	// F10 is the way out on those screens, and it is a key no name contains.
	if key == ebiten.KeyQ && !typesText(screen) {
		return game.ActionQuit, true
	}
	switch screen {
	case game.ScreenTitle:
		switch key {
		case ebiten.KeyP, ebiten.KeyEnter, ebiten.KeySpace:
			return game.ActionPlay, true
		case ebiten.KeyC:
			return game.ActionSelectGame, true
		case ebiten.KeyL:
			return game.ActionLoadGame, true
		case ebiten.KeyO:
			return game.ActionOpenOptions, true
		case ebiten.KeyA:
			return game.ActionAboutSunDog, true
		}
	case game.ScreenCharacterLibrary:
		if key == ebiten.KeyN || key == ebiten.KeyEnter || key == ebiten.KeySpace {
			return game.ActionNewGame, true
		}
		if key == ebiten.KeyM {
			return game.ActionMakeLibrary, true
		}
		if key == ebiten.KeyS {
			return game.ActionStoreGame, true
		}
		if key == ebiten.KeyD {
			return game.ActionDeleteGame, true
		}
		if key == ebiten.KeyL {
			return game.ActionLoadGame, true
		}
	case game.ScreenCharacterCreation:
		// ENTER alone confirms. Space is a character a name may contain, and a
		// key cannot be both a letter and the button that ends the typing.
		if key == ebiten.KeyEnter {
			return game.ActionConfirmCharacterName, true
		}
	case game.ScreenAttributeAllocation:
		switch key {
		case ebiten.KeyArrowUp:
			return game.ActionPreviousAttribute, true
		case ebiten.KeyArrowDown:
			return game.ActionNextAttribute, true
		case ebiten.KeyArrowRight:
			return game.ActionIncreaseAttribute, true
		case ebiten.KeyArrowLeft:
			return game.ActionDecreaseAttribute, true
		case ebiten.KeyEnter, ebiten.KeySpace:
			return game.ActionConfirmAttributes, true
		}
	case game.ScreenCharacterStatus:
		switch key {
		case ebiten.KeyArrowLeft:
			return game.ActionPreviousPocket, true
		case ebiten.KeyArrowRight:
			return game.ActionNextPocket, true
		case ebiten.KeyR, ebiten.KeyEnter, ebiten.KeySpace:
			return game.ActionRest, true
		case ebiten.KeyE:
			return game.ActionEat, true
		case ebiten.KeyH:
			return game.ActionHand, true
		}
	case game.ScreenShipboard:
		switch key {
		case ebiten.KeyB:
			return game.ActionOpenStores, true
		case ebiten.KeyG:
			return game.ActionOpenGround, true
		case ebiten.KeyN:
			return game.ActionOpenNavigation, true
		case ebiten.KeyX:
			return game.ActionOpenTactical, true
		case ebiten.KeyE:
			return game.ActionOpenEngineering, true
		case ebiten.KeyC:
			return game.ActionOpenCargo, true
		case ebiten.KeyH:
			return game.ActionOpenStatus, true
		case ebiten.KeyD:
			return game.ActionDistressCall, true
		case ebiten.KeyS:
			return game.ActionSave, true
		}
	case game.ScreenStores:
		switch key {
		case ebiten.KeyArrowLeft, ebiten.KeyArrowUp:
			return game.ActionPreviousStore, true
		case ebiten.KeyArrowRight, ebiten.KeyArrowDown:
			return game.ActionNextStore, true
		case ebiten.KeyEnter, ebiten.KeySpace:
			return game.ActionTakeStore, true
		case ebiten.KeyH:
			return game.ActionHand, true
		case ebiten.KeyE:
			return game.ActionEat, true
		}
	case game.ScreenGround:
		if key == ebiten.KeyC {
			return game.ActionTalk, true
		}
		switch key {
		case ebiten.KeyArrowUp, ebiten.KeyW:
			return game.ActionMoveNorth, true
		case ebiten.KeyArrowDown:
			return game.ActionMoveSouth, true
		case ebiten.KeyArrowLeft:
			return game.ActionMoveWest, true
		case ebiten.KeyArrowRight:
			return game.ActionMoveEast, true
		case ebiten.KeyS:
			return game.ActionMoveSouth, true
		case ebiten.KeyA:
			return game.ActionMoveWest, true
		case ebiten.KeyD:
			return game.ActionOpenUniteller, true
		case ebiten.KeyR:
			return game.ActionRefuel, true
		case ebiten.KeyF:
			return game.ActionOpenDepot, true
		case ebiten.KeyT:
			return game.ActionOpenTrade, true
		case ebiten.KeyU:
			return game.ActionOpenUniteller, true
		case ebiten.KeyP:
			return game.ActionLoadSupplies, true
		case ebiten.KeyK:
			return game.ActionLoadCryogens, true
		case ebiten.KeyV:
			return game.ActionDeliverMission, true
		case ebiten.KeyO:
			return game.ActionLoadAuxFuel, true
		case ebiten.KeyH:
			return game.ActionOpenStatus, true
		case ebiten.KeyY:
			return game.ActionOpenParts, true
		case ebiten.KeyB:
			return game.ActionOpenTubeStation, true
		case ebiten.KeyE:
			return game.ActionEnterBuilding, true
		}
	case game.ScreenDepot:
		switch key {
		case ebiten.Key1:
			return game.ActionRefuel, true
		case ebiten.Key2:
			return game.ActionRepairHull, true
		case ebiten.Key3:
			return game.ActionLoadAuxFuel, true
		case ebiten.Key4:
			if state.CityID == "banville" {
				return game.ActionOpenFTLBoost, true
			}
		}
	case game.ScreenFuelPurchase:
		switch key {
		case ebiten.Key1, ebiten.KeyEnter, ebiten.KeySpace:
			return game.ActionFuelBuyFull, true
		case ebiten.Key2:
			return game.ActionFuelBuyOne, true
		case ebiten.Key3:
			return game.ActionFuelCancel, true
		}
	case game.ScreenFTLBoost:
		if key == ebiten.Key1 || key == ebiten.KeyEnter {
			return game.ActionInstallFTLBoost, true
		}
		if key == ebiten.Key3 {
			return game.ActionBack, true
		}
	case game.ScreenInterior:
		switch key {
		case ebiten.KeyArrowUp, ebiten.KeyW:
			return game.ActionInteriorNorth, true
		case ebiten.KeyArrowDown, ebiten.KeyS:
			return game.ActionInteriorSouth, true
		case ebiten.KeyArrowLeft, ebiten.KeyA:
			return game.ActionInteriorWest, true
		case ebiten.KeyArrowRight, ebiten.KeyD:
			return game.ActionInteriorEast, true
		case ebiten.KeyC:
			return game.ActionUseCounter, true
		case ebiten.KeyB:
			return game.ActionSitAtBooth, true
		case ebiten.KeyG:
			return game.ActionOpenSlots, true
		case ebiten.KeyE:
			return game.ActionLeaveBuilding, true
		}
	case game.ScreenSlots:
		if key == ebiten.KeySpace || key == ebiten.KeyEnter {
			return game.ActionSpinSlots, true
		}
	case game.ScreenArena:
		switch key {
		case ebiten.KeySpace, ebiten.KeyEnter, ebiten.KeyF:
			return game.ActionArenaAttack, true
		case ebiten.KeyX:
			return game.ActionArenaFlee, true
		}
	case game.ScreenParts:
		if key == ebiten.KeyB || key == ebiten.KeyEnter || key == ebiten.KeySpace {
			return game.ActionBuyPart, true
		}
		if key == ebiten.KeyArrowLeft {
			return game.ActionPreviousPart, true
		}
		if key == ebiten.KeyArrowRight {
			return game.ActionNextPart, true
		}
	case game.ScreenTrade:
		// Under the original's layout the exchange has its pages: Buy and
		// Sell open the quote and the lot list, and buy or sell from there.
		switch state.TradePage {
		case game.TradePageEntrance:
			if key == ebiten.KeyE || key == ebiten.KeyEnter || key == ebiten.KeySpace {
				return game.ActionTradeExchange, true
			}
			if key == ebiten.KeyW {
				return game.ActionTradeWarehousePage, true
			}
			return "", false
		case game.TradePageWarehouse:
			if key == ebiten.KeyS {
				return game.ActionTradeStorePage, true
			}
			if key == ebiten.KeyF {
				return game.ActionTradeFetchPage, true
			}
			if key == ebiten.KeyX {
				return game.ActionTradeSwapPage, true
			}
			return "", false
		case game.TradePageSell, game.TradePageStore, game.TradePageFetch, game.TradePageSwapPod, game.TradePageSwapWarehouse, game.TradePageSellSource, game.TradePageSellWarehouse, game.TradePageSellConfirm, game.TradePageBuyDestination:
			switch key {
			case ebiten.Key1:
				return game.ActionTradePick1, true
			case ebiten.Key2:
				return game.ActionTradePick2, true
			case ebiten.Key3:
				return game.ActionTradePick3, true
			case ebiten.Key4:
				return game.ActionTradePick4, true
			}
			return "", false
		}
		paged := state.Layout == theme.AtariSTLayout && state.TradePage == game.TradePageMain
		if key == ebiten.KeyB || key == ebiten.KeyEnter || key == ebiten.KeySpace {
			if paged {
				return game.ActionTradeBuyPage, true
			}
			return game.ActionBuyGems, true
		}
		if key == ebiten.KeyS {
			if paged {
				return game.ActionTradeSellPage, true
			}
			return game.ActionSellGems, true
		}
		if key == ebiten.KeyP && state.TradePage == game.TradePageBuy {
			return game.ActionTradePass, true
		}
	case game.ScreenNavigation:
		if key == ebiten.KeyArrowLeft {
			return game.ActionPreviousDestination, true
		}
		if key == ebiten.KeyArrowRight {
			return game.ActionNextDestination, true
		}
		if key == ebiten.KeyArrowUp {
			return game.ActionPreviousTargetPlanet, true
		}
		if key == ebiten.KeyArrowDown {
			return game.ActionNextTargetPlanet, true
		}
		if key == ebiten.KeyBracketLeft {
			return game.ActionPreviousTargetSystem, true
		}
		if key == ebiten.KeyBracketRight {
			return game.ActionNextTargetSystem, true
		}
		if key == ebiten.KeyEnter || key == ebiten.KeySpace || key == ebiten.KeyG {
			return game.ActionNavigate, true
		}
	case game.ScreenFlight:
		if key == ebiten.KeyEnter || key == ebiten.KeySpace {
			return game.ActionFlightTick, true
		}
		if key == ebiten.KeyE {
			return game.ActionFlightEngage, true
		}
		if key == ebiten.KeyD {
			return game.ActionDistressCall, true
		}
		if key == ebiten.KeyR {
			return game.ActionFlightResume, true
		}
	case game.ScreenDialog:
		if key == ebiten.KeyArrowLeft {
			return game.ActionPreviousVerb, true
		}
		if key == ebiten.KeyArrowRight {
			return game.ActionNextVerb, true
		}
		if key == ebiten.KeyEnter || key == ebiten.KeySpace {
			return game.ActionSayVerb, true
		}
	case game.ScreenTubeStation:
		if state.TubePage == game.TubePageConfirm {
			// 確認頁只有一個框：TRANSPORT。
			switch key {
			case ebiten.KeyEnter, ebiten.KeySpace, ebiten.KeyT:
				return game.ActionRideTube, true
			}
			break
		}
		// do_tube_station 的目的地清單走 pick_location：1–6 對應各個框。
		switch key {
		case ebiten.Key1:
			return game.ActionTubePick1, true
		case ebiten.Key2:
			return game.ActionTubePick2, true
		case ebiten.Key3:
			return game.ActionTubePick3, true
		case ebiten.Key4:
			return game.ActionTubePick4, true
		case ebiten.Key5:
			return game.ActionTubePick5, true
		case ebiten.Key6:
			return game.ActionTubePick6, true
		}
	case game.ScreenUniteller:
		if state.TellerPage == game.TellerPageTransferPick {
			// pick_location: 1–6 the boxes, O for OTHERS.
			switch key {
			case ebiten.Key1:
				return game.ActionTellerPick1, true
			case ebiten.Key2:
				return game.ActionTellerPick2, true
			case ebiten.Key3:
				return game.ActionTellerPick3, true
			case ebiten.Key4:
				return game.ActionTellerPick4, true
			case ebiten.Key5:
				return game.ActionTellerPick5, true
			case ebiten.Key6:
				return game.ActionTellerPick6, true
			case ebiten.KeyO:
				return game.ActionTellerOthers, true
			}
			return "", false
		}
		if state.TellerPage != game.TellerPageMain {
			// The keypad: A all, 1–4 the four amounts, C clear.
			switch key {
			case ebiten.KeyA:
				return game.ActionTellerAmountAll, true
			case ebiten.Key1:
				return game.ActionTellerAmount100, true
			case ebiten.Key2:
				return game.ActionTellerAmount1000, true
			case ebiten.Key3:
				return game.ActionTellerAmount10000, true
			case ebiten.Key4:
				return game.ActionTellerAmount50000, true
			case ebiten.KeyC:
				return game.ActionTellerAmountClear, true
			}
			return "", false
		}
		// Under the original's layout the three cash moves go through the
		// keypad; the remake layout moves everything in one step.
		keypad := state.Layout == theme.AtariSTLayout
		if key == ebiten.KeyD || key == ebiten.KeyEnter || key == ebiten.KeySpace {
			if keypad {
				return game.ActionTellerDepositPage, true
			}
			return game.ActionDepositCash, true
		}
		if key == ebiten.KeyW {
			if keypad {
				return game.ActionTellerWithdrawPage, true
			}
			return game.ActionWithdrawCash, true
		}
		if key == ebiten.KeyT {
			if keypad {
				return game.ActionTellerTransferPage, true
			}
			return game.ActionTransferBank, true
		}
		if key == ebiten.KeyF {
			return game.ActionPayFine, true
		}
		if key == ebiten.KeyU {
			return game.ActionRideTube, true
		}
	case game.ScreenTactical:
		switch key {
		case ebiten.KeyM:
			return game.ActionTacticalMove, true
		case ebiten.KeyF, ebiten.KeyEnter, ebiten.KeySpace:
			return game.ActionFire, true
		case ebiten.KeyG:
			return game.ActionFireLaser, true
		case ebiten.KeyT:
			return game.ActionTractor, true
		case ebiten.KeyJ:
			return game.ActionJettisonHold1, true
		case ebiten.KeyK:
			return game.ActionJettisonHold2, true
		case ebiten.KeyB:
			return game.ActionJettisonAll, true
		case ebiten.KeyH:
			return game.ActionHail, true
		case ebiten.KeyR:
			return game.ActionRechargeWeapon, true
		case ebiten.KeyO:
			return game.ActionShieldsOff, true
		case ebiten.KeyU:
			return game.ActionFullShields, true
		}
	case game.ScreenEngineering:
		switch key {
		case ebiten.KeyArrowLeft:
			return game.ActionPreviousSystem, true
		case ebiten.KeyArrowRight:
			return game.ActionNextSystem, true
		case ebiten.KeyE:
			return game.ActionRepairSystem, true
		case ebiten.KeyI:
			return game.ActionOpenEquipment, true
		case ebiten.KeyW:
			return game.ActionRepairWeapon, true
		case ebiten.KeyS:
			return game.ActionRepairShields, true
		case ebiten.KeyR, ebiten.KeyEnter, ebiten.KeySpace:
			return game.ActionRepair, true
		}
	case game.ScreenCargo:
		if key == ebiten.KeyE {
			return game.ActionExitCargoPod, true
		}
		if key == ebiten.KeyL || key == ebiten.KeyEnter || key == ebiten.KeySpace {
			return game.ActionLoadCargo, true
		}
		if key == ebiten.KeyU {
			return game.ActionUnloadCargo, true
		}
		if key == ebiten.KeyO {
			return game.ActionUnloadAuxFuel, true
		}
	case game.ScreenOutcome:
		if state.OutcomeKey == "ui.outcome.failure" {
			if key == ebiten.KeyS {
				return game.ActionSave, true
			}
			return "", false
		}
		if key == ebiten.KeyEnter || key == ebiten.KeySpace || key == ebiten.KeyEscape {
			return game.ActionBack, true
		}
	}
	return "", false
}

func (a *app) mouseActionAt(x, y int) game.Action {
	if isFailureTerminal(a.state) {
		return ""
	}
	if a.state.Screen == game.ScreenCharacterStatus && a.statusItemClick(x, y) {
		return ""
	}
	if a.usesOriginalTitle() && a.originalTitle() != nil {
		return originalTitleActionAt(x, y)
	}
	if a.usesSTShipboard() {
		// 原版的船艙沒有「點矩形就到達」這回事：按住左鍵小人才走，放開時它站在哪
		// 個站位裡才分派（stwalker.go）。留著這條捷徑等於讓小人沒有意義，而且會
		// 讓視窗 66、71 這種原版走不進去的矩形變成點一下就進去。
		return ""
	}
	if a.usesSTShipboardBackdrop() {
		if action, ok := a.stZoomActionAt(x, y); ok {
			return action
		}
		return ""
	}
	if a.usesSTGround() {
		return a.stGroundActionAt(x, y)
	}
	if a.usesSTRepair() {
		if action, ok := a.stRepairClick(x, y); ok {
			return action
		}
		return ""
	}
	if a.usesSTGroundBackdrop() {
		if a.stZoomMessageFresh() && (a.state.Screen != game.ScreenTrade || a.state.TradePage == game.TradePageMain) {
			// delay_button: the click ends the message, nothing else.
			a.stMessageFrame = 0
			return ""
		}
		if a.state.Screen == game.ScreenTrade {
			if action, ok := a.stTradeActionAt(x, y); ok {
				return action
			}
			return ""
		}
		if a.state.Screen == game.ScreenUniteller {
			if action, ok := a.stTellerActionAt(x, y); ok {
				return action
			}
			return ""
		}
		if a.state.Screen == game.ScreenTubeStation {
			if action, ok := a.stTubeActionAt(x, y); ok {
				return action
			}
			return ""
		}
		if action, ok := a.stZoomActionAt(x, y); ok {
			return action
		}
	}
	if a.state.Screen == game.ScreenShipboard {
		if action, ok := a.shipboardPanelMouseAction(x, y); ok {
			return action
		}
	}
	if action, ok := remakePanelMouseAction(a.state.Screen, x, y); ok {
		if a.state.Screen == game.ScreenTrade && a.state.TradePage == game.TradePageSellConfirm {
			if action == game.ActionSellGems {
				return game.ActionTradePick2
			}
			if action == game.ActionBuyGems && game.TradeSaleAvailable(a.state) {
				return game.ActionTradePick1
			}
			return ""
		}
		// 電車站的確認頁只有一個動作（TRANSPORT），而點擊帶是靜態表、
		// 掛的是清單頁的六個框。頁一換，那些框就不是框了。
		if a.state.Screen == game.ScreenTubeStation && a.state.TubePage == game.TubePageConfirm {
			if selection, ok := a.reducer.TubeSelected(a.state); ok && selection.Affordable {
				return game.ActionRideTube
			}
			return ""
		}
		if !a.panelActionAvailable(action) {
			return ""
		}
		return action
	}
	if a.state.Screen == game.ScreenGround &&
		x >= sceneX && x < sceneX+world.ViewportWidth()*tilePixels &&
		y >= sceneY && y < sceneY+world.ViewportHeight()*tilePixels {
		// 視埠是城市地圖的一扇窗，所以點到的是窗座標，要先換回地圖座標。
		originX, originY := world.ViewportOrigin(a.state.Position.X, a.state.Position.Y)
		target := game.Position{X: originX + (x-sceneX)/tilePixels, Y: originY + (y-sceneY)/tilePixels}
		return a.groundStepToward(target)
	}
	// 遊戲畫面沒有頁尾條了——那塊在原版的框裡是訊息框，不接受點擊。
	if inGameFrame(a.state.Screen) || y < 157 {
		return ""
	}
	width := logicalWidth
	if x < 0 {
		x = 0
	}
	if x >= width {
		x = width - 1
	}
	return footerMouseAction(a.state.Screen, x)
}

// The static panel table is capacity; only destinations on this page are active.
func (a *app) panelActionAvailable(action game.Action) bool {
	if action == game.ActionOpenFTLBoost {
		return a.state.Screen == game.ScreenDepot && a.state.CityID == "banville"
	}
	if a.state.Screen == game.ScreenTubeStation && a.state.TubePage == game.TubePageList {
		for index, pick := range stTubePickActions {
			if action == pick {
				return index < len(a.reducer.TubeDestinations(a.state))
			}
		}
	}
	return true
}

// mouseActionAtDisplay translates physical-window input back to the retained
// 320×200 UI contract. Modern Apple ground art is intentionally different: it
// occupies a larger, high-resolution map viewport, while its reducer action
// still receives the same map coordinate and collision rules.
func (a *app) mouseActionAtDisplay(x, y int) game.Action {
	if action, ok := a.modernGroundMapActionAt(x, y); ok {
		return action
	}
	// 面板的點擊帶只有一份：mouseActionAt 的 groundBand。這裡曾經另外複製一份
	// 硬編座標，版面一動就與畫出來的列對不上，所以直接落到共用的那份。
	x, y = a.toLayout(x, y)
	return a.mouseActionAt(x, y)
}

// toLayout turns a window pixel back into the 320x200 layout grid, undoing the
// centring border first.
func (a *app) toLayout(x, y int) (int, int) {
	scale := a.viewScale()
	return (x - a.originX) / scale, (y - a.originY) / scale
}

func (a *app) modernGroundMapActionAt(x, y int) (game.Action, bool) {
	group, ok := a.reducer.GroupOf(a.state)
	if !ok || (!a.usesModernGroundRaster(group) && !a.usesOriginalGround()) {
		return "", false
	}
	scale := a.viewScale()
	tileSize := tilePixels * scale
	mapX, mapY := modernGroundOriginX*scale+a.originX, modernGroundOriginY*scale+a.originY
	mapWidth, mapHeight := world.ViewportWidth()*tileSize, world.ViewportHeight()*tileSize
	if x < mapX || x >= mapX+mapWidth || y < mapY || y >= mapY+mapHeight {
		return "", false
	}
	originX, originY := world.ViewportOrigin(a.state.Position.X, a.state.Position.Y)
	target := game.Position{X: originX + (x-mapX)/tileSize, Y: originY + (y-mapY)/tileSize}
	return a.groundStepToward(target), true
}

// The high-resolution ground raster belongs to the tile art, not to the
// interface colours, so it follows the tileset the player picked and shows
// under any chrome.
// usesOriginalGround reports whether the city floor is being drawn from the
// player's own disk. It shares the high-resolution ground's layout because it
// fills the same area: a panel over the left column would cover it.
func (a *app) usesOriginalGround() bool {
	return a.originalDisk != nil && a.state.Screen == game.ScreenGround
}

func (a *app) usesModernGroundRaster(group theme.Group) bool {
	if a.state.Screen != game.ScreenGround || a.state.GroundWorld || group.Raster == nil {
		return false
	}
	return group.TilesetID == theme.ModernTiles
}

type mouseHotspot struct {
	screen     game.Screen
	minX, minY int
	maxX, maxY int
	action     game.Action
}

// remakePanelMouseHotspots is owned by the remake layout in drawMenu and
// drawScreenPanel. These hit areas make visible remake actions clickable; they
// do not claim Apple II hit-test parity. shipboardPanelMouseAction remains the
// separate, evidence-backed original target mapping.
// panelBand is the click band for one row of the shared panel layout. Bands
// are derived from the row geometry instead of written out, because the two
// drifted apart without anyone noticing: the character status, tactical and
// dialog panels each ended up with their bands one or two rows above the text
// they belonged to, so the row a player aimed at ran a different action.
// TestPanelBandsCoverTheDrawnActionRows keeps them together from now on.
func panelBand(screen game.Screen, row int, action game.Action) mouseHotspot {
	y, _ := panelRowYFor(screen, row)
	return mouseHotspot{screen, panelX, y, panelX + panelW, y + panelRowHeight(), action}
}

// panelRowHeight is one button row's click band.
func panelRowHeight() int {
	if panelLayoutST {
		return stMenuPitch
	}
	return panelRowStep
}

// panelBandSplit is a row whose left half and right half do different things.
func panelBandSplit(screen game.Screen, row int, left, right game.Action) []mouseHotspot {
	y, _ := panelRowYFor(screen, row)
	return []mouseHotspot{
		{screen, panelX, y, panelX + panelW/2, y + panelRowStep, left},
		{screen, panelX + panelW/2, y, panelX + panelW, y + panelRowStep, right},
	}
}

// groundBand is the same idea for the ground panel, which packs its rows
// tighter and starts them below a location row and a facilities row that wraps.
func groundBand(row int, action game.Action) mouseHotspot {
	y, _ := panelRowYFor(game.ScreenGround, row)
	return mouseHotspot{game.ScreenGround, panelX, y, panelX + panelW, y + panelRowHeight(), action}
}

// routeBandSplit is the navigation panel's selector rows, which sit under the
// map rather than on the shared row grid.
func routeBandSplit(row int, left, right game.Action) []mouseHotspot {
	y, step := routeRowY+row*routeRowStep, routeRowStep
	if panelLayoutST {
		y, step = stMenuRowY(row)-stMenuTextY, stMenuPitch
	}
	return []mouseHotspot{
		{game.ScreenNavigation, panelX, y, panelX + panelW/2, y + step, left},
		{game.ScreenNavigation, panelX + panelW/2, y, panelX + panelW, y + step, right},
	}
}

// navigationMapBand is the map-as-confirm-button: the left box map in the
// remake layout, the viewport in the original's.
func navigationMapBand() mouseHotspot {
	if panelLayoutST {
		return mouseHotspot{game.ScreenNavigation, frameViewportX, frameViewportY, frameViewportX + frameViewportW, frameViewportY + frameViewportH, game.ActionNavigate}
	}
	return mouseHotspot{game.ScreenNavigation, routeMapX, routeMapY, routeMapX + routeMapWidth, routeMapY + routeMapHeight, game.ActionNavigate}
}

// remakePanelMouseHotspots is owned by the remake layout in drawMenu and
// drawScreenPanel. These hit areas make visible remake actions clickable; they
// do not claim Apple II hit-test parity. shipboardPanelMouseAction remains the
// separate, evidence-backed original target mapping.
//
// The row numbers below are indices into what screenPanelLines returns for that
// screen, so a row added or removed there moves its band with it.
// The hotspot tables, one per layout, built on first use under that layout.
var panelMouseHotspotTables = map[bool][]mouseHotspot{}

func panelMouseHotspots() []mouseHotspot {
	table, ok := panelMouseHotspotTables[panelLayoutST]
	if !ok {
		table = buildPanelMouseHotspots()
		panelMouseHotspotTables[panelLayoutST] = table
	}
	return table
}

func buildPanelMouseHotspots() []mouseHotspot {
	spots := []mouseHotspot{
		// The title and library screens draw their own layout, so their bands
		// stay written out.
		{game.ScreenTitle, 20, 66, 300, 84, game.ActionPlay},
		{game.ScreenTitle, 20, 84, 300, 100, game.ActionSelectGame},
		{game.ScreenTitle, 20, 100, 300, 116, game.ActionLoadGame},
		{game.ScreenTitle, 20, 116, 300, 132, game.ActionOpenOptions},

		// The library's five keys sit on one row now (libraryKeysY), so the
		// bands split that row five ways. The slot list above it is picked by
		// clicking a slot, which drawLibrary lays out in two columns.
		{game.ScreenCharacterLibrary, 20, libraryKeysY, 76, libraryKeysY + 14, game.ActionNewGame},
		{game.ScreenCharacterLibrary, 76, libraryKeysY, 132, libraryKeysY + 14, game.ActionMakeLibrary},
		{game.ScreenCharacterLibrary, 132, libraryKeysY, 188, libraryKeysY + 14, game.ActionStoreGame},
		{game.ScreenCharacterLibrary, 188, libraryKeysY, 244, libraryKeysY + 14, game.ActionLoadGame},
		{game.ScreenCharacterLibrary, 244, libraryKeysY, 300, libraryKeysY + 14, game.ActionDeleteGame},

		panelBand(game.ScreenCharacterStatus, 6, game.ActionRest),
		panelBand(game.ScreenCharacterStatus, 7, game.ActionEat),
		panelBand(game.ScreenStores, 3, game.ActionTakeStore),
		panelBand(game.ScreenStores, 4, game.ActionHand),
		panelBand(game.ScreenStores, 5, game.ActionEat),

		panelBand(game.ScreenFlight, 3, game.ActionFlightTick),
		panelBand(game.ScreenFlight, 4, game.ActionFlightEngage),
		panelBand(game.ScreenFlight, 5, game.ActionDistressCall),
		panelBand(game.ScreenFlight, 6, game.ActionFlightResume),

		panelBand(game.ScreenTrade, 5, game.ActionBuyGems),
		panelBand(game.ScreenTrade, 6, game.ActionSellGems),

		panelBand(game.ScreenUniteller, 2, game.ActionDepositCash),
		panelBand(game.ScreenUniteller, 3, game.ActionWithdrawCash),
		panelBand(game.ScreenUniteller, 4, game.ActionTransferBank),
		panelBand(game.ScreenUniteller, 5, game.ActionPayFine),

		// 一顆行星最多六座城市，所以電車站的清單最長五條——第六個框是
		// pick_location 為第六條留的，這裡填不到。
		panelBand(game.ScreenTubeStation, 1, game.ActionTubePick1),
		panelBand(game.ScreenTubeStation, 2, game.ActionTubePick2),
		panelBand(game.ScreenTubeStation, 3, game.ActionTubePick3),
		panelBand(game.ScreenTubeStation, 4, game.ActionTubePick4),
		panelBand(game.ScreenTubeStation, 5, game.ActionTubePick5),

		panelBand(game.ScreenTactical, 5, game.ActionTacticalMove),
		panelBand(game.ScreenTactical, 6, game.ActionFire),
		panelBand(game.ScreenTactical, 7, game.ActionRechargeWeapon),
		panelBand(game.ScreenTactical, 8, game.ActionFullShields),

		panelBand(game.ScreenDialog, 6, game.ActionSayVerb),

		panelBand(game.ScreenCargo, 4, game.ActionLoadCargo),
		panelBand(game.ScreenCargo, 5, game.ActionUnloadCargo),
		panelBand(game.ScreenCargo, 6, game.ActionUnloadAuxFuel),
		panelBand(game.ScreenCargo, 7, game.ActionExitCargoPod),
		panelBand(game.ScreenFuelPurchase, 4, game.ActionFuelBuyFull),
		panelBand(game.ScreenFuelPurchase, 5, game.ActionFuelBuyOne),
		panelBand(game.ScreenFuelPurchase, 6, game.ActionFuelCancel),
		panelBand(game.ScreenDepot, 0, game.ActionRefuel),
		panelBand(game.ScreenDepot, 1, game.ActionRepairHull),
		panelBand(game.ScreenDepot, 2, game.ActionLoadAuxFuel),
		panelBand(game.ScreenDepot, 3, game.ActionOpenFTLBoost),
		panelBand(game.ScreenFTLBoost, 3, game.ActionInstallFTLBoost),
		panelBand(game.ScreenFTLBoost, 4, game.ActionBack),

		groundBand(0, game.ActionOpenDepot),
		groundBand(1, game.ActionOpenTrade),
		groundBand(2, game.ActionOpenUniteller),
		groundBand(3, game.ActionLoadSupplies),
		groundBand(4, game.ActionLoadCryogens),
		groundBand(5, game.ActionDeliverMission),
		groundBand(6, game.ActionOpenTubeStation),
		groundBand(7, game.ActionLoadAuxFuel),

		// Confirming a route by mouse uses the map itself; the footer documents
		// ENTER.
		// The navigation map itself is the confirm button; it fills the panel
		// between the route line and the selector rows.
		navigationMapBand(),

		{game.ScreenOutcome, 20, 110, 300, 132, game.ActionBack},
	}
	spots = append(spots, panelBandSplit(game.ScreenDialog, 5, game.ActionPreviousVerb, game.ActionNextVerb)...)
	// The three selector rows each show the current choice and split
	// left/right, so the 12 systems, 18 planets and 58 cities stay reachable in
	// one panel.
	spots = append(spots, routeBandSplit(0, game.ActionPreviousTargetSystem, game.ActionNextTargetSystem)...)
	spots = append(spots, routeBandSplit(1, game.ActionPreviousTargetPlanet, game.ActionNextTargetPlanet)...)
	spots = append(spots, routeBandSplit(2, game.ActionPreviousDestination, game.ActionNextDestination)...)
	return spots
}

func remakePanelMouseAction(screen game.Screen, x, y int) (game.Action, bool) {
	for _, hotspot := range panelMouseHotspots() {
		if hotspot.screen != screen {
			continue
		}
		if x >= hotspot.minX && x < hotspot.maxX && y >= hotspot.minY && y < hotspot.maxY {
			return hotspot.action, true
		}
	}
	return "", false
}

// shipboardPanelMouseAction is intentionally evidence-backed and narrow. The
// Apple IIe oracle reaches the Sub-C Engines consumer from the corresponding
// shipboard target; other shipboard labels stay pending until their own
// consumer evidence exists.
// The row and column the Sub-C Engines station is drawn at, in the same grid
// drawShipboardPanel walks.
const (
	subcStationRow    = 3
	subcStationColumn = 1
)

func (a *app) shipboardPanelMouseAction(x, y int) (game.Action, bool) {
	// 座標從畫出來的那份表推：視窗大小隨內容變，寫死的矩形一定會失準。
	window, columnWidth := a.shipboardWindow()
	left := window.columnX(subcStationColumn, columnWidth)
	top := window.rowY(subcStationRow, shipboardRowStep)
	if x >= left && x < left+columnWidth && y >= top && y < top+shipboardRowStep {
		return game.ActionOpenEngineering, true
	}
	return "", false
}

func directionTo(current, target game.Position) game.Action {
	if target.X < current.X {
		return game.ActionMoveWest
	}
	if target.X > current.X {
		return game.ActionMoveEast
	}
	if target.Y < current.Y {
		return game.ActionMoveNorth
	}
	if target.Y > current.Y {
		return game.ActionMoveSouth
	}
	return game.ActionOpenTrade
}

func footerMouseAction(screen game.Screen, x int) game.Action {
	switch screen {
	case game.ScreenFuelPurchase:
		if x < logicalWidth/3 {
			return game.ActionFuelBuyFull
		}
		if x < logicalWidth*2/3 {
			return game.ActionFuelBuyOne
		}
		return game.ActionFuelCancel
	case game.ScreenTitle:
		if x < logicalWidth/4 {
			return game.ActionPlay
		}
		if x < logicalWidth/2 {
			return game.ActionSelectGame
		}
		if x < logicalWidth*3/4 {
			return game.ActionLoadGame
		}
		return game.ActionOpenOptions
	case game.ScreenCharacterLibrary:
		if x < logicalWidth/5 {
			return game.ActionNewGame
		}
		if x < logicalWidth*2/5 {
			return game.ActionMakeLibrary
		}
		if x < logicalWidth*3/5 {
			return game.ActionStoreGame
		}
		if x < logicalWidth*4/5 {
			return game.ActionLoadGame
		}
		return game.ActionDeleteGame
	case game.ScreenCharacterCreation:
		if x < logicalWidth*2/3 {
			return game.ActionConfirmCharacterName
		}
		return game.ActionBack
	case game.ScreenCharacterStatus:
		switch {
		case x < 106:
			return game.ActionRest
		case x < 213:
			return game.ActionEat
		default:
			return game.ActionBack
		}
	case game.ScreenShipboard:
		switch {
		case x < 60:
			return game.ActionOpenGround
		case x < 120:
			return game.ActionOpenNavigation
		case x < 180:
			return game.ActionOpenTactical
		case x < 240:
			return game.ActionOpenEngineering
		case x < 280:
			return game.ActionOpenCargo
		default:
			return game.ActionOpenStatus
		}
	case game.ScreenGround:
		switch {
		case x < 53:
			return game.ActionOpenTrade
		case x < 106:
			return game.ActionOpenUniteller
		case x < 160:
			return game.ActionRefuel
		case x < 213:
			return game.ActionLoadSupplies
		case x < 266:
			return game.ActionLoadCryogens
		default:
			return game.ActionDeliverMission
		}
	case game.ScreenTrade:
		if x < logicalWidth/2 {
			return game.ActionBuyGems
		}
		return game.ActionSellGems
	case game.ScreenParts:
		switch {
		case x < logicalWidth/3:
			return game.ActionPreviousPart
		case x < logicalWidth*2/3:
			return game.ActionNextPart
		default:
			return game.ActionBuyPart
		}
	case game.ScreenNavigation:
		switch {
		case x < logicalWidth/3:
			return game.ActionPreviousDestination
		case x < logicalWidth*2/3:
			return game.ActionNextDestination
		default:
			return game.ActionNavigate
		}
	case game.ScreenUniteller:
		switch {
		case x < logicalWidth/3:
			return game.ActionDepositCash
		case x < logicalWidth*2/3:
			return game.ActionWithdrawCash
		default:
			return game.ActionTransferBank
		}
	case game.ScreenTactical:
		switch {
		case x < 64:
			return game.ActionTacticalMove
		case x < 128:
			return game.ActionFire
		case x < 192:
			return game.ActionRechargeWeapon
		case x < 256:
			return game.ActionShieldsOff
		default:
			return game.ActionFullShields
		}
	case game.ScreenEngineering:
		switch {
		case x < logicalWidth/4:
			return game.ActionRepairWeapon
		case x < logicalWidth/2:
			return game.ActionRepairShields
		case x < logicalWidth*3/4:
			return game.ActionRepair
		default:
			return game.ActionOpenEquipment
		}
	case game.ScreenEquipment:
		return game.ActionBack
	case game.ScreenCargo:
		if x < logicalWidth/2 {
			return game.ActionLoadCargo
		}
		return game.ActionUnloadCargo
	case game.ScreenOutcome:
		return game.ActionBack
	default:
		return game.ActionBack
	}
}

func (a *app) Draw(screen *ebiten.Image) {
	group, ok := a.reducer.GroupOf(a.state)
	if !ok {
		return
	}
	setPanelLayout(a.state.Layout)
	if a.usesHD() {
		a.drawHD(screen, group)
		return
	}
	if a.uiLayer == nil {
		a.uiLayer = ebiten.NewImage(logicalWidth, logicalHeight)
	}
	if a.textLayer == nil {
		a.textLayer = ebiten.NewImage(screen.Bounds().Dx(), screen.Bounds().Dy())
	}
	a.textLayer.Clear()
	// The text goes on last so it sits above the ground raster, which is drawn
	// straight onto the window after the ui buffer is blown up.
	defer screen.DrawImage(a.textLayer, &ebiten.DrawImageOptions{})
	if a.state.Landing.Phase != game.LandingIdle && !a.state.Sleep.Active {
		a.drawLandingOverlay(screen, group)
		return
	}
	a.uiLayer.Fill(group.Palette[group.UI.Background])
	// The help card replaces the frame rather than sitting over it. Text is
	// composited last and above everything, so a card drawn on top of a live
	// screen would still have that screen's words showing through it.
	if a.showHelp {
		a.drawHeader(a.uiLayer, group)
		a.drawHelp(a.uiLayer, group)
		a.drawFooter(a.uiLayer, group)
		screen.Fill(group.Palette[group.UI.Background])
		helpOptions := &ebiten.DrawImageOptions{}
		helpOptions.Filter = ebiten.FilterNearest
		helpOptions.GeoM.Scale(float64(a.viewScale()), float64(a.viewScale()))
		helpOptions.GeoM.Translate(float64(a.originX), float64(a.originY))
		screen.DrawImage(a.uiLayer, helpOptions)
		return
	}
	if a.showOptions {
		if !a.options.diskOnly {
			a.drawHeader(a.uiLayer, group)
		}
		a.drawOptions(a.uiLayer, group)
		if !a.options.diskOnly {
			a.drawFooter(a.uiLayer, group)
		}
		screen.Fill(group.Palette[group.UI.Background])
		cardOptions := &ebiten.DrawImageOptions{}
		cardOptions.Filter = ebiten.FilterNearest
		cardOptions.GeoM.Scale(float64(a.viewScale()), float64(a.viewScale()))
		cardOptions.GeoM.Translate(float64(a.originX), float64(a.originY))
		screen.DrawImage(a.uiLayer, cardOptions)
		return
	}
	if art := a.originalTitle(); a.usesOriginalTitle() && art != nil {
		// The original's title takes the whole frame: no header, no footer,
		// the key hints live in the window's rows and the F1 card.
		if a.showCredits && art.credits[0] != nil {
			a.drawOriginalCredits(a.uiLayer, art)
		} else {
			a.drawOriginalTitle(a.uiLayer, art)
		}
		screen.Fill(color.RGBA{0, 0, 0, 255})
		titleOptions := &ebiten.DrawImageOptions{}
		titleOptions.Filter = ebiten.FilterNearest
		titleOptions.GeoM.Scale(float64(a.viewScale()), float64(a.viewScale()))
		titleOptions.GeoM.Translate(float64(a.originX), float64(a.originY))
		screen.DrawImage(a.uiLayer, titleOptions)
		return
	}
	if isMenuScreen(a.state.Screen) {
		a.drawMenu(a.uiLayer, group)
	} else if a.state.Screen == game.ScreenOutcome {
		a.drawOutcome(a.uiLayer, group)
	} else {
		// 遊戲畫面走原版的框：框佔滿 320×200，所以沒有另外的頁首頁尾條，
		// 畫面名稱在左方框的標題列上，鍵位提示在下方訊息框裡。
		if a.usesSTShipboard() {
			a.drawSTShipboard(a.uiLayer)
		} else if a.usesSTShipboardBackdrop() {
			a.drawSTShipboard(a.uiLayer)
			a.drawSTZoom(a.uiLayer, group, a.lookup(screenTitleKeys[a.state.Screen]))
		} else if a.usesSTRepair() {
			a.drawSTRepair(a.uiLayer, group)
		} else if a.state.Screen == game.ScreenGround && a.state.GroundWorld && !a.usesSTGround() {
			a.drawSurfaceOverview(a.uiLayer, group)
		} else if a.usesSTGround() && a.drawSTGround(a.uiLayer, true) {
			// The city fills the screen, as it does on the original.
		} else if a.usesSTGroundBackdrop() && a.drawSTGround(a.uiLayer, false) {
			// A window over the city: the left box as a window, the rows and
			// the readouts as on the framed screens, the message in a window.
			// No message window under a zoom window: the original shows a
			// shop's answer inside the window and nothing else.
			if a.state.Screen == game.ScreenTrade {
				a.drawSTTrade(a.uiLayer)
			} else if a.state.Screen == game.ScreenUniteller {
				a.drawSTTeller(a.uiLayer)
			} else if a.state.Screen == game.ScreenTubeStation {
				a.drawSTTubeStation(a.uiLayer)
			} else {
				a.drawSTZoom(a.uiLayer, group, a.lookup(screenTitleKeys[a.state.Screen]))
			}
		} else if a.usesSTLayout() {
			// The Atari ST layout: the original's frame with its own readouts
			// and gauges; the scene, the panel rows and the message box are
			// the same code drawn into the original's boxes.
			a.drawSTFrame(a.uiLayer)
			a.drawMap(a.uiLayer, group)
			a.drawScreenPanel(a.uiLayer, group)
			a.drawSTReadouts(a.uiLayer)
			if a.state.Screen == game.ScreenTactical {
				// The original's tactical screen fills the message box with
				// the scanners and the status window.
				a.drawSTTacticalStatus(a.uiLayer)
			} else {
				a.drawMessageStrip(a.uiLayer, group)
			}
		} else {
			a.drawFrame(a.uiLayer, group)
			a.drawMap(a.uiLayer, group)
			a.drawScreenPanel(a.uiLayer, group)
			a.drawGauges(a.uiLayer, group)
			// 每張遊戲畫面都有下方訊息框——它是框的一部分，不是某些畫面才有的條。
			a.drawMessageStrip(a.uiLayer, group)
		}
	}
	// The chrome goes on after the scene, not before it. The shipboard picture
	// is 280x192 and the band between the two bars is only 137 pixels tall, so
	// something has to be covered; the footer has always been drawn last and
	// the header now matches it. Drawn first, the header bar was painted over
	// by the picture and its dark-on-light text vanished into it.
	//
	// The cover-art title screen has no header bar: its top band carries the
	// wordmark instead.
	// 選單畫面（標題、保存庫、結局）仍走頁首頁尾條——它們不是遊戲畫面，沒有原版的框。
	// 遊戲畫面已經在上面畫過框了，框佔滿整個 320×200，再疊頁首頁尾會把它切掉。
	if inGameFrame(a.state.Screen) {
		// 畫面名稱寫在左方框的標題列上，由 drawScreenPanel 負責。
	} else {
		a.drawHeader(a.uiLayer, group)
		a.drawFooter(a.uiLayer, group)
	}

	screen.Fill(group.Palette[group.UI.Background])
	uiOptions := &ebiten.DrawImageOptions{}
	uiOptions.Filter = ebiten.FilterNearest
	uiOptions.GeoM.Scale(float64(a.viewScale()), float64(a.viewScale()))
	uiOptions.GeoM.Translate(float64(a.originX), float64(a.originY))
	screen.DrawImage(a.uiLayer, uiOptions)
	if a.state.Screen == game.ScreenNavigation {
		// The route maps are drawn by the panel, which owns that area.
		return
	}
	if a.usesModernGroundRaster(group) {
		a.drawModernGroundMap(screen, group)
	}
}

// aboardShip names the screens the character is inside the ship on. The
// viewport behind their floating window shows the ship, not the city the ship
// happens to be parked in — the window is small now, so what is behind it is
// visible and has to be the right place.
func aboardShip(screen game.Screen) bool {
	switch screen {
	case game.ScreenShipboard, game.ScreenCargo, game.ScreenEquipment, game.ScreenEngineering:
		return true
	}
	return false
}

// inGameFrame reports whether this screen is drawn inside the original's
// chrome. The menus are not: they are full-frame screens of their own.
func inGameFrame(screen game.Screen) bool {
	return !isMenuScreen(screen) && screen != game.ScreenOutcome
}

// The save library lists all eight slots at once. The original's Library Disk
// held one game in one place; the remake keeps eight, so the screen has to
// show which is which — a number picks one, and the arrows walk the list.
const (
	librarySlotsY    = 62
	librarySlotStep  = 14
	librarySlotRows  = 4
	librarySlotColX  = 28
	librarySlotColX2 = 168
	librarySlotColW  = 132
	libraryKeysY     = 122
)

func (a *app) drawLibrary(screen *ebiten.Image, group theme.Group) {
	ink := group.Palette[group.UI.Foreground]
	accent := group.Palette[group.UI.Accent]
	a.drawTextWithin(screen, a.lookup("ui.character_library"), 28, 44, 132, ink)
	summaries := map[string]storage.SlotSummary{}
	if list, err := storage.ListLibrary(a.libraryPath); err == nil {
		for _, summary := range list {
			summaries[summary.ID] = summary
		}
	} else {
		a.drawTextWithin(screen, a.lookup("ui.library_not_created"), 168, 44, 132, accent)
	}
	for index, slot := range prototypeLibrarySlots {
		x := librarySlotColX
		row := index
		if index >= librarySlotRows {
			x = librarySlotColX2
			row = index - librarySlotRows
		}
		marker := "  "
		colour := ink
		if slot == a.librarySlot {
			marker = "▶ "
			colour = accent
		}
		label := fmt.Sprintf("%s%d %s", marker, index+1, a.librarySlotLine(summaries, slot))
		a.drawTextWithin(screen, label, x, librarySlotsY+row*librarySlotStep, librarySlotColW, colour)
	}
	a.drawTextWithin(screen, a.libraryKeyRow(), 28, libraryKeysY, 274, ink)
}

// libraryKeyRow is the row of keys under the slot list. The names are the
// short ones: five keys and the number range have to fit one row.
func (a *app) libraryKeyRow() string {
	return "N " + a.lookup("ui.library_key_new") +
		"　M " + a.lookup("ui.library_key_make") +
		"　S " + a.lookup("ui.library_key_store") +
		"　L " + a.lookup("ui.library_key_load") +
		"　D " + a.lookup("ui.library_key_delete") +
		"　" + a.lookup("ui.library_key_pick")
}

// librarySlotLine is one slot's line: who is in it and where, or that it is
// empty. The clock is left out — the line has to fit half the panel.
func (a *app) librarySlotLine(summaries map[string]storage.SlotSummary, slot string) string {
	summary, ok := summaries[slot]
	if !ok || summary.Character == "" {
		return a.lookup("ui.library_slot_empty")
	}
	if summary.Location == "" {
		return summary.Character
	}
	return summary.Character + "·" + summary.Location
}

func isMenuScreen(screen game.Screen) bool {
	return screen == game.ScreenTitle || screen == game.ScreenCharacterLibrary || screen == game.ScreenCharacterCreation || screen == game.ScreenAttributeAllocation
}

func (a *app) drawOutcome(screen *ebiten.Image, group theme.Group) {
	uiFillRect(screen, 14, 28, 292, 118, color.RGBA{0, 0, 0, 225}, false)
	uiFillRect(screen, 20, 34, 280, 2, group.Palette[group.UI.Accent], false)
	a.drawWrappedTextWithin(screen, a.lookup("ui.outcome"), 28, 44, 264, 1, group.Palette[group.UI.Foreground])
	a.drawWrappedTextWithin(screen, a.lookup(a.state.OutcomeKey), 28, 68, 264, 2, group.Palette[group.UI.Foreground])
	a.drawWrappedTextWithin(screen, a.messageLine(), 28, 91, 264, 2, group.Palette[group.UI.Foreground])
	instructionKey := "ui.outcome_return"
	if isFailureTerminal(a.state) {
		instructionKey = "ui.outcome_failure_terminal"
	}
	a.drawWrappedTextWithin(screen, a.lookup(instructionKey), 28, 119, 264, 2, group.Palette[group.UI.Accent])
}

func (a *app) drawMenu(screen *ebiten.Image, group theme.Group) {
	uiFillRect(screen, 12, 28, 296, 122, color.RGBA{0, 0, 0, 150}, false)
	uiFillRect(screen, 17, 33, 286, 2, group.Palette[group.UI.Accent], false)
	if a.state.Screen == game.ScreenTitle {
		a.drawTextWithin(screen, a.lookup("ui.title"), 28, 48, 270, group.Palette[group.UI.Foreground])
		a.drawTextWithin(screen, a.lookup("ui.play_sundog"), 28, 72, 270, group.Palette[group.UI.Foreground])
		a.drawTextWithin(screen, a.lookup("ui.select_game"), 28, 88, 270, group.Palette[group.UI.Foreground])
		a.drawTextWithin(screen, a.lookup("ui.load_game"), 28, 104, 270, group.Palette[group.UI.Foreground])
		a.drawTextWithin(screen, a.lookup("ui.options"), 28, 120, 270, group.Palette[group.UI.Foreground])
	} else if a.state.Screen == game.ScreenCharacterLibrary {
		a.drawLibrary(screen, group)
	} else if a.state.Screen == game.ScreenCharacterCreation {
		a.drawTextWithin(screen, a.lookup("ui.character_creation"), 28, 48, 270, group.Palette[group.UI.Foreground])
		a.drawTextWithin(screen, a.lookup("ui.character_name_prompt"), 28, 72, 270, group.Palette[group.UI.Foreground])
		draft := a.state.CharacterDraft
		if draft == "" {
			draft = "＿"
		} else {
			draft += "＿"
		}
		a.drawTextWithin(screen, a.lookup("ui.character_name")+"："+draft, 28, 94, 270, group.Palette[group.UI.Foreground])
		a.drawTextWithin(screen, a.lookup("ui.character_creation_note"), 28, 116, 270, group.Palette[group.UI.Foreground])
	} else {
		a.drawTextWithin(screen, a.lookup("ui.attribute_allocation"), 28, 42, 270, group.Palette[group.UI.Foreground])
		for index, label := range game.AttributeLabels {
			marker := "  "
			if index == a.state.AttributeSelected {
				marker = "▶ "
			}
			line := fmt.Sprintf("%s%s  %d", marker, label, a.state.Attributes[index])
			a.drawTextWithin(screen, line, 38, 59+index*13, 120, group.Palette[group.UI.Foreground])
		}
		points := fmt.Sprintf("%s：%d", a.lookup("ui.attribute_points_remaining"), a.state.AttributePoints)
		a.drawTextWithin(screen, points, 168, 68, 126, group.Palette[group.UI.Foreground])
		a.drawWrappedTextWithin(screen, a.lookup("ui.attribute_allocation_note"), 168, 88, 126, 3, group.Palette[group.UI.Foreground])
	}
	a.drawTextWithin(screen, a.messageLine(), 28, menuMessageY, 270, group.Palette[group.UI.Foreground])
}

// drawHelp is the F1 card: every key the game listens to, on one screen, in the
// interface colours the player picked. It goes over the whole frame because it
// is a reading mode — Update stops feeding the game while it is up.
func (a *app) drawHelp(screen *ebiten.Image, group theme.Group) {
	uiFillRect(screen, 8, 22, 304, 134, color.RGBA{0, 0, 0, 235}, false)
	uiFillRect(screen, 12, 26, 296, 2, group.Palette[group.UI.Accent], false)
	a.drawTextWithin(screen, a.lookup("ui.help.title"), 14, 32, 292, group.Palette[group.UI.Accent])
	rows := []string{
		a.lookup("ui.help.mouse"),
		a.lookup("ui.help.escape"),
		a.lookup("ui.help.arrows"),
		a.lookup("ui.help.confirm"),
		a.lookup("ui.help.save_load"),
		a.lookup("ui.help.tactical"),
		a.lookup("ui.help.cards"),
		a.lookup("ui.help.title_keys"),
		a.lookup("ui.help.sound"),
		a.lookup("ui.help.presentation"),
		a.lookup("ui.help.presentation_persists"),
		a.lookup("ui.help.quit"),
	}
	if a.state.Screen == game.ScreenTactical {
		rows[0] = a.lookup("ui.help.tactical_aim")
		rows[2] = a.lookup("ui.help.tactical_fire")
		rows[3] = a.lookup("ui.help.tactical_shields")
	}
	if a.state.Screen == game.ScreenUniteller {
		rows[1] = a.lookup("ui.help.teller_commit")
	}
	if a.state.Screen == game.ScreenGround {
		rows[4] = a.lookup("ui.help.ground_save")
		rows[5] = a.lookup("ui.help.ground_parts")
	}
	if a.state.Screen == game.ScreenEngineering {
		rows[4] = a.lookup("ui.help.library_save")
		rows[5] = a.lookup("ui.help.engineering_shortcuts")
	}
	for index, row := range rows {
		if row == "" {
			continue
		}
		a.drawTextWithin(screen, row, 14, helpRowY0+index*helpRowStep, 292, group.Palette[group.UI.Foreground])
	}
	a.drawTextWithin(screen, a.lookup("ui.help.close"), 14, 146, 292, group.Palette[group.UI.Accent])
}

func (a *app) drawHeader(screen *ebiten.Image, group theme.Group) {
	uiFillRect(screen, 0, 0, logicalWidth, 20, group.Palette[group.UI.Foreground], false)
	uiFillRect(screen, 0, 18, logicalWidth, 2, group.Palette[group.UI.Accent], false)
	a.drawTextWithin(screen, a.lookup("ui.title"), 6, 3, 196, group.Palette[group.UI.Background])
	a.drawTextWithin(screen, screenLabel(a.labels, a.state.Screen), 210, 4, 104, group.Palette[group.UI.Background])
}

func (a *app) drawMap(screen *ebiten.Image, group theme.Group) {
	if aboardShip(a.state.Screen) {
		a.drawSpaceBackdrop(screen, group)
		return
	}
	if a.state.Screen == game.ScreenFlight {
		if a.usesSTLayout() {
			// In flight the original's viewport is the starfield with the
			// ship's mark; the plan is text in the message box.
			a.drawSpaceBackdrop(screen, group)
			a.drawSTViewportCross(screen)
			return
		}
		a.drawFlightPlot(screen, group)
		return
	}
	if a.state.Screen == game.ScreenTactical {
		if a.usesSTLayout() {
			// The original keeps the starfield up and marks the ship's own
			// position; the enemy is in the status window below.
			a.drawSpaceBackdrop(screen, group)
			a.drawSTViewportCross(screen)
			return
		}
		a.drawEnemyOutline(screen, group)
		return
	}
	if a.usesModernGroundRaster(group) {
		// The official 128px/cell atlas is composited into the physical output
		// after this compatibility UI layer has been scaled. Rendering it here
		// would downsample the art back to the old 16px prototype contract.
		return
	}
	originX, originY := world.ViewportOrigin(a.state.Position.X, a.state.Position.Y)
	if a.originalDisk != nil && a.drawOriginalGround(screen, group, originX, originY) {
		return
	}
	for y := 0; y < world.ViewportHeight(); y++ {
		for x := 0; x < world.ViewportWidth(); x++ {
			terrain, ok := game.GroundTerrainAt(a.state, a.state.CityID, originX+x, originY+y)
			if !ok {
				continue
			}
			tileID := string(terrain)
			pattern, ok := group.Patterns[tileID]
			if !ok {
				continue
			}
			a.drawPattern(screen, sceneX+x*tilePixels, sceneY+y*tilePixels, pattern, group.TilePalette)
		}
	}
	playerX, playerY := a.state.Position.X-originX, a.state.Position.Y-originY
	if playerX >= 0 && playerX < world.ViewportWidth() && playerY >= 0 && playerY < world.ViewportHeight() {
		if pattern, ok := group.Patterns[theme.Player]; ok {
			a.drawPattern(screen, sceneX+playerX*tilePixels, sceneY+playerY*tilePixels, pattern, group.TilePalette)
		}
	}
}

// drawOriginalGround draws the city floor from the player's own disk: the four
// bit code of each cell, through the original's own translation table, into one
// of the three tile strips the city's map header names.
//
// It reports whether it drew. A city the original itself cannot draw — the two
// placeholder records name strips shorter than the codes they use — falls back
// to the remake's tiles rather than showing holes.
func (a *app) drawOriginalGround(screen *ebiten.Image, group theme.Group, originX, originY int) bool {
	_, cityNumber, ok := game.GroundTerrainCode(a.state, a.state.CityID, originX, originY)
	if !ok {
		return false
	}
	cache := a.originalTiles[cityNumber]
	if cache == nil {
		set, err := a.originalDisk.LoadCityTiles(cityNumber)
		if err != nil {
			log.Printf("城市 %d 的原版圖塊讀不出來，改用重製版：%v", cityNumber, err)
			a.originalTiles[cityNumber] = &originalTileCache{}
			return false
		}
		cache = &originalTileCache{set: set, images: map[int]*ebiten.Image{}}
		a.originalTiles[cityNumber] = cache
	}
	if cache.set == nil {
		return false
	}

	drawn := 0
	for y := 0; y < world.ViewportHeight(); y++ {
		for x := 0; x < world.ViewportWidth(); x++ {
			cellX, cellY := originX+x, originY+y
			code, _, ok := game.GroundTerrainCode(a.state, a.state.CityID, cellX, cellY)
			if !ok {
				continue
			}
			// The original rolls a die for which of a terrain's two looks to
			// draw. A canvas redrawn every frame cannot roll, so the cell's own
			// position decides, and the same cell always looks the same.
			roll := (cellX*7 + cellY*13) & 3
			tile := cache.tile(code, roll)
			if tile == nil {
				continue
			}
			options := &ebiten.DrawImageOptions{}
			options.Filter = ebiten.FilterNearest
			options.GeoM.Scale(0.5, 0.5)
			options.GeoM.Translate(float64(sceneX+x*tilePixels), float64(sceneY+y*tilePixels))
			screen.DrawImage(tile, options)
			drawn++
		}
	}
	if drawn == 0 {
		return false
	}
	if column, row, ok := a.groundShipCell(originX, originY, world.ViewportWidth(), world.ViewportHeight()); ok {
		a.drawGroundShip(screen, sceneX+column*tilePixels, sceneY+row*tilePixels, tilePixels)
	}
	playerX, playerY := a.state.Position.X-originX, a.state.Position.Y-originY
	if playerX >= 0 && playerX < world.ViewportWidth() && playerY >= 0 && playerY < world.ViewportHeight() {
		if !a.drawGroundWalker(screen, sceneX+playerX*tilePixels, sceneY+playerY*tilePixels) {
			if pattern, ok := group.Patterns[theme.Player]; ok {
				a.drawPattern(screen, sceneX+playerX*tilePixels, sceneY+playerY*tilePixels, pattern, group.TilePalette)
			}
		}
	}
	return true
}

// walkerPattern picks the original's sprite number for the player: the
// people family, 6 walking vertically and 8 horizontally (XMOVEONG births
// the walker as pattern 6; the direction table is the sprite handler's:
// +0 vertical, +2 horizontal, +1/+3 the diagonals). The 0–3 family is the
// other set of shapes the same handler steers; it is not the walker.
func walkerPattern(heading game.Heading, _ game.Position) int {
	if heading == game.HeadingEast || heading == game.HeadingWest {
		return stPersonPatterns + 2
	}
	return stPersonPatterns
}

// drawGroundWalker draws the player with the original's own 7x7 walker sprite
// (docs/atari-st/ground-rules.json, spritePatterns): pattern 0 walking
// vertically, 2 horizontally, and the same shape six entries later as the
// second stride frame, alternated here by the cell parity the tile dice
// already use. It reports whether it drew; without a readable disk the theme's
// own player pattern stays.
func (a *app) drawGroundWalker(screen *ebiten.Image, x, y int) bool {
	if !a.ensureGroundSprites() {
		return false
	}
	ink := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	a.drawSTSprite(screen, walkerPattern(a.state.Heading, a.state.Position), x, y, ink)
	return true
}

// --- put_ship (XMOVEONG:0x07): the SunDog parked on the city floor ---
//
// draw_map and redraw_map call wmap and then put_ship, so the ship goes on top
// of the finished city. put_ship draws two font-strip characters six pixels
// apart with draw_char, which centres each glyph on its anchor; together
// characters 11 and 12 are the twelve-by-eight top view of the ship. The
// original draws it only when the ship's own city (MAINLIB_G4a) is the one
// being walked and its cell is inside the eight-by-five view, and it offsets
// the pair inside the cell by the ship's sub-cell pixel position (G4d, G4e).
//
// The remake keeps the ship's city and cell in the save (State.ShipCityID and
// ShipPosition, schema 27) and applies the same two guards. What it does not
// keep is the sub-cell pixel offset — a grid has nowhere to put one — so the
// pair is centred in the cell. That difference is listed in WORKLIST B.
const (
	stCharShipGroundNose = 11
	stCharShipGroundTail = 12
	stGroundShipGap      = 6
	stGroundShipWidth    = 12
	stGroundShipHeight   = 8
)

// groundShipCell is the cell the ship is parked on and whether it is inside a
// view whose top-left cell is (originX, originY) and which is columns by rows.
func (a *app) groundShipCell(originX, originY, columns, rows int) (int, int, bool) {
	if a.state.GroundWorld || a.state.ShipCityID == "" || a.state.ShipCityID != a.state.CityID {
		// put_ship's first guard: the ship is parked in another city.
		return 0, 0, false
	}
	column, row := a.state.ShipPosition.X-originX, a.state.ShipPosition.Y-originY
	column, row = (column%40+40)%40, (row%24+24)%24
	if column < 0 || column >= columns || row < 0 || row >= rows {
		return 0, 0, false
	}
	return column, row, true
}

// drawGroundShip draws the pair centred in a cell of the given size whose
// top-left pixel is (left, top). drawSTChar anchors a glyph at (x-3, y-4), so
// the pair covers x-3..x+8 and y-4..y+3; the anchor is placed to put that box
// in the middle of the cell. Without the disk's font strip there is no ship to
// draw, the same rule drawGroundWalker follows.
func (a *app) drawGroundShip(screen *ebiten.Image, left, top, cell int) bool {
	if a.stFont() == nil {
		return false
	}
	x := left + (cell-stGroundShipWidth)/2 + 3
	y := top + (cell-stGroundShipHeight)/2 + 4
	white := stPen(1)
	a.drawSTChar(screen, stCharShipGroundNose, x, y, white)
	a.drawSTChar(screen, stCharShipGroundTail, x+stGroundShipGap, y, white)
	return true
}

// ensureGroundSprites reads the disk's sprite patterns once and reports whether
// they are there. The walker on the ground and the one in the corridors share
// them, so neither owns the read.
func (a *app) ensureGroundSprites() bool {
	if !a.groundSpritesRead {
		a.groundSpritesRead = true
		if a.originalDisk == nil {
			return false
		}
		patterns, err := a.originalDisk.GroundSprites()
		if err != nil {
			log.Printf("原版地面 sprite 讀不出來，改用重製版的：%v", err)
			return false
		}
		a.groundSprites = &patterns
	}
	return a.groundSprites != nil
}

// drawSTSprite draws one of the disk's sprite patterns doubled, the way the
// ground walker is drawn; the room's people (XMOVEINB birth_sprite, patterns
// 6–9) use the same table. groundSprites must already be read.
func (a *app) drawSTSprite(screen *ebiten.Image, pattern, x, y int, ink color.RGBA) {
	a.drawSTSpriteAt(screen, pattern, x, y, 2, ink)
}

// drawSTSpriteAt is the same at a chosen scale. The ground draws the walker
// doubled so it reads in a 40-pixel cell; the corridors draw it 1:1, which is
// the 7×7 the live run measures on the original's own screen.
func (a *app) drawSTSpriteAt(screen *ebiten.Image, pattern, x, y, scale int, ink color.RGBA) {
	if a.usesHD() {
		a.drawHDPerson(screen, pattern, x, y, scale, ink)
		return
	}

	if a.groundSprites == nil || pattern < 0 || pattern >= len(a.groundSprites) {
		return
	}
	rows := a.groundSprites[pattern]
	for row, bits := range rows {
		for column := 0; column < 7; column++ {
			if bits&(1<<(6-column)) == 0 {
				continue
			}
			uiFillRect(screen, float32(x+column*scale), float32(y+row*scale),
				float32(scale), float32(scale), ink, false)
		}
	}
}

// tile decodes one tile once and keeps it.
func (c *originalTileCache) tile(code, roll int) *ebiten.Image {
	key := code<<2 | roll
	if image, ok := c.images[key]; ok {
		return image
	}
	source, drawn := c.set.Tile(code, roll)
	if !drawn {
		c.images[key] = nil
		return nil
	}
	image := ebiten.NewImageFromImage(source)
	c.images[key] = image
	return image
}

// The route maps share the panel the ground map uses. The original draws them
// at its own scale in its own frame; what these functions own is the fit onto
// this canvas, not where anything is relative to anything else.
const (
	routeMapX      = panelX + 4
	routeMapY      = 50
	routeMapWidth  = panelW - 8
	routeMapHeight = 48
	// The selector rows sit under the map, closer together than the ten pixels
	// the other screens use, and stop above the message strip at y = 137.
	routeRowY    = 99
	routeRowStep = 9
)

// drawNavigationMap draws whichever of the original's three route maps the
// player is working in. The maps are not decoration: every position on them
// comes from the source tables, so moving the selector moves the marker.
func (a *app) drawNavigationMap(screen *ebiten.Image, group theme.Group) {
	uiFillRect(screen, routeMapX, routeMapY, routeMapWidth, routeMapHeight,
		group.Palette[group.UI.Background], false)
	switch a.state.NavigationView {
	case game.NavigationSystem:
		a.drawSystemMap(screen, group)
	case game.NavigationPlanet:
		a.drawPlanetMap(screen, group)
	default:
		a.drawSectorChart(screen, group)
	}
}

// drawSectorChart is the three-dimensional star chart. The original places a
// system at (field0 x 12, field1 x 10) and draws a circle whose radius is
// 6 - field2 / 2, so a system further along the third axis is a smaller circle.
// The placement is kept and only the step is rescaled to this panel.
func (a *app) drawSectorChart(screen *ebiten.Image, group theme.Group) {
	foreground := group.Palette[group.UI.Foreground]
	accent := group.Palette[group.UI.Accent]
	panel := group.Palette[group.UI.Panel]

	target, hasTarget := a.reducer.World.Destination(a.state.NavigationTargetID)
	for _, system := range a.reducer.World.Systems() {
		// This game's chart, not the factory one: the original re-rolls the
		// coordinates at new-game (internal/game/sector.go).
		coordinates, _ := a.reducer.SystemCoordinate(a.state, system.ID)
		x := float32(routeMapX + 4 + coordinates[0]*7)
		y := float32(routeMapY + 2 + coordinates[1]*3)
		radius := float32(6-coordinates[2]/2) / 2
		if radius < 1 {
			radius = 1
		}
		colour := panel
		switch {
		case hasTarget && system.ID == target.SystemID:
			colour = accent
		case system.ID == a.state.SystemID:
			colour = foreground
		}
		uiStrokeCircle(screen, x, y, radius, 1, colour, false)
		if system.ID == a.state.SystemID {
			// The original boxes the system the ship is in. A cross reads the
			// same at this size and does not swallow the circle.
			uiStrokeLine(screen, x-3, y, x+3, y, 1, foreground, false)
			uiStrokeLine(screen, x, y-3, x, y+3, 1, foreground, false)
		}
	}
}

// drawSystemMap is the star system: the star at the centre, each planet on its
// own orbit. Both the orbit radius and the angle are the planet record's own.
func (a *app) drawSystemMap(screen *ebiten.Image, group theme.Group) {
	foreground := group.Palette[group.UI.Foreground]
	accent := group.Palette[group.UI.Accent]
	panel := group.Palette[group.UI.Panel]

	centreX := float32(routeMapX + routeMapWidth/2)
	centreY := float32(routeMapY + routeMapHeight/2)
	uiFillRect(screen, centreX-2, centreY-2, 5, 5, accent, false)

	target, hasTarget := a.reducer.World.Destination(a.state.NavigationTargetID)
	systemID := a.state.SystemID
	if hasTarget {
		systemID = target.SystemID
	}
	for _, planet := range a.reducer.World.PlanetsInSystem(systemID) {
		// The orbit is a circle of the planet's own radius; the planet sits on
		// it at its own angle.
		orbit := a.reducer.PlanetOrbit(a.state, planet)
		scaled := float32(orbit.Radius) * 3 / 4
		uiStrokeCircle(screen, centreX, centreY, scaled, 1, panel, false)
		x, y := game.PolarToRect(orbit.Angle, orbit.Radius)
		planetX := centreX + float32(x)*3/4
		planetY := centreY + float32(y)*3/4
		colour := foreground
		if hasTarget && planet.ID == target.PlanetID {
			colour = accent
		}
		uiFillRect(screen, planetX-1, planetY-1, 3, 3, colour, false)
	}
}

// remakePlanetMapPoint fits change_xy's full 40×24 domain into the map panel.
// Four pixels around each centre keep the city square and player cross inside.
func remakePlanetMapPoint(destination world.Destination) (float32, float32) {
	const inset = 4
	return float32(routeMapX + inset + destination.MapX*(routeMapWidth-1-2*inset)/147),
		float32(routeMapY + inset + destination.MapY*(routeMapHeight-1-2*inset)/94)
}

// drawPlanetMap is the planet's surface: one square per city, at the position
// the city record carries. The original draws character 20 there, and puts the
// ship marker on the city the player is standing in.
func (a *app) drawPlanetMap(screen *ebiten.Image, group theme.Group) {
	foreground := group.Palette[group.UI.Foreground]
	accent := group.Palette[group.UI.Accent]

	target, hasTarget := a.reducer.World.Destination(a.state.NavigationTargetID)
	planetID := a.state.PlanetID
	if hasTarget {
		planetID = target.PlanetID
	}
	for _, destination := range a.reducer.DestinationsOnPlanet(a.state, planetID) {
		x, y := remakePlanetMapPoint(destination)
		colour := foreground
		if hasTarget && destination.CityID == target.CityID {
			colour = accent
		}
		uiFillRect(screen, x-2, y-2, 5, 5, colour, false)
		if destination.CityID == a.state.CityID {
			uiStrokeLine(screen, x-4, y, x+4, y, 1, accent, false)
			uiStrokeLine(screen, x, y-4, x, y+4, 1, accent, false)
		}
	}
}

// drawFlightPlot is the in-system view: the star at the centre, the planets of
// the system on their own orbits, and the ship where the interpolation has it.
// Every position comes from the source projection; what this function owns is
// the scale, because the remake's canvas is not the original's frame.
func (a *app) drawFlightPlot(screen *ebiten.Image, group theme.Group) {
	const (
		plotCenterX = frameViewportX + frameViewportW/2
		plotCenterY = frameViewportY + frameViewportH/2
		plotScale   = 2
		plotRight   = frameViewportX + frameViewportW - 4
	)
	foreground := group.Palette[group.UI.Foreground]
	accent := group.Palette[group.UI.Accent]

	place := func(angle, radius int) (float32, float32) {
		x, y := game.PolarToRect(angle, radius)
		return float32(plotCenterX + x*plotScale), float32(plotCenterY + y*plotScale)
	}

	// The star. Everything else is measured from it, so it is drawn first.
	uiFillRect(screen, plotCenterX-2, plotCenterY-2, 5, 5, accent, false)

	for _, planet := range a.reducer.World.PlanetsInSystem(a.state.SystemID) {
		orbit := a.reducer.PlanetOrbit(a.state, planet)
		x, y := place(orbit.Angle, orbit.Radius)
		uiFillRect(screen, x-2, y-2, 4, 4, foreground, false)
		// The label goes below the planet and is bounded by what is left of the
		// canvas, so an orbit near the edge cannot push its name off the screen.
		labelX := int(x) - 12
		if labelX < 142 {
			labelX = 142
		}
		a.drawTextWithin(screen, localization.Planet(a.labels, planet.ID, planet.Name),
			labelX, int(y)+11, plotRight-labelX, foreground)
	}

	angle, radius := game.FlightPosition(a.state)
	shipX, shipY := place(angle, radius)
	uiStrokeLine(screen, shipX-3, shipY, shipX+3, shipY, 1, accent, false)
	uiStrokeLine(screen, shipX, shipY-3, shipX, shipY+3, 1, accent, false)
}

// drawEnemyOutline draws the enemy the player is fighting. The points are the
// original's own polyline, in its own byte coordinates; what this function owns
// is where on the canvas the shape is centred, because the original draws it
// relative to a window the remake does not have.
func (a *app) drawEnemyOutline(screen *ebiten.Image, group theme.Group) {
	if a.usesSTLayout() {
		return
	}
	points := game.EnemyShipOutline(a.state)
	if len(points) < 2 {
		return
	}
	const (
		sceneCenterX = frameViewportX + frameViewportW/2
		sceneCenterY = frameViewportY + frameViewportH/2
	)
	minX, minY := points[0][0], points[0][1]
	maxX, maxY := minX, minY
	for _, point := range points[1:] {
		minX = min(minX, point[0])
		maxX = max(maxX, point[0])
		minY = min(minY, point[1])
		maxY = max(maxY, point[1])
	}
	offsetX := float32(sceneCenterX - (minX+maxX)/2)
	offsetY := float32(sceneCenterY - (minY+maxY)/2)

	colour := group.Palette[group.UI.Foreground]
	if !a.state.CombatActive {
		colour = group.Palette[group.UI.Accent]
	}
	for index := 0; index < len(points)-1; index++ {
		uiStrokeLine(screen,
			float32(points[index][0])+offsetX, float32(points[index][1])+offsetY,
			float32(points[index+1][0])+offsetX, float32(points[index+1][1])+offsetY,
			1, colour, false)
	}
}

func (a *app) drawModernGroundMap(screen *ebiten.Image, group theme.Group) {
	if group.Raster == nil {
		return
	}
	atlas, err := a.modernRasterImage(group.Raster)
	if err != nil {
		log.Printf("decode Modern Apple raster atlas: %v", err)
		return
	}
	scale := a.viewScale()
	tileSize := tilePixels * scale
	mapX, mapY := modernGroundOriginX*scale+a.originX, modernGroundOriginY*scale+a.originY
	originX, originY := world.ViewportOrigin(a.state.Position.X, a.state.Position.Y)
	for y := 0; y < world.ViewportHeight(); y++ {
		for x := 0; x < world.ViewportWidth(); x++ {
			terrain, ok := game.GroundTerrainAt(a.state, a.state.CityID, originX+x, originY+y)
			if !ok {
				continue
			}
			a.drawModernRasterTile(screen, atlas, group.Raster, modernGroundBaseTile(terrain), mapX+x*tileSize, mapY+y*tileSize, tileSize)
		}
	}
	for y := 0; y < world.ViewportHeight(); y++ {
		for x := 0; x < world.ViewportWidth(); x++ {
			terrain, ok := game.GroundTerrainAt(a.state, a.state.CityID, originX+x, originY+y)
			if !ok {
				continue
			}
			if sprite, ok := modernGroundSprite(terrain); ok {
				a.drawModernRasterTile(screen, atlas, group.Raster, sprite, mapX+x*tileSize, mapY+y*tileSize, tileSize)
			}
		}
	}
	playerX, playerY := a.state.Position.X-originX, a.state.Position.Y-originY
	if playerX >= 0 && playerX < world.ViewportWidth() && playerY >= 0 && playerY < world.ViewportHeight() {
		a.drawModernRasterTile(screen, atlas, group.Raster, theme.Player, mapX+playerX*tileSize, mapY+playerY*tileSize, tileSize)
	}
	uiStrokeRect(screen, float32(mapX-2), float32(mapY-2), float32(world.ViewportWidth()*tileSize+4), float32(world.ViewportHeight()*tileSize+4), 2, group.Palette[group.UI.Accent], false)
}

func (a *app) modernRasterImage(raster *theme.RasterAtlas) (*ebiten.Image, error) {
	if raster == nil {
		return nil, fmt.Errorf("Modern Apple raster atlas is missing")
	}
	if a.modernAtlas != nil && a.modernAtlasHash == raster.AtlasSHA256 {
		return a.modernAtlas, nil
	}
	decoded, err := png.Decode(bytes.NewReader(raster.PNG()))
	if err != nil {
		return nil, err
	}
	a.modernAtlas = ebiten.NewImageFromImage(decoded)
	a.modernAtlasHash = raster.AtlasSHA256
	return a.modernAtlas, nil
}

func (a *app) drawModernRasterTile(screen, atlas *ebiten.Image, raster *theme.RasterAtlas, semantic string, x, y, destinationSize int) {
	tile, ok := raster.Tile(semantic)
	if !ok || tile.Width <= 0 || tile.Height <= 0 {
		return
	}
	source := atlas.SubImage(image.Rect(tile.X, tile.Y, tile.X+tile.Width, tile.Y+tile.Height)).(*ebiten.Image)
	options := &ebiten.DrawImageOptions{}
	options.Filter = ebiten.FilterLinear
	options.GeoM.Scale(float64(destinationSize)/float64(tile.Width), float64(destinationSize)/float64(tile.Height))
	options.GeoM.Translate(float64(x), float64(y))
	screen.DrawImage(source, options)
}

// modernGroundBaseTile picks a visual substrate for a semantic object cell.
// It is presentation-only: world.Terrain and IsPassable remain the reducer's
// unchanged typed contract.
func modernGroundBaseTile(terrain world.Terrain) string {
	switch terrain {
	case world.TerrainWater:
		return theme.Water
	case world.TerrainRoad:
		return theme.Road
	case world.TerrainWall:
		return theme.Wall
	case world.TerrainCity, world.TerrainShip, world.TerrainCargo:
		return theme.Road
	default:
		return theme.Grass
	}
}

func modernGroundSprite(terrain world.Terrain) (string, bool) {
	switch terrain {
	case world.TerrainCity:
		return theme.City, true
	case world.TerrainShip:
		return theme.Ship, true
	case world.TerrainCargo:
		return theme.Cargo, true
	default:
		return "", false
	}
}

// shipInteriorPicture is the original's corridor picture (image 8, raw
// 0x126ca), the scene move_on_ship walks through.
const shipInteriorPicture = 8

// shipInteriorImages decodes the corridor once into two palette variants: the
// original's running lights are a palette animation (rule lights.running) —
// palette index 5 flips between red 0x700 and black while the picture itself
// is never redrawn. The remake bakes the two states instead.
func (a *app) shipInteriorImages() (off, on *ebiten.Image, ok bool) {
	if !a.shipInteriorRead {
		a.shipInteriorRead = true
		if a.originalDisk == nil {
			return nil, nil, false
		}
		picture, err := a.originalDisk.DecodeImage(shipInteriorPicture)
		if err != nil {
			log.Printf("原版艙內圖讀不出來，改用太空背景：%v", err)
			return nil, nil, false
		}
		// The walker collides against this picture's palette indices, so keep
		// them: XMOVEONS ship_disp hands set_coll_color the string
		// "1100000000000001", which makes colours 0, 1 and 15 the walls
		// (docs/atari-st/shipboard-walker.md).
		a.cabinFloor = make([]bool, picture.Width*picture.Height)
		a.cabinWidth, a.cabinHeight = picture.Width, picture.Height
		for i, pixel := range picture.Pixels {
			if i >= len(a.cabinFloor) {
				break
			}
			a.cabinFloor[i] = stCabinPassable[pixel&0x0f]
		}
		for variant := 0; variant < 2; variant++ {
			colours := originalmedia.PaletteRGBA()
			colours[5] = color.RGBA{A: 255}
			if variant == 1 {
				colours[5] = color.RGBA{R: 255, A: 255}
			}
			decoded := image.NewRGBA(image.Rect(0, 0, picture.Width, picture.Height))
			for y := 0; y < picture.Height; y++ {
				for x := 0; x < picture.Width; x++ {
					decoded.SetRGBA(x, y, colours[picture.Pixels[y*picture.Width+x]&0x0f])
				}
			}
			a.shipInterior[variant] = ebiten.NewImageFromImage(decoded)
		}
	}
	if a.shipInterior[0] == nil {
		return nil, nil, false
	}
	return a.shipInterior[0], a.shipInterior[1], true
}

// drawShipInterior puts the original's corridor across the viewport, halved
// like the space scene, with its two light animations: the palette blink
// (lights.running: red one beat in five) and the outward bay sweep
// (lights.whoosh-bars: bars at 0x84−6t and 0xda+6t, y 0x7e–0x80, t 0–9).
// The beat length in frames is the remake's; the shapes are the original's.
func (a *app) drawShipInterior(screen *ebiten.Image, off, on *ebiten.Image) {
	const beatFrames = 8
	beat := int(a.frames) / beatFrames
	backdrop := off
	if beat%5 == 1 {
		backdrop = on
	}
	options := &ebiten.DrawImageOptions{}
	options.Filter = ebiten.FilterNearest
	options.GeoM.Scale(0.5, 0.5)
	originX := float64(frameViewportX) + (frameViewportW-160)/2
	originY := float64(frameViewportY) + (frameViewportH-100)/2
	options.GeoM.Translate(originX, originY)
	screen.DrawImage(backdrop, options)
	sweep := beat % 10
	ink := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	left := float32(originX) + float32(0x84-6*sweep)/2
	right := float32(originX) + float32(0xda+6*sweep)/2
	top := float32(originY) + 0x7e/2
	if left >= float32(originX) {
		uiFillRect(screen, left, top, 1, 2, ink, false)
	}
	if right <= float32(originX)+159 {
		uiFillRect(screen, right, top, 1, 2, ink, false)
	}
}

func (a *app) drawSpaceBackdrop(screen *ebiten.Image, group theme.Group) {
	if a.usesHD() {
		a.drawHDStars(screen)
		return
	}

	if a.state.Screen == game.ScreenShipboard {
		if off, on, ok := a.shipInteriorImages(); ok {
			a.drawShipInterior(screen, off, on)
			return
		}
	}
	starsOnly := a.usesSTLayout() && a.state.Screen == game.ScreenTactical
	key := fmt.Sprintf("%s:%t:%d:%d:%d:%d:%d:%d:%d", group.ID, starsOnly, a.state.Position.X, a.state.Position.Y,
		a.state.TimeMinutes, a.state.Fuel, a.state.WeaponCharge, a.state.ShieldLevel, a.state.HullCondition)
	if a.spaceImage == nil || a.spaceKey != key {
		renderScene := render.RenderSpaceScene
		if starsOnly {
			renderScene = render.RenderSpaceStarfield
		}
		imageData, err := renderScene(a.state, group)
		if err != nil {
			log.Printf("render Apple II-inspired space scene: %v", err)
			return
		}
		a.spaceImage = ebiten.NewImageFromImage(imageData)
		a.spaceKey = key
	}
	if a.spaceImage == nil {
		return
	}
	options := &ebiten.DrawImageOptions{}
	options.Filter = ebiten.FilterNearest
	// 這張圖是 280×192，中央視埠只有 165×116，所以縮一半（140×96）再置中。
	// 一半是整數比例，最近鄰縮下去是每隔一列取一個像素，不會磨出灰邊。
	const spaceScale = 0.5
	width := render.SpaceSceneWidth * spaceScale
	height := render.SpaceSceneHeight * spaceScale
	options.GeoM.Scale(spaceScale, spaceScale)
	originX := float64(frameViewportX) + (frameViewportW-width)/2
	originY := float64(frameViewportY) + (frameViewportH-height)/2
	if a.state.Screen == game.ScreenTactical && (a.starScrollX != 0 || a.starScrollY != 0) {
		// The slid starfield: the picture wraps inside its own rectangle.
		w, h := int(width), int(height)
		dx, dy := ((a.starScrollX%w)+w)%w, ((a.starScrollY%h)+h)%h
		clip := screen.SubImage(image.Rect(int(originX), int(originY), int(originX)+w, int(originY)+h)).(*ebiten.Image)
		for _, off := range [][2]int{{dx - w, dy - h}, {dx, dy - h}, {dx - w, dy}, {dx, dy}} {
			tiled := &ebiten.DrawImageOptions{}
			tiled.Filter = ebiten.FilterNearest
			tiled.GeoM.Scale(spaceScale, spaceScale)
			tiled.GeoM.Translate(originX+float64(off[0]), originY+float64(off[1]))
			clip.DrawImage(a.spaceImage, tiled)
		}
		return
	}
	options.GeoM.Translate(originX, originY)
	screen.DrawImage(a.spaceImage, options)
}

// screenTitleKeys names each game screen the way its panel title does.
var screenTitleKeys = map[game.Screen]string{
	game.ScreenStores:          "ui.ship_stores",
	game.ScreenCharacterStatus: "ui.character_status",
	game.ScreenGround:          "ui.ground",
	game.ScreenFuelPurchase:    "ui.fuel_purchase",
	game.ScreenDepot:           "ui.depot_title",
	game.ScreenFTLBoost:        "ui.ftl_title",
	game.ScreenTrade:           "ui.trade_exchange",
	game.ScreenParts:           "ui.parts_counter",
	game.ScreenNavigation:      "ui.navigation",
	game.ScreenFlight:          "ui.flight",
	game.ScreenUniteller:       "ui.uniteller",
	game.ScreenTubeStation:     "ui.tube.title",
	game.ScreenTactical:        "ui.tactical",
	game.ScreenEngineering:     "ui.engineering",
	game.ScreenEquipment:       "ui.equipment",
	game.ScreenCargo:           "ui.cargo_hold",
	game.ScreenDialog:          "ui.dialog",
	game.ScreenInterior:        "ui.interior",
	game.ScreenSlots:           "ui.slots.title",
	game.ScreenArena:           "ui.arena.title",
	game.ScreenScore:           "ui.score.title",
}

func (a *app) drawScreenPanel(screen *ebiten.Image, group theme.Group) {
	if a.state.Screen == game.ScreenShipboard {
		a.drawShipboardPanel(screen, group)
		return
	}
	titleKey, ok := screenTitleKeys[a.state.Screen]
	if !ok {
		return
	}

	if !a.usesSTLayout() {
		uiFillRect(screen, panelX, panelY, panelW, panelH, color.RGBA{0, 0, 0, 235}, false)
		uiFillRect(screen, panelX+4, panelY+4, panelW-8, 16, group.Palette[group.UI.Panel], false)
		a.drawTextWithin(screen, a.lookup(titleKey), panelTextX, panelY+7, panelTextW, group.Palette[group.UI.Background])
	}
	if a.state.Screen == game.ScreenEngineering {
		a.drawEngineeringPanel(screen, group)
		return
	}
	if a.state.Screen == game.ScreenEquipment {
		a.drawEquipmentPanel(screen, group)
		return
	}

	if a.state.Screen == game.ScreenInterior {
		a.drawInteriorBackdrop(screen)
	}
	lines := a.screenPanelLines()
	if a.state.Screen == game.ScreenNavigation {
		if a.usesSTLayout() {
			// The original draws the maps in the viewport and keeps the menu
			// in the left box; the selector rows are that menu here.
			switch a.state.NavigationView {
			case game.NavigationSystem:
				a.drawSTSystemMap(screen)
			case game.NavigationPlanet:
				a.drawSTPlanetMap(screen)
			default:
				a.drawSTSectorChart(screen)
			}
			for index, line := range lines {
				a.drawSTMenuButton(screen, index, line)
			}
			return
		}
		// 航程那一行放在下方訊息框，它有 259 像素；左方框只有 68，裝不下燃料與時間。
		// 星圖佔了左方框的中段，選擇列排在它底下。
		a.drawNavigationMap(screen, group)
		for index, line := range lines {
			a.drawTextWithin(screen, line, panelTextX, routeRowY+index*routeRowStep, panelTextW, group.Palette[group.UI.Foreground])
		}
		return
	}
	readouts := clampInt(panelReadoutLines(a.state.Screen), 0, len(lines))
	window := a.readoutWindow(lines)
	hideReadouts := a.usesSTLayout() && a.state.Screen == game.ScreenTactical
	if readouts > 0 && !hideReadouts {
		a.drawFloatingWindow(screen, group, window)
	}
	// 指南針與圖示列畫在浮動視窗裡，所以要等視窗鋪好底才畫，否則會被那塊不透明的底蓋掉。
	if a.state.Screen == game.ScreenCharacterStatus {
		a.drawCompass(screen, group, window)
		a.drawCarriedIcons(screen, window)
		a.drawManFigure(screen, group)
	}
	for index, line := range lines {
		if index < readouts {
			if hideReadouts {
				continue
			}
			a.drawTextWithin(screen, line, window.columnX(0, 0), window.rowY(index, floatRowStep),
				window.textWidth(), group.Palette[group.UI.Foreground])
			continue
		}
		if a.usesSTLayout() {
			a.drawSTMenuButton(screen, index-readouts, line)
			continue
		}
		y, _ := panelRowYFor(a.state.Screen, index)
		a.drawTextWithin(screen, line, panelTextX, y, panelTextW, group.Palette[group.UI.Foreground])
	}
}

// attributeLine is the character status window's five attributes, in the
// original's own order and with its own two-letter labels. Luck prints as "??"
// because XDOUSERM:0x0d show_stats prints it that way — the player is not meant
// to see the number, and translating that placeholder would say more than the
// original does.
func (a *app) attributeLine() string {
	parts := make([]string, 0, game.AttributeCount)
	for index, label := range game.AttributeLabels {
		value := a.lookup("ui.attribute.hidden")
		if index != game.AttrLuck {
			value = strconv.Itoa(a.state.Attribute(index))
		}
		parts = append(parts, fmt.Sprintf("%s %s", label, value))
	}
	return strings.Join(parts, "  ")
}

// panelReadoutLines is how many of a screen's leading panel lines are readouts
// rather than buttons. Readouts pack two facts on a line, and the left box is
// only 68 pixels of writable width, so there they came out as an ellipsis.
// They are drawn in the floating window over the viewport instead, which has
// 143 — the buttons stay in the left box, which is what the player clicks.
func panelReadoutLines(screen game.Screen) int {
	switch screen {
	case game.ScreenStores:
		return 3
	case game.ScreenFuelPurchase:
		return 4
	case game.ScreenDepot:
		return 0
	case game.ScreenFTLBoost:
		return 3
	case game.ScreenCharacterStatus:
		return 6
	case game.ScreenTrade:
		return 5
	case game.ScreenFlight:
		return 3
	case game.ScreenUniteller:
		return 2
	case game.ScreenTubeStation:
		// 第一行是「選擇目的城市」，其餘每一行是一座城市。
		return 1
	case game.ScreenTactical:
		return 5
	case game.ScreenDialog:
		return 5
	case game.ScreenCargo:
		return 4
	case game.ScreenParts:
		// 零件櫃台整頁都是讀數，沒有按鈕列。
		return 8
	}
	return 0
}

func (a *app) sleepActionLabel() string {
	if a.state.Sleep.Active {
		if a.state.Sleep.Forced {
			return a.lookup("ui.sleep_forced")
		}
		return a.lookup("ui.wake")
	}
	return a.lookup("ui.rest")
}

// panelRowYFor is where one button row of screenPanelLines is drawn in the left
// box. The drawing, the click bands and the layout tests all go through it, so
// a row cannot move in one of them without moving in the other two.
//
// It answers for button rows only. The readout rows are in the floating window,
// whose position depends on how much it holds, so their y is worked out where
// they are drawn — and nothing clicks them.
func panelRowYFor(screen game.Screen, index int) (int, bool) {
	readouts := panelReadoutLines(screen)
	if index < readouts {
		return 0, false
	}
	if panelLayoutST {
		return stMenuRowY(index - readouts), true
	}
	return panelRowY0 + (index-readouts)*panelRowStep, true
}

// panelLayoutST is which layout the panel rows answer for. Draw and Update set
// it from the state each frame; the hotspot tables are built once per value.
var panelLayoutST bool

func setPanelLayout(layout theme.LayoutID) { panelLayoutST = layout == theme.AtariSTLayout }

// screenPanelLines is what the left panel puts under its title on the current
// screen, one string per row. It is separate from the drawing so a test can
// count the rows and check that the longest screen still clears the shared
// status-message strip.
func (a *app) screenPanelLines() []string {
	if a.state.Screen == game.ScreenTrade && a.state.TradePage == game.TradePageSellConfirm {
		lot, _ := game.PendingTradeLot(a.state)
		first, second := "ui.trade.accept", "ui.trade.withdraw"
		amount := game.FormatCredits(game.Credits(a.state.TradeSalePrice))
		if a.state.CityID == world.MissionDestinationID {
			first, second = "ui.trade.deliver", "ui.trade.withhold"
			amount = fmt.Sprintf("%s %d", a.lookup("ui.trade.demand"), a.state.Colony.Remaining[lot.Goods])
		}
		if !game.TradeSaleAvailable(a.state) {
			first, second = "ui.trade.nothing", "ui.trade.exit"
		}
		return []string{a.lookup("ui.message.trade_confirm_sale"), a.stLotName(lot), amount,
			fmt.Sprintf("%s %s", a.lookup("ui.trade.your_limit"), game.FormatCredits(game.TradeFunds(a.state))), "", a.lookup(first), a.lookup(second)}
	}
	targetDestination, _ := a.reducer.World.Destination(a.state.NavigationTargetID)
	flightProgressLine := fmt.Sprintf("%s：%d%%", a.lookup("ui.flight_progress"), game.FlightProgress(a.state))
	if a.state.FuelRescuePending {
		// Keep ETA visible after save/load while retaining target and ticks.
		flightProgressLine = fmt.Sprintf("%s %d", a.lookup("ui.message.refuel_eta"), a.state.FuelRescueDue)
	}
	marketLine := a.lookup("ui.market_unknown")
	if quote, ok := a.reducer.World.Market(a.state.CityID); ok {
		// The market quote's key is a source key (atarist.cargo.fuel), not a UI
		// key, so looking it up in the UI catalog puts the raw key on screen.
		// CargoName is what maps it to ui.cargo.<id> and falls back to the
		// source record's own English name rather than to a key.
		marketLine = fmt.Sprintf("%s %d／%d", a.reducer.CargoName(a.state), quote.BuyPrice, quote.SellPrice)
	}
	return map[game.Screen][]string{
		game.ScreenStores: {
			fmt.Sprintf("%s %d／20: %s", a.lookup("ui.ship_stores"), a.state.SelectedPartIndex+1, a.storeItemLabel()),
			fmt.Sprintf("%s: %s", a.lookup("ui.hand"), a.handLabel()),
			fmt.Sprintf("%s: %d／4", a.lookup("ui.carry"), game.CarriedItemCount(a.state)),
			a.lookup("ui.stores_keys"),
			a.lookup("ui.hand_key"),
			a.lookup("ui.eat"),
		},
		game.ScreenCharacterStatus: {
			// The manual's page 6 lists four readings, in this order. They are
			// the original's four G3d bytes on its own 0-255 scale.
			fmt.Sprintf("%s：%d  %s：%d", a.lookup("ui.vigor"), a.state.Vigor,
				a.lookup("ui.rest"), a.state.Rest),
			fmt.Sprintf("%s：%d  %s：%d", a.lookup("ui.health"), a.state.Health,
				a.lookup("ui.nourishment"), a.state.Nourishment),
			// The compass has a drawn dial of its own (drawCompass), so the
			// left box does not spend a row repeating it as text -- that row
			// is what the fourth reading needed.
			fmt.Sprintf("%s：%d  %s：%d", a.lookup(foodLabelKey(a.state)), game.CarriedFoodCount(a.state),
				a.lookup("ui.time"), a.state.TimeMinutes),
			fmt.Sprintf("%s：%s  %s：%d／%d", a.lookup("ui.hand"), a.handLabel(),
				a.lookup("ui.carry"), game.CarriedItemCount(a.state), game.CarrySlots),
			a.attributeLine(),
			a.lookup("ui.hand_key"),
			a.sleepActionLabel(),
			a.lookup("ui.eat"),
		},
		game.ScreenGround: {
			a.lookup("ui.depot_title"),
			a.lookup("ui.trade_exchange"),
			a.lookup("ui.uniteller"),
			a.lookup("ui.load_supplies"),
			a.lookup("ui.load_cryogens"),
			a.lookup("ui.deliver_mission"),
			a.lookup("ui.tube.title"),
			a.lookup("ui.load_aux_fuel"),
		},
		game.ScreenFuelPurchase: a.fuelPurchasePanelLines(),
		game.ScreenDepot:        a.depotPanelLines(),
		game.ScreenFTLBoost: {
			fmt.Sprintf("%s：%s", a.lookup("ui.ftl_price"), game.FormatCredits(game.FTLBoostPrice(a.state))),
			fmt.Sprintf("%s：%s", a.lookup("ui.fuel_available"), game.FormatCredits(max(0, a.state.Bank)+a.state.Cash)),
			fmt.Sprintf("%s：%d → 9", a.lookup("ui.ftl_range"), a.state.JumpRange),
			a.lookup("ui.ftl_install"), a.lookup("ui.fuel_cancel"),
		},
		game.ScreenParts: a.partsCounterLines(),
		game.ScreenTrade: {
			a.lookup("ui.market_quote"),
			marketLine,
			fmt.Sprintf("%s：%s", a.lookup("ui.cash"), game.FormatCredits(a.state.Cash)),
			fmt.Sprintf("%s：%s", a.lookup("ui.bank"), game.FormatCredits(a.state.Bank)),
			fmt.Sprintf("%s：%d", a.lookup("ui.cargo_units"), game.CargoUnits(a.state)),
			a.lookup("ui.buy"),
			a.lookup("ui.sell"),
		},
		game.ScreenFlight: {
			fmt.Sprintf("%s：%s", a.lookup("ui.navigation_target"), a.state.NavigationTarget),
			flightProgressLine,
			fmt.Sprintf("%s：%d／%d", a.lookup("ui.flight_tick"), a.state.FlightTick, a.state.FlightTicks),
			a.lookup("ui.flight_tick_key"),
			a.lookup("ui.flight_engage_key"),
			a.lookup("ui.flight_distress_key"),
			a.lookup("ui.flight_resume_key"),
		},
		game.ScreenNavigation: {
			// The route maps take the middle of the panel, so this screen keeps
			// only which of the three levels each selector row is on. The cost
			// is in the message box, which is wide enough for it.
			a.navigationSelectorLine("ui.system", targetDestination.SystemName),
			a.navigationSelectorLine("ui.planet", targetDestination.PlanetName),
			a.navigationSelectorLine("ui.city", targetDestination.CityName),
		},
		game.ScreenUniteller: {
			fmt.Sprintf("%s：%s  %s：%s", a.lookup("ui.cash"), game.FormatCredits(a.state.Cash),
				a.lookup("ui.bank"), game.FormatCredits(a.state.Bank)),
			fmt.Sprintf("%s：%s", a.lookup("ui.action.pay_fine"), game.FormatCredits(a.reducer.ParkingFine(a.state))),
			a.lookup("ui.deposit"),
			a.lookup("ui.withdraw"),
			a.lookup("ui.transfer"),
			a.lookup("ui.action.pay_fine"),
		},
		game.ScreenTubeStation: a.tubeStationLines(),
		game.ScreenTactical: {
			fmt.Sprintf("%s：%s", a.lookup("ui.enemy_ship"), a.reducer.EnemyShipName(a.state)),
			fmt.Sprintf("%s：%d%%", a.lookup("ui.enemy_hull"), game.EnemyHullPercent(a.state)),
			fmt.Sprintf("%s：%d", a.lookup("ui.combat_range"), a.state.CombatRange),
			fmt.Sprintf("%s：%d", a.lookup("ui.weapon_charge"), a.state.WeaponCharge),
			fmt.Sprintf("%s：%d", a.lookup("ui.shield_level"), a.state.ShieldLevel),
			a.lookup("ui.tactical_move"),
			a.lookup("ui.fire"),
			a.lookup("ui.recharge"),
			a.lookup("ui.full_shields"),
			// The tractor beam, the jettison bolts and the comm channel are
			// keys only, with no row of their own: these lines are drawn in
			// step with the click regions below, and the panel's last row may
			// not go past y=127. Their legend lives on the F1 overlay and in
			// docs/manual/remake-controls-zh-Hant.md.
		},
		game.ScreenEngineering: {
			fmt.Sprintf("%s：%d／%d", a.lookup("ui.hull_condition"), a.state.HullCondition, game.MaxHullCondition()),
			fmt.Sprintf("%s：%d", a.lookup("ui.fuel"), a.state.Fuel),
			a.lookup("ui.repair"),
		},
		game.ScreenDialog:   a.dialogPanelLines(),
		game.ScreenInterior: a.interiorPanelLines(),
		game.ScreenSlots:    a.slotsPanelLines(),
		game.ScreenArena:    a.arenaPanelLines(),
		game.ScreenScore:    a.scorePanelLines(),
		game.ScreenCargo: {
			fmt.Sprintf("%s×%d", a.reducer.CargoName(a.state), a.state.Gems),
			fmt.Sprintf("%s：%d/%d  %s：%d", a.lookup("ui.cargo_units"), game.CargoUnits(a.state),
				game.CargoLots, a.lookup("ui.aux_fuel"), game.AuxFuelTanks(a.state)),
			fmt.Sprintf("%s：%d", a.lookup("ui.supplies"), game.SuppliesAboard(a.state)),
			fmt.Sprintf("%s：%d", a.lookup("ui.cryogens"), game.CryogensAboard(a.state)),
			a.lookup("ui.load_cargo"),
			a.lookup("ui.unload_cargo"),
			a.lookup("ui.unload_aux_fuel"),
			a.lookup("ui.pod_exit"),
		},
	}[a.state.Screen]
}

func (a *app) fuelPurchasePanelLines() []string {
	p := a.state.FuelPurchase
	kind, quantity, capacity := "ui.gauge.fuel", p.Quantity, 16
	if p.Kind == game.FuelPurchaseAux {
		kind, quantity, capacity = "ui.aux_fuel", p.Quantity-(16-game.CargoLots), game.CargoLots
	}
	if p.Kind == game.FuelPurchaseHull {
		kind = "ui.hull_condition"
	}
	price := game.FormatCredits(p.UnitPrice)
	if p.UnitPrice < 0 {
		price = a.lookup("ui.fuel_unavailable")
	}
	lines := []string{
		a.lookup(kind),
		fmt.Sprintf("%s：%s", a.lookup("ui.fuel_unit_price"), price),
		fmt.Sprintf("%s：%d／%d", a.lookup("ui.fuel_quantity"), quantity, capacity),
		fmt.Sprintf("%s：%s", a.lookup("ui.fuel_available"), game.FormatCredits(p.Funds)),
		a.lookup("ui.fuel_buy_full"), a.lookup("ui.fuel_buy_one"), a.lookup("ui.fuel_cancel"),
	}
	return lines
}

func (a *app) depotPanelLines() []string {
	lines := []string{a.lookup("ui.depot_refuel"), a.lookup("ui.depot_repair"), a.lookup("ui.depot_aux")}
	if a.state.CityID == "banville" {
		lines = append(lines, a.lookup("ui.ftl_title"))
	}
	return lines
}

func (a *app) drawShipboardPanel(screen *ebiten.Image, group theme.Group) {
	uiFillRect(screen, panelX, panelY, panelW, panelH, color.RGBA{0, 0, 0, 235}, false)
	uiFillRect(screen, panelX+4, panelY+4, panelW-8, 16, group.Palette[group.UI.Panel], false)
	a.drawTextWithin(screen, a.lookup("ui.shipboard"), panelTextX, panelY+7, panelTextW, group.Palette[group.UI.Background])
	a.drawWrappedTextWithin(screen, a.lookup("ui.shipboard_hint"), panelTextX, panelRowY0,
		panelTextW, 3, group.Palette[group.UI.Foreground])
	// 十二個艙位在 82 寬的左方框裡排不成兩欄，所以照原版的作法蓋一個小視窗在視埠上。
	window, columnWidth := a.shipboardWindow()
	a.drawFloatingWindow(screen, group, window)
	for index, row := range shipboardStationRows(a.labels) {
		y := window.rowY(index, shipboardRowStep)
		a.drawTextWithin(screen, row[0], window.columnX(0, columnWidth), y,
			columnWidth-floatColumnGap, group.Palette[group.UI.Foreground])
		a.drawTextWithin(screen, row[1], window.columnX(1, columnWidth), y,
			columnWidth-floatColumnGap, group.Palette[group.UI.Foreground])
	}
}

// shipboardStationRows is the ship's twelve stations, two to a row, in the
// original's own order. The drawing and the click band both read it, so a
// station cannot move in one without moving in the other.
func shipboardStationRows(labels catalog.Catalog) [][2]string {
	name := func(key string) string { return lookupLabel(labels, key, key) }
	return [][2]string{
		{name("ui.airlock"), name("ui.ship_locker")},
		{name("ui.ship_stores"), name("ui.pilotage")},
		{name("ui.tactical"), name("ui.shields")},
		{name("ui.warp_drive"), name("ui.subc_engine")},
		{name("ui.guns"), name("ui.pod_locker")},
		{name("ui.pod_driver"), name("ui.cargo_pod")},
	}
}

// shipboardWindow sizes the station window around its two columns, and reports
// how wide a column came out.
func (a *app) shipboardWindow() (floatWindow, int) {
	rows := shipboardStationRows(a.labels)
	names := make([]string, 0, len(rows)*2)
	for _, row := range rows {
		names = append(names, row[0], row[1])
	}
	columnWidth := a.widestRow(names) + floatColumnGap
	window := fitFloatWindow(columnWidth*2, len(rows)*shipboardRowStep)
	// 視窗被夾到最大寬度時，欄寬要跟著收，否則右欄會畫到框外面。
	if available := (window.textWidth()) / 2; columnWidth > available {
		columnWidth = available
	}
	return window, columnWidth
}

// floatWindow is where a floating window ended up. It is a value, not a set of
// constants: the window is only as big as what it holds, so the rows, the
// compass and the icon row all read their positions off the box they are in.
type floatWindow struct {
	x, y, w, h int
}

// rowY is the y of one row inside the window.
func (w floatWindow) rowY(index, step int) int { return w.y + floatPadding + index*step }

// columnX is the x of one column, given how wide a column is.
func (w floatWindow) columnX(column, width int) int { return w.x + floatPadding + column*width }

// textWidth is how wide one full-width row may be.
func (w floatWindow) textWidth() int { return w.w - 2*floatPadding }

// bottomRowY is the strip along the bottom that the compass and the carried
// items share.
func (w floatWindow) bottomRowY() int { return w.y + w.h - floatBottomRowInset }

// fitFloatWindow centres a window of the given content inside the viewport,
// clamped to the viewport's inner edge. Content bigger than the viewport is not
// an error: it is drawn clipped by the caller's own width, the same as before.
func fitFloatWindow(contentWidth, contentHeight int) floatWindow {
	// 版面以 8 像素的字面量測，但字實際畫在視窗解析度上，一列的墨會比 8 高一點。
	// 底下多留 floatDescent，最後一列才不會被框線切到。
	width := clampInt(contentWidth+2*floatPadding, floatMinW, floatW)
	height := clampInt(contentHeight+2*floatPadding+floatDescent, floatMinH, floatH)
	return floatWindow{
		x: floatX + (floatW-width)/2,
		y: floatY + (floatH-height)/2,
		w: width,
		h: height,
	}
}

// textWidth measures a string on the layout face, which is the face every
// width in this file is expressed in.
func (a *app) textWidth(value string) int {
	if a.face == nil {
		return 0
	}
	return font.MeasureString(a.face, value).Ceil()
}

// widestRow is how much room the longest of these rows needs.
func (a *app) widestRow(rows []string) int {
	widest := 0
	for _, row := range rows {
		if width := a.textWidth(row); width > widest {
			widest = width
		}
	}
	return widest
}

// readoutWindow is the floating window this screen's readout rows are drawn in.
// The drawing, the compass, the icon row and the layout rules all ask for it,
// so it is worked out in one place rather than four.
func (a *app) readoutWindow(lines []string) floatWindow {
	readouts := clampInt(panelReadoutLines(a.state.Screen), 0, len(lines))
	// 角色狀態視窗的讀數底下還有一排身上的東西，視窗要留出那條的高度。
	extra := 0
	if a.state.Screen == game.ScreenCharacterStatus {
		extra = originalmedia.ItemIconSize + 4
	}
	return a.floatWindowForRows(lines[:readouts], floatRowStep, extra)
}

// floatWindowForRows sizes a one-column window around its rows. extraHeight is
// for whatever else the screen puts under them — the status window's icon row.
func (a *app) floatWindowForRows(rows []string, rowStep, extraHeight int) floatWindow {
	return fitFloatWindow(a.widestRow(rows), len(rows)*rowStep+extraHeight)
}

// drawFloatingWindow paints the opaque box the list screens draw their rows in.
// It sits over the viewport, which is where the original puts its own menus.
func (a *app) drawFloatingWindow(screen *ebiten.Image, group theme.Group, window floatWindow) {
	// 不透明：原版的浮動視窗會把底下的畫面整個蓋掉，半透明會讓文字和地圖疊在一起。
	uiFillRect(screen, float32(window.x), float32(window.y), float32(window.w), float32(window.h),
		color.RGBA{0, 0, 0, 255}, false)
	border := group.Palette[group.UI.Accent]
	if a.usesSTLayout() {
		// The original's windows have the frame's yellow edge.
		border = stColour(stColourBox)
	}
	uiStrokeRect(screen, float32(window.x), float32(window.y), float32(window.w), float32(window.h),
		1, border, false)
}

func (a *app) drawEngineeringPanel(screen *ebiten.Image, group theme.Group) {
	a.drawEngineeringPanelAt(screen, group, panelTextX, engineeringRowY0, panelTextW)
}

// drawEngineeringPanelAt draws the engineering rows with their first row at
// (x, y) and the given text width: the left box on the framed screens, the
// repair window under the Atari ST layout.
func (a *app) drawEngineeringPanelAt(screen *ebiten.Image, group theme.Group, x, y, width int) {
	dy := y - engineeringRowY0
	meterX := x + engineeringLabelWidth + 2
	meterWidth := x + width - meterX
	rows := []struct {
		label string
		value int
	}{
		{label: a.lookup("ui.engineering_fuel_system"), value: a.state.Fuel * 100 / 255},
		{label: a.lookup("ui.engineering_weapon_system"), value: a.state.WeaponCharge * 100 / 255},
		{label: a.lookup("ui.engineering_shield_system"), value: a.state.ShieldLevel * 100 / 255},
		{label: a.lookup("ui.engineering_hull_system"), value: game.HullPercent(a.state)},
	}
	for index, row := range rows {
		rowY := engineeringRowY0 + dy + index*engineeringRowStep
		a.drawTextWithin(screen, row.label, x, rowY, engineeringLabelWidth, group.Palette[group.UI.Foreground])
		a.drawEngineeringMeter(screen, meterX, rowY+2, meterWidth, 7, row.value, group)
	}
	selectedLabel := a.lookup("ui.system_unknown")
	if selected, ok := a.state.SelectedShipSystem(); ok {
		selectedLabel = a.lookup(game.ShipSystemLabelKey(selected.ID))
		// The meter is drawn on a 0..100 scale; the system's own scale is the
		// source's 0..16, so it is shown as a proportion of a working system.
		a.drawEngineeringMeter(screen, meterX, engineeringSelectedY+dy+2, meterWidth, 7,
			selected.Status*100/game.MaxShipSystemStatus(), group)
	}
	a.drawTextWithin(screen, "*"+selectedLabel, x, engineeringSelectedY+dy, engineeringLabelWidth, group.Palette[group.UI.Foreground])
	a.drawTextWithin(screen, fmt.Sprintf("%s：%d", a.lookup("ui.shunts"), a.state.Shunts),
		x, engineeringShuntY+dy, width, group.Palette[group.UI.Foreground])
	// The four rows are the system's redundancy; showing their statuses is what
	// tells the player whether a shunt would achieve anything. 分流器數與各列狀態
	// 分成兩列，一列 68 像素放不下兩者。
	if selected, ok := a.state.SelectedShipSystem(); ok {
		statuses := make([]string, 0, len(selected.Rows))
		for row := range selected.Rows {
			statuses = append(statuses, strconv.Itoa(game.ShipSystemRowStatus(selected, row)))
		}
		a.drawTextWithin(screen, a.lookup("ui.system_rows")+"："+strings.Join(statuses, "·"),
			x, engineeringRowsY+dy, width, group.Palette[group.UI.Foreground])
	}
}

func (a *app) drawEquipmentPanel(screen *ebiten.Image, group theme.Group) {
	keys := []string{}
	if a.reducer.AppleIIText != nil {
		keys = a.reducer.AppleIIText.KeysWithPrefix("apple2.equipment.")
	}
	if len(keys) == 0 {
		a.drawWrappedTextWithin(screen, a.lookup("ui.equipment_unavailable"), panelTextX, panelRowY0, panelTextW, 3, group.Palette[group.UI.Foreground])
		return
	}
	// 十六項設備同樣放不進左方框，走跟艙位一樣的浮動視窗。
	entries := make([]string, 0, len(keys))
	for index, key := range keys {
		entries = append(entries, fmt.Sprintf("%02d %s", index+1, a.equipmentLabel(key)))
	}
	rows := (len(entries) + equipmentColumns - 1) / equipmentColumns
	columnWidth := a.widestRow(entries) + floatColumnGap
	window := fitFloatWindow(columnWidth*equipmentColumns, rows*equipmentRowStep)
	if available := window.textWidth() / equipmentColumns; columnWidth > available {
		columnWidth = available
	}
	a.drawFloatingWindow(screen, group, window)
	for index, entry := range entries {
		a.drawTextWithin(screen, entry,
			window.columnX(index/rows, columnWidth),
			window.rowY(index%rows, equipmentRowStep),
			columnWidth-floatColumnGap, group.Palette[group.UI.Foreground])
	}
	a.drawWrappedTextWithin(screen, a.lookup("ui.equipment_note"), panelTextX, equipmentNoteY,
		equipmentNoteWidth, equipmentNoteMaxLines, group.Palette[group.UI.Foreground])
}

// equipmentLabel prefers the Apple II catalog's own translation, which is where
// these names come from, and falls back to the UI catalog.
func (a *app) equipmentLabel(key string) string {
	if a.reducer.AppleIIText != nil {
		if translated, ok := a.reducer.AppleIIText.Localized(key); ok {
			return translated
		}
	}
	return a.lookup(key)
}

func (a *app) drawEngineeringMeter(screen *ebiten.Image, x, y, width, height, value int, group theme.Group) {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	uiFillRect(screen, float32(x), float32(y), float32(width), float32(height), group.Palette[group.UI.Panel], false)
	if value > 0 {
		fillWidth := width * value / 100
		uiFillRect(screen, float32(x), float32(y), float32(fillWidth), float32(height), group.Palette[group.UI.Accent], false)
	}
	uiStrokeRect(screen, float32(x), float32(y), float32(width), float32(height), 1, group.Palette[group.UI.Foreground], false)
}

func (a *app) currentFacilities() string {
	facilities := a.reducer.GroundFacilitiesAt(a.state, world.TilePosition{X: a.state.Position.X, Y: a.state.Position.Y})
	if len(facilities) == 0 {
		return a.lookup("ui.facility.none")
	}
	labels := make([]string, 0, len(facilities))
	for _, facility := range facilities {
		labels = append(labels, a.lookup("ui.facility."+string(facility)))
	}
	return strings.Join(labels, "、")
}

func (a *app) drawPattern(screen *ebiten.Image, x, y int, pattern theme.PixelPattern, palette []color.RGBA) {
	for row, values := range pattern.Rows {
		for column := range values {
			index, ok := theme.PaletteIndex(values[column])
			if !ok || index >= len(palette) {
				continue
			}
			uiFillRect(screen, float32(x+column*tileScale), float32(y+row*tileScale), tileScale, tileScale, palette[index], false)
		}
	}
}

// drawMessageStrip is the shared status line at the bottom: what just happened,
// and where the ship is headed. It used to live inside drawStatus, which meant
// the screens that skipped the status column lost their messages too -- the
// modern ground layout has been drawing no messages at all.
// drawFrame paints the original's chrome: the seven regions measured in
// docs/ui/original-layout.json. The original's own frame is one 320x200 picture
// on the disk; this draws the same partition with the remake's own lines, so a
// player without the disk gets the same shape.
func (a *app) drawFrame(screen *ebiten.Image, group theme.Group) {
	panel := group.Palette[group.UI.Panel]
	accent := group.Palette[group.UI.Accent]
	background := group.Palette[group.UI.Background]
	// The frame's own ground: everything outside the boxes.
	uiFillRect(screen, 0, 0, logicalWidth, logicalHeight, panel, false)
	box := func(x, y, w, h float32, border color.RGBA) {
		uiFillRect(screen, x, y, w, h, background, false)
		uiStrokeRect(screen, x, y, w, h, 1, border, false)
	}
	box(frameViewportX, frameViewportY, frameViewportW, frameViewportH, accent)
	box(frameLeftX, frameLeftY, frameLeftW, frameLeftH, accent)
	box(frameGaugeX, frameGaugeY, frameGaugeW, frameGaugeH, accent)
	box(frameMessageX, frameMessageY, frameMessageW, frameMessageH, accent)
	box(frameTopLeftX, frameTopLeftY, frameTopLeftW, frameTopLeftH, accent)
	box(frameTopRightX, frameTopRightY, frameTopRightW, frameTopRightH, accent)
	// 原版在這兩個小框上方寫 CONDITION 與 WARP，框裡放讀數。重製版放現金與時間：
	// 兩個玩家隨時想知道、而且短到放得下的數字。
	foreground := group.Palette[group.UI.Foreground]
	a.drawTextWithin(screen, fmt.Sprintf("%s %s", a.lookup("ui.cash"), game.FormatCredits(a.state.Cash)),
		frameTopLeftX+3, frameTopLeftY+3, frameTopLeftW-6, foreground)
	a.drawTextWithin(screen, fmt.Sprintf("%s %d", a.lookup("ui.time"), a.state.TimeMinutes),
		frameTopRightX+3, frameTopRightY+3, frameTopRightW-6, foreground)
}

// gaugeRows are the four vertical bars the original puts down the right-hand
// box, in its own order. It spells the labels one letter per row; the remake
// writes one character per row, which is the same idea.
func (a *app) drawGauges(screen *ebiten.Image, group theme.Group) {
	type gauge struct {
		key     string
		value   int
		maximum int
	}
	// 標籤逐字直書，所以要短。原版那四根也是縮寫：FUEL、GUNS、SHLD、WARP。
	gauges := []gauge{
		{"ui.gauge.fuel", a.state.Fuel, 255},
		{"ui.gauge.guns", a.state.WeaponCharge, 255},
		{"ui.gauge.shield", a.state.ShieldLevel, 255},
		{"ui.gauge.hull", a.state.HullCondition, game.MaxHullCondition()},
	}
	foreground := group.Palette[group.UI.Foreground]
	accent := group.Palette[group.UI.Accent]
	slot := frameGaugeW / len(gauges)
	barTop := frameGaugeY + 6
	barHeight := frameGaugeH - 46
	for index, g := range gauges {
		x := frameGaugeX + index*slot + 2
		uiStrokeRect(screen, float32(x), float32(barTop), float32(slot-4), float32(barHeight), 1, foreground, false)
		filled := 0
		if g.maximum > 0 {
			filled = clampInt(g.value*barHeight/g.maximum, 0, barHeight)
		}
		uiFillRect(screen, float32(x+1), float32(barTop+barHeight-filled),
			float32(slot-6), float32(filled), accent, false)
		// 標籤逐字直書，一列一個字，跟原版把 FUEL 拆成四列同一個作法。
		label := a.lookup(g.key)
		for row, glyph := range []rune(label) {
			a.drawText(screen, string(glyph), x, barTop+barHeight+3+row*9, foreground)
		}
	}
}

// ItemIconStep is how far apart the carried-item icons sit: the original's own
// cell size plus a pixel of air, so two icons never touch.
const ItemIconStep = originalmedia.ItemIconSize + 1

// itemIcon returns the original's picture for an item byte, if the player gave
// a disk and that disk has one. The strip is decoded once: it is 31 cells, and
// re-cutting it every frame would be pure waste.
func (a *app) itemIcon(item int) (*ebiten.Image, bool) {
	if a.usesHD() {
		icon := hdItemImage(game.ItemCode(item), a.viewScale())
		return icon, icon != nil
	}

	if !a.itemIconsRead {
		if a.originalDisk == nil {
			return nil, false
		}
		a.itemIconsRead = true
		icons, err := a.originalDisk.ItemIcons()
		if err != nil {
			// The remake always has its words; a disk without a readable strip
			// costs the pictures, not the screen.
			log.Printf("原版磁碟的物品圖示讀不出來，只顯示文字：%v", err)
			return nil, false
		}
		a.itemIcons = map[int]*ebiten.Image{}
		for code := originalmedia.FirstItemIconCode; ; code++ {
			cell, ok := icons.Icon(code)
			if !ok {
				break
			}
			a.itemIcons[code] = ebiten.NewImageFromImage(cell)
		}
	}
	icon, ok := a.itemIcons[game.ItemCode(item)]
	return icon, ok
}

// manFigurePoints is the original's character-window figure: the 12-point
// polyline WINDOWLI:0x1a draw_man hands MAINLIB:0x40 draw_poly. It lives on
// the disk at raw 0xd482 (docs/atari-st/sine-table.json, blockExtras.manFigure,
// rule man.draw) — geometry data, embedded the way the skyline heights are.
var manFigurePoints = [12][2]int{
	{1, 20}, {6, 20}, {9, 23}, {9, 29}, {4, 33}, {10, 35},
	{10, 36}, {21, 47}, {17, 51}, {9, 43}, {9, 52}, {1, 52},
}

// drawManFigure puts the original's little man in the character screen's left
// box, under the buttons. The original draws him inside its zoom window at
// these very coordinates; the remake's status window is full of readouts, so
// the figure stands in the left box instead, doubled to stay legible.
func (a *app) drawManFigure(screen *ebiten.Image, group theme.Group) {
	const scale = 2
	originX := panelX + (panelW-21*scale)/2
	originY := panelY + panelH - 4 - (52-20)*scale
	ink := group.Palette[group.UI.Foreground]
	for index := 0; index+1 < len(manFigurePoints); index++ {
		x0, y0 := manFigurePoints[index][0], manFigurePoints[index][1]-20
		x1, y1 := manFigurePoints[index+1][0], manFigurePoints[index+1][1]-20
		uiStrokeLine(screen,
			float32(originX+x0*scale), float32(originY+y0*scale),
			float32(originX+x1*scale), float32(originY+y1*scale), 1, ink, false)
	}
}

// interiorBackdropStyles is get_building's per-kind window style
// (city-subsystems.json, interior.layout-*): the first argument each building
// hands make_zoom, which draw_zoom's case table turns into a picture range.
var interiorBackdropStyles = map[game.InteractionKind]int{
	game.KindBarCounter:    originalmedia.StyleBar,
	game.KindEquipmentShop: originalmedia.StyleEquipment,
	game.KindPartsShop:     originalmedia.StylePartsShop,
	game.KindHotel:         originalmedia.StyleHotel,
}

// drawInteriorBackdrop paints the original's own 160x100 room picture across
// the viewport centre, the way draw_zoom paints it across the pop-up room
// window. Which picture follows pick_pic: fixed per city (the hash folds the
// city record's first word with the entry cell, rule interior.city-hash), and
// always the range's base picture in Enlie. Without a disk nothing is drawn —
// the room already works as words and markers.
func (a *app) drawInteriorBackdrop(screen *ebiten.Image) {
	backdrop := a.interiorBackdropImage()
	if backdrop == nil {
		return
	}
	options := &ebiten.DrawImageOptions{}
	options.Filter = ebiten.FilterNearest
	options.GeoM.Translate(
		float64(frameViewportX+(frameViewportW-originalmedia.RoomBackdropWidth)/2),
		float64(frameViewportY+(frameViewportH-originalmedia.RoomBackdropHeight)/2))
	screen.DrawImage(backdrop, options)
}

// interiorBackdropImage is the original's 160×100 room picture for the
// building the character is in, or nil without a disk or a room.
func (a *app) interiorBackdropImage() *ebiten.Image {
	if a.originalDisk == nil || !a.state.Interior.Active {
		return nil
	}
	style, ok := interiorBackdropStyles[a.state.Interior.Kind]
	if !ok {
		return nil
	}
	entry := a.state.Interior.ReturnPosition
	_, cityNumber, ok := game.GroundTerrainCode(a.state, a.state.CityID, entry.X, entry.Y)
	if !ok {
		return nil
	}
	word0, err := a.originalDisk.CityRecordWord0(cityNumber)
	if err != nil {
		return nil
	}
	// The entry coordinates are the remake's own city grid; the original folds
	// its double-resolution entry cell instead. Same shape, remake-owned units.
	hash := word0 + entry.X + entry.Y
	enlie := a.state.SystemID == "enlie" // system.enlie-flag: system 3 of the table
	index, ok := originalmedia.BackdropIndex(style, hash, enlie)
	if !ok {
		return nil
	}
	if a.usesHD() {
		return a.hdRoomImage(index)
	}
	if a.interiorBackdrops == nil {
		a.interiorBackdrops = map[int]*ebiten.Image{}
	}
	backdrop, cached := a.interiorBackdrops[index]
	if !cached {
		decoded, err := a.originalDisk.RoomBackdrop(index)
		if err != nil {
			log.Printf("原版室內背景 %d 讀不出來，只顯示文字：%v", index, err)
			a.interiorBackdrops[index] = nil
			return nil
		}
		backdrop = ebiten.NewImageFromImage(decoded)
		a.interiorBackdrops[index] = backdrop
	}
	return backdrop
}

// carriedItemCodes is what the character is carrying, in the order the icon row
// shows it: what is in the hand first, then the parts in the pockets.
func (a *app) carriedItemCodes() []int {
	codes := make([]int, 0, 1+len(a.state.Parts))
	codes = append(codes, a.state.HandItem)
	codes = append(codes, a.state.Parts...)
	return codes
}

// carriedIconRow pairs each carried item with the x it is drawn at. Items past
// the end of the row are dropped rather than drawn over the window's edge; the
// panel's words still list everything either way.
func (a *app) carriedIconRow(window floatWindow) []carriedIcon {
	codes := a.carriedItemCodes()
	right := window.x + window.w - floatCompassInset
	row := make([]carriedIcon, 0, len(codes))
	for index, item := range codes {
		x := window.columnX(0, 0) + index*carriedIconStep
		if x+originalmedia.ItemIconSize > right {
			break
		}
		row = append(row, carriedIcon{item: item, x: x})
	}
	return row
}

type carriedIcon struct {
	item int
	x    int
}

// drawCarriedIcons puts the character's belongings on one row of the original's
// own item icons. Without a disk there are no pictures and nothing is drawn:
// the panel's words already say what is carried.
func (a *app) drawCarriedIcons(screen *ebiten.Image, window floatWindow) {
	if a.originalDisk == nil {
		return
	}
	for _, cell := range a.carriedIconRow(window) {
		icon, ok := a.itemIcon(game.ItemCode(cell.item))
		if !ok {
			continue
		}
		options := &ebiten.DrawImageOptions{}
		options.Filter = ebiten.FilterNearest
		options.GeoM.Translate(float64(cell.x), float64(window.bottomRowY()))
		if a.usesHD() {
			hdWorldBlit(screen, icon, a.originX+cell.x*a.viewScale(), a.originY+window.bottomRowY()*a.viewScale())
		} else {
			screen.DrawImage(icon, options)
		}
	}
	selectedX := window.columnX(0, 0) + (a.state.SelectedPartIndex+1)*carriedIconStep
	uiStrokeRect(screen, float32(selectedX), float32(window.bottomRowY()), originalmedia.ItemIconSize, originalmedia.ItemIconSize, 1, stColour(stColourWhite), false)
}

func (a *app) statusItemClick(x, y int) bool {
	w := a.readoutWindow(a.screenPanelLines())
	if a.usesSTShipboardBackdrop() || a.usesSTGroundBackdrop() {
		zoom, _, _ := a.stZoomOf()
		w = zoom.content()
	}
	if y < w.bottomRowY() || y >= w.bottomRowY()+originalmedia.ItemIconSize {
		return false
	}
	for index, cell := range a.carriedIconRow(w) {
		if x >= cell.x && x < cell.x+originalmedia.ItemIconSize {
			if index == 0 {
				a.step(game.ActionEat)
			} else {
				a.state.SelectedPartIndex = index - 1
				a.state = a.reducer.PickPocketPart(a.state, index-1)
			}
			return true
		}
	}
	return false
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func (a *app) drawMessageStrip(screen *ebiten.Image, group theme.Group) {
	foreground := group.Palette[group.UI.Foreground]
	x, width := frameMessageX+5, frameMessageW-10
	// 這個框放得下七列（列距九像素）：訊息、所在／目標、情境讀數、鍵位提示兩列、
	// 操作提示兩列。窄的左方框裝不下的那幾行都收在這裡——它有 259 像素。
	second := fmt.Sprintf("%s：%s", a.lookup("ui.navigation_target"),
		localization.Destination(a.labels, a.state.NavigationTargetID, a.state.NavigationTarget))
	third := ""
	switch a.state.Screen {
	case game.ScreenGround:
		second = fmt.Sprintf("%s：%s", a.lookup("ui.location"),
			localization.Destination(a.labels, a.state.CityID, a.state.Location))
		third = fmt.Sprintf("%s：%s", a.lookup("ui.facilities"), a.currentFacilities())
	case game.ScreenNavigation:
		third = a.routeCostLine()
	}
	row := func(index int) int { return statusStripY + index*messageRowStep }
	a.drawTextWithin(screen, a.messageLine(), x, row(0), width, foreground)
	a.drawTextWithin(screen, second, x, row(1), width, foreground)
	if third != "" {
		a.drawTextWithin(screen, third, x, row(2), width, foreground)
	}
	// 原版的下方框是文字區，鍵位提示也放這裡——重製版的頁尾原本佔掉畫面最底下 43 列，
	// 那塊在原版的框裡是訊息框的一部分。框只有 269 寬，比原本的整行窄，所以鍵位提示
	// 要能折成兩列；用 FitText 裁只會無聲吃掉最後幾個鍵。
	a.drawWrappedTextWithin(screen, footerLabel(a.labels, a.state), x, row(3), width, 2, group.Palette[group.UI.Accent])
	a.drawWrappedTextWithin(screen, footerHelp(a.labels, a.state, group), x, row(5), width, 2, foreground)
}

// handLabel names what the character is holding, or says the hand is empty.
func (a *app) handLabel() string {
	if a.state.HandItem == 0 {
		return a.lookup("ui.hand_empty")
	}
	if name, ok := game.ItemName(a.state.HandItem); ok {
		return name
	}
	return a.lookup("ui.hand_empty")
}

func headingKey(heading game.Heading) string {
	switch heading {
	case game.HeadingSouth:
		return "ui.heading.south"
	case game.HeadingWest:
		return "ui.heading.west"
	case game.HeadingEast:
		return "ui.heading.east"
	default:
		return "ui.heading.north"
	}
}

// drawCompass is the status window's compass: a ring with the needle on the
// direction the character last walked. The original draws one; which way it
// points is the remake's own reading of "the direction you are facing".
func (a *app) drawCompass(screen *ebiten.Image, group theme.Group, window floatWindow) {
	// 指南針畫在浮動視窗右下角——讀數列在上面，這裡是視窗裡唯一空著的地方。
	const radius = 9
	centreX := float32(window.x + window.w - floatCompassInset)
	centreY := float32(window.bottomRowY() + radius - 2)
	foreground := group.Palette[group.UI.Foreground]
	accent := group.Palette[group.UI.Accent]
	uiStrokeCircle(screen, centreX, centreY, radius, 1, foreground, false)
	dx, dy := 0, -1
	switch a.state.Heading {
	case game.HeadingSouth:
		dx, dy = 0, 1
	case game.HeadingWest:
		dx, dy = -1, 0
	case game.HeadingEast:
		dx, dy = 1, 0
	}
	uiStrokeLine(screen, centreX, centreY,
		centreX+float32(dx*(radius-1)), centreY+float32(dy*(radius-1)), 2, accent, false)
}

func foodLabelKey(state game.State) string {
	if state.FoodItemKey != "" {
		return state.FoodItemKey
	}
	return "apple2.ship.food"
}

func (a *app) drawFooter(screen *ebiten.Image, group theme.Group) {
	uiFillRect(screen, 0, 157, logicalWidth, 43, group.Palette[group.UI.Foreground], false)
	uiFillRect(screen, 0, 157, logicalWidth, 2, group.Palette[group.UI.Accent], false)
	footer := footerLabel(a.labels, a.state)
	a.drawTextWithin(screen, footer, 5, 162, 310, group.Palette[group.UI.Background])
	a.drawWrappedTextWithin(screen, footerHelp(a.labels, a.state, group), 5, footerHelpY, footerHelpWidth, footerHelpMaxLines, group.Palette[group.UI.Background])
}

func footerHelp(labels catalog.Catalog, state game.State, group theme.Group) string {
	screen := state.Screen
	preset := game.PresentationOf(state)
	lookName := lookupLabel(labels, preset.DisplayNameKey, string(preset.ID))
	if isFailureTerminal(state) {
		return fmt.Sprintf(lookupLabel(labels, "ui.footer.failure", "S: save  Q: quit  Look: %s"), lookName)
	}
	rightAction := lookupLabel(labels, "ui.back", "返回")
	if screen == game.ScreenShipboard {
		rightAction = lookupLabel(labels, "ui.character_status", "角色狀態")
	}
	// 這行畫在原版的下方訊息框裡，只有 269 像素寬、折兩列。呈現的完整名稱要留著
	// （玩家要知道現在是哪一套），所以省的是動詞不是名稱。
	return fmt.Sprintf(lookupLabel(labels, "ui.footer.normal", "Mouse: buttons/map  Right: %s  F8: %s"),
		rightAction, lookName)
}

// drawText puts one line on the text layer at the window's resolution.
//
// Callers work in the 320x200 layout grid, so coordinates coming from the ui
// buffer are multiplied up; the few places that already draw at window
// resolution pass their own coordinates and are left alone. Measurement stays
// on the 8-pixel face, so what fits on a line does not change with the scale.
func (a *app) drawText(screen *ebiten.Image, value string, x, y int, c color.RGBA) {
	if a.textLayer == nil {
		ebitentext.Draw(screen, value, a.face, x, y+8, c)
		return
	}
	scale := a.viewScale()
	face := a.displayFace()
	if screen == a.uiLayer {
		ebitentext.Draw(a.textLayer, value, face, x*scale+a.originX, (y+8)*scale+a.originY, c)
		return
	}
	ebitentext.Draw(a.textLayer, value, face, x+a.originX, y+8*scale+a.originY, c)
}

func (a *app) drawTextWithin(screen *ebiten.Image, value string, x, y, maxWidth int, c color.RGBA) {
	if a.usesHD() && screen == a.uiLayer {
		a.drawHDTextWithin(value, x, y, maxWidth, c)
		return
	}
	a.drawText(screen, localization.FitText(a.face, value, maxWidth), x, y, c)
}

func (a *app) drawWrappedTextWithin(screen *ebiten.Image, value string, x, y, maxWidth, maxLines int, c color.RGBA) {
	for index, line := range localization.WrapText(a.face, value, maxWidth, maxLines) {
		a.drawText(screen, line, x, y+index*wrappedRowStep, c)
	}
}

// navigationSelectorLine renders one level of the route selector as
// "label < current >", so the visible row states the current choice and shows
// tubeFareLine names the tube and what the ride to the selected destination
// costs. The tube only runs between cities of one planet, so a destination it
// cannot reach shows the label alone rather than a made-up figure.
func (a *app) tubeFareLine() string {
	fare, ok := a.reducer.TubeFare(a.state, a.state.NavigationTargetID)
	if !ok {
		return a.lookup("ui.action.tube")
	}
	// 這一列在 68 寬的左方框裡，「搭乘領航台電車」放不下，用短名。
	return fmt.Sprintf("%s：%d", a.lookup("ui.tube_short"), fare)
}

// tubeStationLines is do_tube_station for the panel layouts: the list page is
// the prompt and one row per city with its fare; the confirmation page is the
// four rows the window shows and the TRANSPORT row.
func (a *app) tubeStationLines() []string {
	if a.state.TubePage == game.TubePageConfirm {
		selection, ok := a.reducer.TubeSelected(a.state)
		if !ok {
			return []string{a.lookup("ui.message.tube_no_destination")}
		}
		lines := []string{
			a.lookup("ui.tube.destination"),
			a.lookup("ui.tube.city") + selection.CityName,
			a.lookup("ui.tube.planet") + selection.PlanetName,
			a.lookup("ui.tube.system") + selection.SystemName,
			a.lookup("ui.tube.cost") + game.FormatCredits(selection.Destination.Fare) + a.lookup("ui.tube.credits"),
		}
		if selection.Affordable {
			return append(lines, a.lookup("ui.tube.transport"))
		}
		return append(lines, a.lookup("ui.tube.insufficient"))
	}
	lines := []string{a.lookup("ui.tube.select_1") + a.lookup("ui.tube.select_2")}
	for _, destination := range a.reducer.TubeDestinations(a.state) {
		lines = append(lines, fmt.Sprintf("%s：%s", destination.Name, game.FormatCredits(destination.Fare)))
	}
	if len(lines) == 1 {
		lines = append(lines, a.lookup("ui.message.tube_no_destination"))
	}
	return lines
}

// navigationSelectorLine renders one level of the route selector as
// "label < current >", so the visible row states the current choice and shows
// that both directions are clickable. It shows the source name rather than the
// localized one: the row label already says which level this is, and the fully
// localized route stays on the target line above.
func (a *app) navigationSelectorLine(labelKey, name string) string {
	return fmt.Sprintf("%s < %s >", a.lookup(labelKey), name)
}

// partsCounterLines describes the warehouse's parts counter: what is on offer,
// what it costs, how many are aboard, and what the ship's stores hold.
func (a *app) partsCounterLines() []string {
	part, ok := a.state.SelectedPart()
	if !ok {
		return []string{a.lookup("ui.parts_counter")}
	}
	name, _ := game.ItemName(part)
	price, _ := a.reducer.ShopItemPrice(a.state, part)
	lines := []string{
		a.lookup("ui.parts_counter"),
		a.lookup("ui.item." + itemKeySuffix(name)),
		fmt.Sprintf("%s：%d", a.lookup("ui.parts_price"), price),
		fmt.Sprintf("%s：%d", a.lookup("ui.parts_carried"), game.CarriedPartCount(a.state, part)),
		fmt.Sprintf("%s：%s  %s：%s", a.lookup("ui.cash"), game.FormatCredits(a.state.Cash), a.lookup("ui.bank"), game.FormatCredits(a.state.Bank)),
	}
	stored := make([]string, 0, len(a.state.Stores))
	for _, item := range a.state.Stores {
		if item == 0 {
			continue
		}
		storeName, ok := game.ItemName(item)
		if !ok {
			continue
		}
		stored = append(stored, a.lookup("ui.item."+itemKeySuffix(storeName)))
	}
	if len(stored) > 0 {
		// 物資最多十二件，接成一行遠比任何一個框寬，所以在這裡就折好。
		lines = append(lines, localization.WrapText(a.face,
			a.lookup("ui.ship_stores")+"："+strings.Join(stored, "、"), floatTextW, 3)...)
	}
	return lines
}

// itemKeySuffix turns a source item name into its catalog key suffix, matching
// how the catalog was generated from the evidence document.
func itemKeySuffix(name string) string {
	out := make([]rune, 0, len(name))
	dash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
			dash = false
		default:
			if !dash {
				out = append(out, '-')
				dash = true
			}
		}
	}
	return strings.Trim(string(out), "-")
}

// routeCostLine shows what the selected route costs from where the ship is.
// Every figure on it is the original's — the jump distance from the star
// system coordinates, the fuel from the flight loop's three-phase cycle, the
// clock from do_in_system's own ETA — and all three move as the ship moves.
func (a *app) routeCostLine() string {
	target, _ := a.reducer.World.Destination(a.state.NavigationTargetID)
	cost, ok := a.reducer.RouteCost(a.state)
	if !ok {
		return fmt.Sprintf("%s：%s", a.lookup("ui.route_cost"), target.Label)
	}
	line := fmt.Sprintf("%s：%s %d／%s %d%s", a.lookup("ui.route_cost"),
		a.lookup("ui.fuel"), cost.FuelCost,
		a.lookup("ui.time"), cost.TotalMinutes, a.lookup("ui.minutes"))
	if cost.JumpDistance > 0 {
		line += fmt.Sprintf("  %s %d／%d", a.lookup("ui.route_jump"), cost.JumpDistance, a.state.JumpRange)
	}
	return line
}

func (a *app) lookup(key string) string {
	if value, ok := a.labels.Lookup(key); ok {
		return value
	}
	return "[" + key + "]"
}

func (a *app) messageLine() string {
	text := a.lookup(a.state.MessageKey)
	if a.state.MessageKey == "ui.message.refuel_eta" && a.state.FuelRescuePending {
		return fmt.Sprintf("%s %d", text, a.state.FuelRescueDue)
	}
	return text
}

func screenLabel(labels catalog.Catalog, screen game.Screen) string {
	key := "ui.shipboard"
	switch screen {
	case game.ScreenTitle:
		key = "ui.title"
	case game.ScreenCharacterLibrary:
		key = "ui.character_library"
	case game.ScreenCharacterCreation:
		key = "ui.character_creation"
	case game.ScreenAttributeAllocation:
		key = "ui.attribute_allocation"
	case game.ScreenCharacterStatus:
		key = "ui.character_status"
	case game.ScreenGround:
		key = "ui.ground"
	case game.ScreenFuelPurchase:
		key = "ui.fuel_purchase"
	case game.ScreenDepot:
		key = "ui.depot_title"
	case game.ScreenFTLBoost:
		key = "ui.ftl_title"
	case game.ScreenTrade:
		key = "ui.trade_exchange"
	case game.ScreenParts:
		key = "ui.parts_counter"
	case game.ScreenNavigation:
		key = "ui.navigation"
	case game.ScreenFlight:
		key = "ui.flight"
	case game.ScreenUniteller:
		key = "ui.uniteller"
	case game.ScreenTactical:
		key = "ui.tactical"
	case game.ScreenEngineering:
		key = "ui.engineering"
	case game.ScreenEquipment:
		key = "ui.equipment"
	case game.ScreenCargo:
		key = "ui.cargo_hold"
	case game.ScreenDialog:
		key = "ui.dialog"
	case game.ScreenOutcome:
		key = "ui.outcome"
	}
	if value, ok := labels.Lookup(key); ok {
		return value
	}
	return string(screen)
}

func footerLabel(labels catalog.Catalog, state game.State) string {
	if state.Sleep.Active && state.Screen == game.ScreenCharacterStatus {
		return lookupLabel(labels, "ui.help.sleep", "")
	}
	screen := state.Screen
	if screen == game.ScreenDepot {
		return lookupLabel(labels, "ui.help.depot", "")
	}
	if screen == game.ScreenFTLBoost {
		return lookupLabel(labels, "ui.help.ftl", "")
	}
	if screen == game.ScreenFuelPurchase {
		return lookupLabel(labels, "ui.help.fuel_purchase", "")
	}
	if isFailureTerminal(state) {
		return lookupLabel(labels, "ui.outcome_failure_terminal", "ui.outcome_failure_terminal")
	}
	if screen == game.ScreenTitle {
		return "P：" + lookupLabel(labels, "ui.play_sundog", "開始遊玩") +
			"　C：" + lookupLabel(labels, "ui.select_game", "角色保存庫") +
			"　L：" + lookupLabel(labels, "ui.load_game", "載入遊戲") +
			"　O：" + lookupLabel(labels, "ui.options", "選項")
	}
	if screen == game.ScreenCharacterLibrary {
		return "N " + lookupLabel(labels, "ui.library_key_new", "新遊戲") +
			"　M " + lookupLabel(labels, "ui.library_key_make", "建庫") +
			"　S " + lookupLabel(labels, "ui.library_key_store", "儲存") +
			"　L " + lookupLabel(labels, "ui.library_key_load", "載入") +
			"　D " + lookupLabel(labels, "ui.library_key_delete", "刪除") +
			"　" + lookupLabel(labels, "ui.library_key_pick", "1–8 選槽")
	}
	if screen == game.ScreenCharacterCreation {
		return lookupLabel(labels, "ui.help.character_creation", "ui.help.character_creation")
	}
	if screen == game.ScreenNavigation {
		previous := "上一個目的地"
		next := "下一個目的地"
		navigate := "確認導航"
		if value, ok := labels.Lookup("ui.destination_previous"); ok {
			previous = value
		}
		if value, ok := labels.Lookup("ui.destination_next"); ok {
			next = value
		}
		if value, ok := labels.Lookup("ui.navigate"); ok {
			navigate = value
		}
		system := lookupLabel(labels, "ui.system", "星系")
		planet := lookupLabel(labels, "ui.planet", "行星")
		return "[ ] " + system + "　↑↓ " + planet + "　←→ " + previous + "／" + next +
			"　ENTER：" + navigate
	}
	if screen == game.ScreenDialog {
		return lookupLabel(labels, "ui.help.dialog_footer", "ui.help.dialog_footer")
	}
	if screen == game.ScreenUniteller {
		deposit := "存入"
		withdraw := "提領"
		transfer := "轉帳"
		if value, ok := labels.Lookup("ui.deposit"); ok {
			deposit = value
		}
		if value, ok := labels.Lookup("ui.withdraw"); ok {
			withdraw = value
		}
		if value, ok := labels.Lookup("ui.transfer"); ok {
			transfer = value
		}
		fine := lookupLabel(labels, "ui.action.pay_fine", "繳罰金")
		tube := lookupLabel(labels, "ui.action.tube", "搭電車")
		return "D：" + deposit + "　W：" + withdraw + "　T：" + transfer +
			"　F：" + fine + "　U：" + tube
	}
	if screen == game.ScreenEngineering {
		return lookupLabel(labels, "ui.help.engineering_shortcuts", "←／→：選系統　E：修系統　W：武器　S：護盾　R：船體　I：設備")
	}
	if screen == game.ScreenEquipment {
		return lookupLabel(labels, "ui.help.equipment_back", "ESC／右鍵：返回")
	}
	if screen == game.ScreenGround {
		return lookupLabel(labels, "ui.help.ground_shortcuts", "T：交易　U：銀行　R：燃料　P：物資　K：冷凍　V：交付　O：輔助燃料")
	}
	keys := map[game.Screen]string{
		game.ScreenTitle:            "ui.play_sundog",
		game.ScreenCharacterLibrary: "ui.new_game",
		game.ScreenCharacterStatus:  "ui.rest",
		game.ScreenShipboard:        "ui.ground",
		game.ScreenGround:           "ui.trade_exchange",
		game.ScreenTrade:            "ui.buy",
		game.ScreenParts:            "ui.buy_part",
		game.ScreenNavigation:       "ui.navigate",
		game.ScreenUniteller:        "ui.deposit",
		game.ScreenTactical:         "ui.fire",
		game.ScreenEngineering:      "ui.repair",
		game.ScreenEquipment:        "ui.equipment",
		game.ScreenCargo:            "ui.load_cargo",
		game.ScreenOutcome:          "ui.outcome_return",
	}
	key := keys[screen]
	if value, ok := labels.Lookup(key); ok {
		return fmt.Sprintf(lookupLabel(labels, "ui.primary_action_hint", "%s"), value)
	}
	return fmt.Sprintf(lookupLabel(labels, "ui.primary_action_hint", "%s"), lookupLabel(labels, "ui.action.default", "ui.action.default"))
}

func isFailureTerminal(state game.State) bool {
	return state.Screen == game.ScreenOutcome && state.OutcomeKey == "ui.outcome.failure"
}

func lookupLabel(labels catalog.Catalog, key, fallback string) string {
	if value, ok := labels.Lookup(key); ok {
		return value
	}
	return fallback
}

// Layout renders at the window's own resolution rather than at a fixed buffer
// size, so a resized window gains real pixels instead of a stretched picture.
func (a *app) Layout(outsideWidth, outsideHeight int) (int, int) {
	if outsideWidth <= 0 || outsideHeight <= 0 {
		// Headless runs and tests never get a real window size.
		a.setViewport(displayWidth, displayHeight)
		return displayWidth, displayHeight
	}
	a.setViewport(outsideWidth, outsideHeight)
	return outsideWidth, outsideHeight
}

// setViewport refits the frame after the window changed size. The text face and
// the text layer both depend on the size, so both are dropped when it moves.
func (a *app) setViewport(width, height int) {
	if a.viewW == width && a.viewH == height {
		return
	}
	a.viewW, a.viewH = width, height
	a.scale, a.originX, a.originY = viewportFor(width, height)
	a.textLayer = nil
}

// viewScale is the running multiple. It falls back to the startup value so the
// coordinate helpers stay usable before the first Layout call.
func (a *app) viewScale() int {
	if a.scale < 1 {
		return displayScale
	}
	return a.scale
}

// displayFace returns the text face for the current scale, building each one
// once. Sizes repeat as the window is dragged back and forth.
func (a *app) displayFace() font.Face {
	scale := a.viewScale()
	if face, ok := a.faces[scale]; ok {
		return face
	}
	if a.newFace == nil {
		return a.face
	}
	if a.faces == nil {
		a.faces = map[int]font.Face{}
	}
	face := a.newFace(8 * scale)
	a.faces[scale] = face
	return face
}

// dialogPanelLines wraps the conversation into the 320×200 panel. The other
// party's line is the long one, so it takes the wrapped rows and the player's
// own choice sits on the fixed rows the hotspots point at.
func (a *app) dialogPanelLines() []string {
	line, err := a.reducer.DialogLine(a.state)
	if err != nil {
		line = a.lookup("ui.message.dialog_unavailable")
	}
	rows := localization.WrapText(a.face, line, floatTextW, 4)
	for len(rows) < 4 {
		rows = append(rows, "")
	}
	said := a.state.DialogSaid
	if said == "" {
		said = "—"
	}
	verb := "—"
	if verbs := a.reducer.PlayerVerbs(); a.state.DialogVerb >= 0 && a.state.DialogVerb < len(verbs) {
		verb = verbs[a.state.DialogVerb]
	}
	return append(rows[:4],
		fmt.Sprintf("%s：%s", a.lookup("ui.dialog_said"), said),
		fmt.Sprintf("%s < %s >", a.lookup("ui.dialog_verb"), verb),
		a.lookup("ui.dialog_say"),
	)
}

// interiorPanelLines is what the room shows: where the player is standing and
// what is within reach. The original draws the room; this names it.
func (a *app) interiorPanelLines() []string {
	interior := a.state.Interior
	place := a.lookup("ui.interior.door")
	switch {
	case interior.AtCounter():
		place = a.lookup("ui.interior.counter")
	case interior.AtSlots():
		place = a.lookup("ui.interior.slots")
	case interior.BoothIndex() >= 0:
		place = a.lookup("ui.interior.booth")
	case !interior.AtDoor():
		place = "—"
	}
	lines := []string{
		a.lookup(game.InteriorLabelKey(interior.Kind)),
		fmt.Sprintf("%d, %d　%s", interior.Position.X, interior.Position.Y, place),
		fmt.Sprintf("%s：%d／%d", a.lookup("ui.interior.customers"),
			interior.Customers, interior.CustomerCap),
	}
	if interior.AtCounter() {
		lines = append(lines, a.lookup("ui.message.at_counter"))
	}
	if interior.AtSlots() {
		lines = append(lines, a.lookup("ui.message.at_slots"))
	}
	if interior.BoothIndex() >= 0 {
		lines = append(lines, a.lookup("ui.message.at_booth"))
	}
	return lines
}

// slotsPanelLines shows the stake, the three faces and what the last spin paid.
func (a *app) slotsPanelLines() []string {
	machine := a.state.Slots
	lines := []string{fmt.Sprintf("%s：%d Cr", a.lookup("ui.slots.bet"), machine.Bet)}
	if !machine.Spun {
		return append(lines, a.lookup("ui.slots.spin"))
	}
	faces := make([]string, 0, len(machine.Reels))
	for _, face := range machine.Reels {
		faces = append(faces, a.lookup(game.SlotFaceNames[face]))
	}
	lines = append(lines, strings.Join(faces, "　"))
	lines = append(lines, fmt.Sprintf("%s：%d Cr", a.lookup("ui.slots.payout"), machine.Payout))
	return append(lines, a.lookup("ui.slots.spin"))
}

// arenaPanelLines shows the fight: what the player is holding, what is left of
// them, and how many opponents are still up.
func (a *app) arenaPanelLines() []string {
	arena := a.state.Arena
	weapon := a.lookup("ui.arena.barehanded")
	if name, ok := game.ItemName(arena.Gun); ok {
		weapon = name
	}
	return []string{
		fmt.Sprintf("%s：%s", a.lookup("ui.arena.weapon"), weapon),
		fmt.Sprintf("%s：%d", a.lookup("ui.arena.hitpoints"), arena.HitPoints),
		fmt.Sprintf("%s：%d", a.lookup("ui.arena.shield"), arena.ShieldCharges),
		fmt.Sprintf("%s：%d／%d", a.lookup("ui.arena.opponents"), game.ArenaStanding(arena), len(arena.Thugs)),
	}
}

// scorePanelLines is the original's own tally, in its own order. The no-death
// line is shown because the original shows it, and it is not in the total
// because the original does not add it either.
func (a *app) scorePanelLines() []string {
	score := game.GameScore(a.state)
	lines := []string{
		fmt.Sprintf("%s：%d", a.lookup("ui.score.banville_cargo"), score.BanvilleCargo),
		fmt.Sprintf("%s：-%d", a.lookup("ui.score.time_penalty"), score.TimePenalty),
	}
	if score.NoDeathBonus > 0 {
		lines = append(lines, fmt.Sprintf("%s：%d", a.lookup("ui.score.no_death_bonus"), score.NoDeathBonus))
	} else {
		lines = append(lines, fmt.Sprintf("%s：-%d", a.lookup("ui.score.restart_penalty"), score.RestartPenalty))
	}
	return append(lines, fmt.Sprintf("%s：%d", a.lookup("ui.score.total"), score.Total))
}

// drawSkyline puts the six buildings init_city generated on the horizon.
// Count, spacing and shapes are the original's: each building is seven bars
// whose heights come from the disk's own 8-shape table (game.SkylineHeights,
// rule landing.skyline-column-heights). The original paints the strip six
// pixels tall at most inside its viewport; the remake's landing overlay fills
// the frame, so one height unit here is a taller remake-owned bar.
func (a *app) drawSkyline(horizon int, group theme.Group) {
	const (
		unitHeight = 6
		unitWidth  = 3
	)
	ink := group.Palette[group.UI.Foreground]
	for _, building := range a.state.Landing.Skyline {
		profile := game.SkylineHeights[building.Style%len(game.SkylineHeights)]
		for column, units := range profile {
			height := units * unitHeight
			top := horizon - height
			if top < 0 {
				top = 0
			}
			if top >= horizon {
				continue
			}
			uiFillRect(a.uiLayer, float32(building.X+column*unitWidth), float32(top),
				float32(unitWidth), float32(horizon-top), ink, false)
		}
	}
}

// drawLandingOverlay is XLANDFX on screen: a horizon line walking up or down
// the display with a starfield above it. The band the stars fill is horizon - 6
// and nothing is drawn outside 6..117, which is do_stars' own gate. The stars
// are placed by a fixed hash of their index, so the field stays put from frame
// to frame instead of flickering.
func (a *app) drawLandingOverlay(screen *ebiten.Image, group theme.Group) {
	background := group.Palette[group.UI.Background]
	foreground := group.Palette[group.UI.Foreground]
	a.uiLayer.Fill(background)

	horizon := a.state.Landing.Horizon
	if band, ok := a.state.Landing.StarBand(); ok && band > 0 {
		top := horizon - band
		if top < 0 {
			top = 0
		}
		for index := 0; index < 96; index++ {
			mixed := uint32(index)*2654435761 + 2246822519
			x := int(mixed % uint32(logicalWidth))
			y := top + int((mixed>>16)%uint32(band))
			if y < 0 || y >= logicalHeight {
				continue
			}
			uiFillRect(a.uiLayer, float32(x), float32(y), 1, 1, foreground, false)
		}
	}
	if horizon >= 0 && horizon < logicalHeight {
		a.drawSkyline(horizon, group)
		uiFillRect(a.uiLayer, 0, float32(horizon), logicalWidth, 1, foreground, false)
		uiFillRect(a.uiLayer, 0, float32(horizon+1), logicalWidth,
			float32(logicalHeight-horizon-1), group.Palette[group.UI.Panel], false)
	}
	a.drawTextWithin(a.uiLayer, a.lookup("ui.message.landing"), 12, statusStripY+1, logicalWidth-24, foreground)

	screen.Fill(background)
	options := &ebiten.DrawImageOptions{}
	options.Filter = ebiten.FilterNearest
	options.GeoM.Scale(float64(a.viewScale()), float64(a.viewScale()))
	options.GeoM.Translate(float64(a.originX), float64(a.originY))
	screen.DrawImage(a.uiLayer, options)
}
