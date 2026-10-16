package main

import (
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/localization"
	"github.com/wicanr2/sundog-remake/internal/prefs"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/theme"
)

// The options card is the F1 help card's sibling: a reading-and-choosing mode
// over the frame that never reaches the reducer. What it changes is the
// presentation the state carries (chrome, tileset) and the app's own disk;
// what it persists is the settings file, so the choice outlives this game.
//
// The original has no options screen. This one is remake-owned by that fact,
// not because anything remains to be read from the disk.

type optionsCard struct {
	// row is the highlighted line; see the optionsRow constants.
	row int
	// draft is the disk path as typed so far; it becomes the disk on ENTER.
	draft string
	// statusKey is the catalog key of the last outcome line, or "".
	statusKey string
	// statusDetail is appended to the status line: a path, an error.
	statusDetail string
	// diskOnly is the fail-closed first-run gate. Until a valid disk opens,
	// only the path and detection rows exist and the card cannot be dismissed.
	diskOnly bool
	// lookOnly is the other first-run gate: the player picks a look before the
	// title. It is not fail-closed — every choice is valid — but it cannot be
	// dismissed without choosing one, because "no answer" is what the settings
	// file already means and the card would come back.
	lookOnly bool
}

const (
	optionsRowLanguage     = 0
	optionsRowPresentation = 1
	optionsRowDisk         = 2
	optionsRowDetect       = 3
	optionsRowCount        = 4
	optionsRowY0           = 40
	optionsRowStep         = 14
	optionsStatusY         = 130
	optionsHintY           = 146
	optionsMarkerX         = 14
	optionsLabelX          = 26
	optionsValueX          = 112
	optionsValueW          = 194
)

func optionsRowY(row int) int { return optionsRowY0 + row*optionsRowStep }

func (a *app) openOptions() {
	a.showOptions = true
	a.options = optionsCard{draft: a.atariDiskPath}
}

func (a *app) openDiskBootstrap() {
	a.showOptions = true
	a.showHelp = false
	a.options = optionsCard{row: optionsRowDisk, draft: a.atariDiskPath, diskOnly: true}
}

// openLookBootstrap asks for a look on the first run. It comes after the disk
// gate because a disk is what makes the Atari ST preset show anything.
func (a *app) openLookBootstrap() {
	a.showOptions = true
	a.showHelp = false
	// row indexes the preset list here, not the options rows: the card opens on
	// the first preset, which is also the default.
	a.options = optionsCard{draft: a.atariDiskPath, lookOnly: true}
}

// closeOptions writes the settings on the way out, so a card opened and closed
// without touching anything still records the current choice — which is also
// how a first-run disk detection becomes the remembered default.
func (a *app) closeOptions() {
	if (a.options.diskOnly || a.state.Screen == game.ScreenTitle) && a.originalDisk == nil {
		a.openDiskBootstrap()
		return
	}
	a.showOptions = false
	a.lookChosen = true
	a.savePreferences()
}

// updateOptions is Update while the card is up. Like the help card it swallows
// the frame's input: the key that closes it must not reach the game behind.
func (a *app) updateOptions() {
	if a.options.lookOnly {
		a.updateLookBootstrap()
		return
	}
	if !a.options.diskOnly && (inpututil.IsKeyJustPressed(ebiten.KeyF2) ||
		inpututil.IsKeyJustPressed(ebiten.KeyEscape) ||
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)) {
		a.closeOptions()
		return
	}
	card := &a.options
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		if card.diskOnly {
			card.row = optionsRowDisk + optionsRowDetect - card.row
		} else {
			card.row = (card.row + optionsRowCount - 1) % optionsRowCount
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		if card.diskOnly {
			card.row = optionsRowDisk + optionsRowDetect - card.row
		} else {
			card.row = (card.row + 1) % optionsRowCount
		}
	}
	left := inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft)
	right := inpututil.IsKeyJustPressed(ebiten.KeyArrowRight)
	confirm := inpututil.IsKeyJustPressed(ebiten.KeyEnter)
	switch card.row {
	case optionsRowLanguage:
		if left {
			a.cycleLocale(-1)
		} else if right || confirm || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
			a.cycleLocale(1)
		}
	case optionsRowPresentation:
		if left {
			a.cyclePresentation(-1)
		} else if right || confirm || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
			a.cyclePresentation(1)
		}
	case optionsRowDisk:
		// The path row types: every printable key is part of the path, so
		// none of them is a command here. Space is a character a path may hold.
		if chars := ebiten.InputChars(); len(chars) > 0 {
			card.draft += string(chars)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && card.draft != "" {
			runes := []rune(card.draft)
			card.draft = string(runes[:len(runes)-1])
		}
		if confirm {
			a.applyDisk(card.draft)
		}
	case optionsRowDetect:
		if confirm || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
			a.detectDisk()
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		if row, ok := a.optionsRowAt(x, y); ok {
			a.clickOptionsRow(row)
		}
	}
}

// optionsRowAt maps a window pixel to a card row.
func (a *app) optionsRowAt(x, y int) (int, bool) {
	x, y = a.toLayout(x, y)
	if x < 8 || x >= logicalWidth-8 {
		return 0, false
	}
	if a.options.diskOnly {
		switch {
		case y >= 78 && y < 96:
			return optionsRowDisk, true
		case y >= 100 && y < 118:
			return optionsRowDetect, true
		default:
			return 0, false
		}
	}
	if a.options.lookOnly {
		if row, ok := lookBootstrapRowAt(y); ok {
			return row, true
		}
		return 0, false
	}
	first, last := 0, optionsRowCount-1
	for row := first; row <= last; row++ {
		top := optionsRowY(row) - 4
		if y >= top && y < top+optionsRowStep {
			return row, true
		}
	}
	return 0, false
}

// clickOptionsRow is a click's meaning: a value row advances, the path row
// only takes the highlight (typing does the rest), detect runs.
func (a *app) clickOptionsRow(row int) {
	a.options.row = row
	switch row {
	case optionsRowLanguage:
		a.cycleLocale(1)
	case optionsRowPresentation:
		a.cyclePresentation(1)
	case optionsRowDetect:
		a.detectDisk()
	}
}

func (a *app) cycleLocale(delta int) {
	locales := localization.Locales()
	index := 0
	for i, locale := range locales {
		if locale == a.locale {
			index = i
			break
		}
	}
	if a.applyLocale(locales[(index+delta+len(locales))%len(locales)]) {
		a.options.statusKey = "ui.options.language_changed"
		a.savePreferences()
	}
}

// cyclePresentation moves along the preset list by delta. It writes the state
// directly rather than through the reducer: the reducer's cycle only goes
// forward, and the card is presentation — but the state still carries the
// choice, so a save remembers it and every draw reads one place.
func (a *app) cyclePresentation(delta int) {
	presets := theme.Presentations()
	current := game.PresentationOf(a.state)
	index := 0
	for i, preset := range presets {
		if preset.ID == current.ID {
			index = i
		}
	}
	a.state = game.WithPresentation(a.state, presets[(index+delta+len(presets))%len(presets)])
	a.state.Saved = false
	a.state.MessageKey = "ui.message.presentation_changed"
	a.savePreferences()
}

// applyDisk makes path the player's disk. The regular options card may clear
// it, but the title bootstrap refuses an empty path; an invalid disk never
// replaces the current one.
func (a *app) applyDisk(path string) bool {
	if path == "" {
		if a.options.diskOnly {
			a.options.statusKey, a.options.statusDetail = "ui.options.disk_none", ""
			return false
		}
		a.useDisk(nil)
		a.options.statusKey, a.options.statusDetail = "ui.options.disk_cleared", ""
		a.savePreferences()
		return true
	}
	disk, err := originalmedia.Open(path)
	if err != nil {
		a.options.statusKey, a.options.statusDetail = "ui.options.disk_failed", err.Error()
		return false
	}
	a.useDisk(disk)
	a.options.statusKey, a.options.statusDetail = "ui.options.disk_loaded", disk.Path()
	a.savePreferences()
	if a.options.diskOnly {
		a.showOptions = false
		a.enterOriginalTitle()
	}
	return true
}

// detectDisk tries every .st file in the usual places and keeps the first that
// opens as a SunDog disk.
func (a *app) detectDisk() bool {
	for _, candidate := range prefs.DiskCandidates() {
		disk, err := originalmedia.Open(candidate)
		if err != nil {
			continue
		}
		a.useDisk(disk)
		a.options.draft = disk.Path()
		a.options.statusKey, a.options.statusDetail = "ui.options.disk_loaded", disk.Path()
		a.savePreferences()
		if a.options.diskOnly {
			a.showOptions = false
			a.enterOriginalTitle()
		}
		return true
	}
	a.options.statusKey, a.options.statusDetail = "ui.options.disk_none", ""
	return false
}

// useDisk swaps the original disk and drops everything decoded from the old
// one, so the next frame decodes from the new. nil puts the disk away.
func (a *app) useDisk(disk *originalmedia.Disk) {
	a.originalDisk = disk
	a.originalTiles = nil
	a.itemIcons, a.itemIconsRead = nil, false
	a.shipInterior, a.shipInteriorRead = [2]*ebiten.Image{}, false
	a.groundSprites, a.groundSpritesRead = nil, false
	a.interiorBackdrops = nil
	a.originalTitleArt, a.originalTitleRead = nil, false
	a.stFrameImage, a.stFrameRead = nil, false
	a.stFontStrip, a.stFontRead, a.stBackdrops = nil, false, nil
	a.stGroundPattern, a.stGroundPatternRead = nil, false
	a.atariDiskPath = ""
	if disk != nil {
		a.originalTiles = map[int]*originalTileCache{}
		a.atariDiskPath = disk.Path()
		log.Printf("地表圖塊改用原版磁碟：%s", disk.Path())
	}
}

// preferences is the settings file's view of the app right now.
func (a *app) preferences() prefs.Preferences {
	preset := game.PresentationOf(a.state)
	presentation := ""
	if a.lookChosen {
		presentation = string(preset.ID)
	}
	return prefs.Preferences{
		Locale:       string(a.locale),
		Presentation: presentation,
		Theme:        string(preset.Chrome),
		Tileset:      string(preset.Tileset),
		Layout:       string(preset.Layout),
		AtariDisk:    a.atariDiskPath,
	}
}

// savePreferences writes the settings file. A failure is logged, not shown:
// the choice still holds for this run, and there is nothing the player can do
// about an unwritable config directory from inside the game.
func (a *app) savePreferences() {
	if a.settingsPath == "" {
		return
	}
	if err := a.preferences().Save(a.settingsPath); err != nil {
		log.Printf("settings not written: %v", err)
	}
}

func (a *app) drawOptions(screen *ebiten.Image, group theme.Group) {
	if a.options.diskOnly {
		a.drawDiskBootstrap(screen)
		return
	}
	if a.options.lookOnly {
		a.drawLookBootstrap(screen)
		return
	}
	fg, accent := group.Palette[group.UI.Foreground], group.Palette[group.UI.Accent]
	uiFillRect(screen, 8, 22, 304, 134, color.RGBA{0, 0, 0, 235}, false)
	uiFillRect(screen, 12, 26, 296, 2, accent, false)
	a.drawTextWithin(screen, a.lookup("ui.options"), 14, 32, 292, accent)

	disk := a.options.draft
	if a.options.row == optionsRowDisk {
		disk += "＿"
	} else if disk == "" {
		disk = a.lookup("ui.options.disk_unset")
	}
	rows := []struct{ label, value string }{
		{a.lookup("ui.options.language"), "◀ " + a.lookup(localization.LocaleNameKey(a.locale)) + " ▶"},
		{a.lookup("ui.options.presentation"), "◀ " + a.presentationName() + " ▶"},
		{a.lookup("ui.options.disk"), disk},
		{a.lookup("ui.options.detect"), a.lookup("ui.options.detect_where")},
	}
	for row, line := range rows {
		y := optionsRowY(row)
		if row == a.options.row {
			a.drawTextWithin(screen, "▶", optionsMarkerX, y, 12, accent)
		}
		a.drawTextWithin(screen, line.label, optionsLabelX, y, optionsValueX-optionsLabelX-4, fg)
		if line.value != "" {
			a.drawTextWithin(screen, line.value, optionsValueX, y, optionsValueW, fg)
		}
	}
	if a.originalDisk == nil && a.options.statusKey == "" {
		a.drawTextWithin(screen, a.lookup("ui.options.disk_note"), optionsLabelX, optionsRowY(optionsRowDetect)+12, 280, fg)
	} else if a.options.statusKey == "" {
		a.drawTextWithin(screen, a.lookup("ui.options.audio_note"), optionsLabelX, optionsRowY(optionsRowDetect)+12, 280, fg)
	}
	if a.options.statusKey != "" {
		status := a.lookup(a.options.statusKey)
		if a.options.statusDetail != "" {
			status += "：" + a.options.statusDetail
		}
		a.drawTextWithin(screen, status, 14, optionsStatusY, 292, fg)
	}
	a.drawTextWithin(screen, a.lookup("ui.options.hint"), 14, optionsHintY, 292, accent)
}

// drawDiskBootstrap is the only screen available without a readable original
// disk. It deliberately contains no game action and no replacement title art.
func (a *app) drawDiskBootstrap(screen *ebiten.Image) {
	frame, fg := titleFrameColour(a.titleColorStep()), color.RGBA{255, 255, 255, 255}
	screen.Fill(color.RGBA{0, 0, 0, 255})
	uiFillRect(screen, 18, 26, 284, 148, frame, false)
	uiFillRect(screen, 20, 28, 280, 144, color.RGBA{0, 0, 0, 255}, false)
	a.drawTextCentered(screen, "SunDog™ Frozen Legacy", 24, 296, 42, titleLetterColour(a.titleColorStep()))
	a.drawTextWithin(screen, a.lookup("ui.options.disk_bootstrap_note"), 30, 58, 260, fg)

	disk := a.options.draft
	if a.options.row == optionsRowDisk {
		disk += "＿"
	}
	if disk == "" {
		disk = a.lookup("ui.options.disk_unset")
	}
	if a.options.row == optionsRowDisk {
		a.drawText(screen, "▶", 28, 82, frame)
	}
	a.drawText(screen, a.lookup("ui.options.disk"), 42, 82, fg)
	a.drawTextWithin(screen, disk, 118, 82, 172, fg)
	if a.options.row == optionsRowDetect {
		a.drawText(screen, "▶", 28, 104, frame)
	}
	a.drawText(screen, a.lookup("ui.options.detect"), 42, 104, fg)
	a.drawTextWithin(screen, a.lookup("ui.options.detect_where"), 118, 104, 172, fg)
	if a.options.statusKey != "" {
		status := a.lookup(a.options.statusKey)
		if a.options.statusDetail != "" {
			status += "：" + a.options.statusDetail
		}
		a.drawTextWithin(screen, status, 30, 130, 260, fg)
	}
	a.drawTextWithin(screen, a.lookup("ui.options.disk_bootstrap_hint"), 30, 152, 260, frame)
}

// presentationName is the current preset's name in the player's language.
func (a *app) presentationName() string {
	return a.lookup(theme.PresentationNameKey(game.PresentationOf(a.state).ID))
}

// The first-run look card: one row per preset, big enough to click.
const (
	lookRowY0   = 60
	lookRowStep = 26
	// The footer owns y=157 down, and it is drawn over the card, so every line
	// of this one has to finish above it.
	lookHintY = 138
)

// lookBootstrapRowAt maps a layout y to a preset index, or reports no row.
func lookBootstrapRowAt(y int) (int, bool) {
	for index := range theme.Presentations() {
		top := lookRowY0 + index*lookRowStep - 6
		if y >= top && y < top+lookRowStep {
			return index, true
		}
	}
	return 0, false
}

// updateLookBootstrap is the first-run look card's input. Up and down move,
// ENTER or a click takes the highlighted preset; there is no way out without
// choosing, because the settings file's "no answer" is what opened this card.
func (a *app) updateLookBootstrap() {
	presets := theme.Presentations()
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
		a.options.row = (a.options.row + len(presets) - 1) % len(presets)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
		a.options.row = (a.options.row + 1) % len(presets)
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		x, y := ebiten.CursorPosition()
		x, y = a.toLayout(x, y)
		if x >= 8 && x < logicalWidth-8 {
			if row, ok := lookBootstrapRowAt(y); ok {
				a.options.row = row
				a.chooseLook(presets[row])
				return
			}
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		a.chooseLook(presets[a.options.row%len(presets)])
	}
}

// chooseLook applies the picked preset, records that the question was asked,
// and lets the game start.
func (a *app) chooseLook(preset theme.Presentation) {
	a.state = game.WithPresentation(a.state, preset)
	a.lookChosen = true
	a.showOptions = false
	a.options = optionsCard{}
	a.savePreferences()
}

// drawLookBootstrap is the first thing a new player sees. It says what each
// preset is rather than only naming it: the names mean something to someone
// who knows both machines, and nothing to everyone else.
func (a *app) drawLookBootstrap(screen *ebiten.Image) {
	frame, fg := titleFrameColour(a.titleColorStep()), color.RGBA{255, 255, 255, 255}
	screen.Fill(color.RGBA{0, 0, 0, 255})
	uiFillRect(screen, 18, 20, 284, 136, frame, false)
	uiFillRect(screen, 20, 22, 280, 132, color.RGBA{0, 0, 0, 255}, false)
	a.drawTextWithin(screen, a.lookup("ui.options.look_bootstrap_title"), 28, 30, 264, frame)
	a.drawWrappedTextWithin(screen, a.lookup("ui.options.look_bootstrap_note"), 28, 42, 264, 1, fg)
	for index, preset := range theme.Presentations() {
		y := lookRowY0 + index*lookRowStep
		if index == a.options.row {
			a.drawTextWithin(screen, "▶", 28, y, 12, frame)
		}
		a.drawTextWithin(screen, a.lookup(preset.DisplayNameKey), 42, y, 250, fg)
		a.drawTextWithin(screen, a.lookup(preset.DisplayNameKey+".note"), 42, y+11, 250, frame)
	}
	a.drawTextWithin(screen, a.lookup("ui.options.look_bootstrap_hint"), 28, lookHintY, 264, frame)
}
