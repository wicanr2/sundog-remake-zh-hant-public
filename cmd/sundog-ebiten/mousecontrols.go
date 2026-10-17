package main

// The optional command card is a remake input adapter. It delegates to the
// same actions as the keyboard; neither its pages nor its text editor is saved.
import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	ebitentext "github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/localization"
	"github.com/wicanr2/sundog-remake/internal/theme"
)

type mouseControlCard struct {
	open      bool
	capture   bool
	system    bool
	page      int
	mode      string
	shift     bool
	directory string
	files     []mouseFile
	roots     []string
	errorKey  string
}
type mouseFile struct {
	name, path string
	directory  bool
}
type mouseControl struct {
	id, label  string
	action     game.Action
	x, y, w, h int
}

const mousePageSize = 12

func (a *app) arenaOwnsRightButton() bool {
	return a.state.Screen == game.ScreenArena && a.usesSTArena() && a.state.Arena.Active
}

func mouseRectContains(x, y, bx, by, w, h int) bool {
	return x >= bx && x < bx+w && y >= by && y < by+h
}

func (a *app) mouseContextActions() []game.Action {
	if isFailureTerminal(a.state) {
		return []game.Action{game.ActionSave}
	}
	if a.state.Landing.Phase != game.LandingIdle {
		return []game.Action{game.ActionSkipLanding}
	}
	if a.state.Sleep.Active {
		return []game.Action{game.ActionWake}
	}
	seen := map[game.Action]bool{}
	var actions []game.Action
	if a.state.Screen == game.ScreenTactical && a.state.TacticalSlewing {
		actions = append(actions, game.ActionTacticalStop)
		seen[game.ActionTacticalStop] = true
	}
	for _, key := range polledKeys {
		if action, ok := keyAction(key, a.state); ok && action != game.ActionQuit && !seen[action] {
			seen[action] = true
			actions = append(actions, action)
		}
	}
	return actions
}

func (a *app) mouseChoices() []mouseControl {
	button := func(id, key string) mouseControl {
		label := a.lookup(key)
		if id == "music" {
			label += ": " + a.musicValue()
		}
		if id == "sfx" {
			status := "ui.music.on"
			if a.sounds == nil || a.sounds.muted {
				status = "ui.music.off"
			}
			label += ": " + a.lookup(status)
		}
		return mouseControl{id: id, label: label}
	}
	if a.mouseCard.mode == "disk" {
		choices := []mouseControl{button("parent", "ui.mouse.parent")}
		for i, file := range a.mouseCard.files {
			name := file.name
			if file.directory {
				name = "[" + name + "]"
			}
			choices = append(choices, mouseControl{id: fmt.Sprintf("file:%d", i), label: name})
		}
		if len(a.mouseCard.roots) > 0 {
			choices = append(choices, button("roots", "ui.mouse.drives"))
		}
		return choices
	}
	if a.showOptions && !a.mouseCard.system {
		// The original disk and first-look gates remain owned by options.go.
		if a.options.lookOnly {
			var choices []mouseControl
			for i, p := range a.presentationChoices() {
				choices = append(choices, mouseControl{id: fmt.Sprintf("look:%d", i), label: p})
			}
			return choices
		}
		choices := []mouseControl{button("browse", "ui.mouse.browse"), button("detect", "ui.options.detect")}
		if !a.options.diskOnly {
			choices = append(choices, button("locale", "ui.options.language"), button("presentation", "ui.options.presentation"), button("music", "ui.mouse.music"), button("sfx", "ui.mouse.sfx"), button("options-close", "ui.back"))
		}
		return choices
	}
	if a.showHelp && !a.mouseCard.system {
		return []mouseControl{button("help", "ui.back")}
	}
	if a.showCredits && !a.mouseCard.system {
		return []mouseControl{button("credits-close", "ui.back")}
	}
	if a.state.Screen == game.ScreenCharacterLibrary && !a.mouseCard.system {
		// Slots remain directly clickable behind the card; expose them here as
		// well so choosing an action never depends on a hidden selection.
		var choices []mouseControl
		for i := range prototypeLibrarySlots {
			choices = append(choices, mouseControl{id: fmt.Sprintf("slot:%d", i), label: fmt.Sprintf("%d", i+1)})
		}
		for _, action := range a.mouseContextActions() {
			choices = append(choices, mouseControl{id: "action", label: a.lookup("ui.mouse.action." + string(action)), action: action})
		}
		return append(choices, mouseControl{id: "action", label: a.lookup("ui.back"), action: game.ActionBack})
	}
	if a.mouseCard.system {
		choices := []mouseControl{button("help", "ui.mouse.help"), button("music", "ui.mouse.music"), button("sfx", "ui.mouse.sfx")}
		if !isFailureTerminal(a.state) && !a.showOptions {
			choices = append(choices, button("options", "ui.options"))
		}
		if !isFailureTerminal(a.state) && !a.showOptions && !a.showHelp {
			choices = append(choices, button("presentation", "ui.options.presentation"))
			if !isMenuScreen(a.state.Screen) && a.state.Screen != game.ScreenScore {
				choices = append(choices, mouseControl{id: "action", label: a.lookup("ui.mouse.action.open-library"), action: game.ActionOpenLibrary})
			}
		}
		if !isMenuScreen(a.state.Screen) && !a.showOptions {
			choices = append(choices, mouseControl{id: "action", label: a.lookup("ui.mouse.action.save"), action: game.ActionSave})
		}
		return append(choices, mouseControl{id: "action", label: a.lookup("ui.quit"), action: game.ActionQuit})
	}
	var choices []mouseControl
	if a.state.Screen == game.ScreenCharacterCreation {
		choices = append(choices, button("name", "ui.mouse.keyboard"))
	}
	for _, action := range a.mouseContextActions() {
		choices = append(choices, mouseControl{id: "action", label: a.lookup("ui.mouse.action." + string(action)), action: action})
	}
	if a.state.HandItem != 0 && a.state.HandOrigin.Kind != "" {
		choices = append(choices, mouseControl{id: "action", label: a.lookup("ui.mouse.action.cancel-item"), action: game.ActionCancelItem})
	}
	if !isFailureTerminal(a.state) && a.state.Landing.Phase == game.LandingIdle && !a.state.Sleep.Active {
		action := game.ActionBack
		if a.state.Screen == game.ScreenShipboard || a.state.Screen == game.ScreenGround {
			action = game.ActionOpenStatus
		}
		choices = append(choices, mouseControl{id: "action", label: a.lookup("ui.mouse.action." + string(action)), action: action})
	}
	return choices
}

func (a *app) presentationChoices() []string {
	var choices []string
	for _, p := range theme.Presentations() {
		choices = append(choices, a.lookup(p.DisplayNameKey))
	}
	return choices
}

func (a *app) mouseButtons() []mouseControl {
	if !a.mouseCard.open {
		return []mouseControl{{id: "open", label: a.lookup("ui.mouse.menu"), x: 280, y: 1, w: 39, h: 14}}
	}
	buttons := []mouseControl{{id: "close", label: a.lookup("ui.mouse.close"), x: 258, y: 18, w: 42, h: 16}}
	if a.mouseCard.mode == "name" {
		letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-'"
		if !a.mouseCard.shift {
			letters = strings.ToLower(letters)
		}
		for i, r := range letters {
			buttons = append(buttons, mouseControl{id: "char:" + string(r), label: string(r), x: 22 + (i%10)*28, y: 62 + (i/10)*21, w: 26, h: 19})
		}
		for i, b := range []mouseControl{{id: "shift", label: a.lookup("ui.mouse.shift")}, {id: "space", label: a.lookup("ui.mouse.space")}, {id: "erase", label: a.lookup("ui.mouse.erase")}, {id: "name-confirm", label: a.lookup("ui.mouse.confirm")}} {
			b.x, b.y, b.w, b.h = 22+i*70, 151, 68, 21
			buttons = append(buttons, b)
		}
		return buttons
	}
	choices := a.mouseChoices()
	pages := (len(choices) + mousePageSize - 1) / mousePageSize
	if pages < 1 {
		pages = 1
	}
	if a.mouseCard.page >= pages {
		a.mouseCard.page = pages - 1
	}
	start := a.mouseCard.page * mousePageSize
	for i := start; i < len(choices) && i < start+mousePageSize; i++ {
		b := choices[i]
		j := i - start
		b.x, b.y, b.w, b.h = 22+(j%2)*140, 54+(j/2)*19, 136, 18
		buttons = append(buttons, b)
	}
	buttons = append(buttons, mouseControl{id: "prev", label: "<", x: 22, y: 170, w: 28, h: 13}, mouseControl{id: "next", label: ">", x: 270, y: 170, w: 28, h: 13})
	if a.mouseCard.mode != "disk" {
		buttons = append(buttons, mouseControl{id: "context", label: a.lookup("ui.mouse.context"), x: 22, y: 185, w: 136, h: 14}, mouseControl{id: "system", label: a.lookup("ui.mouse.system"), x: 162, y: 185, w: 136, h: 14})
	}
	return buttons
}

func (a *app) mouseControlAt(displayX, displayY int) (mouseControl, bool) {
	if !a.pointerInView(displayX, displayY) {
		return mouseControl{}, false
	}
	x, y := a.toLayout(displayX, displayY)
	for _, b := range a.mouseButtons() {
		if mouseRectContains(x, y, b.x, b.y, b.w, b.h) {
			return b, true
		}
	}
	return mouseControl{}, false
}

func (a *app) updateMouseControls() bool {
	wasOpen := a.mouseCard.open
	if a.mouseCard.capture && !wasOpen {
		if !ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
			a.mouseCard.capture = false
		}
		return true
	}
	if wasOpen && (inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) || inpututil.IsKeyJustPressed(ebiten.KeyEscape)) {
		a.mouseCard.open = false
		a.mouseCard.capture = true
		return true
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		if b, ok := a.mouseControlAt(ebiten.CursorPosition()); ok {
			a.mouseCard.capture = true
			a.activateMouseControl(b)
			return true
		}
	}
	if wasOpen && inpututil.MouseButtonPressDuration(ebiten.MouseButtonLeft) >= 20 && inpututil.MouseButtonPressDuration(ebiten.MouseButtonLeft)%4 == 0 {
		if b, ok := a.mouseControlAt(ebiten.CursorPosition()); ok && b.id == "action" && mouseRepeatable(b.action) {
			a.activateMouseControl(b)
		}
	}
	return wasOpen
}

func mouseRepeatable(action game.Action) bool {
	return strings.HasPrefix(string(action), "previous-") || strings.HasPrefix(string(action), "next-") || strings.HasPrefix(string(action), "interior-") || action == game.ActionIncreaseAttribute || action == game.ActionDecreaseAttribute
}

func (a *app) activateMouseControl(b mouseControl) {
	switch b.id {
	case "open":
		if a.usesOriginalTitle() {
			if art := a.originalTitle(); art != nil && a.originalTitleShowsNotice(art) {
				art.noticeDone = true
			}
		}
		a.mouseCard.open = true
		a.mouseCard.page = 0
		a.mouseCard.system = false
		a.mouseCard.mode = ""
	case "close":
		a.mouseCard.open = false
	case "context":
		a.mouseCard.system = false
		a.mouseCard.page = 0
	case "system":
		a.mouseCard.system = true
		a.mouseCard.page = 0
	case "prev":
		if a.mouseCard.page > 0 {
			a.mouseCard.page--
		}
	case "next":
		if (a.mouseCard.page+1)*mousePageSize < len(a.mouseChoices()) {
			a.mouseCard.page++
		}
	case "name":
		a.mouseCard.mode = "name"
	case "shift":
		a.mouseCard.shift = !a.mouseCard.shift
	case "space":
		a.state = game.AppendCharacterNameDraft(a.state, []rune{' '})
	case "erase":
		a.state = game.BackspaceCharacterNameDraft(a.state)
	case "name-confirm":
		a.step(game.ActionConfirmCharacterName)
		if a.state.Screen != game.ScreenCharacterCreation {
			a.mouseCard.open = false
			a.mouseCard.mode = ""
		}
	case "browse":
		a.openMouseDiskBrowser()
	case "parent":
		if a.mouseCard.directory == "" {
			home, _ := os.UserHomeDir()
			a.readMouseDirectory(home)
		} else if filepath.Dir(a.mouseCard.directory) == a.mouseCard.directory && len(a.mouseCard.roots) > 0 {
			a.readMouseDrives()
		} else {
			a.readMouseDirectory(filepath.Dir(a.mouseCard.directory))
		}
	case "roots":
		a.readMouseDrives()
	case "detect":
		a.detectDisk()
		a.mouseCard.open = false
	case "locale":
		a.cycleLocale(1)
	case "presentation":
		a.cyclePresentation(1)
	case "music":
		a.toggleMusic()
	case "sfx":
		if a.sounds.toggleMute() {
			a.state.MessageKey = "ui.message.sound_off"
		} else {
			a.state.MessageKey = "ui.message.sound_on"
		}
	case "help":
		a.showHelp = !a.showHelp
		a.mouseCard.open = false
	case "options":
		a.openOptions()
		a.mouseCard.open = false
	case "options-close":
		a.closeOptions()
		a.mouseCard.open = false
	case "credits-close":
		a.showCredits = false
		a.mouseCard.open = false
	case "action":
		before := a.state.Screen
		a.step(b.action)
		keep := strings.HasPrefix(string(b.action), "previous-") || strings.HasPrefix(string(b.action), "next-") || b.action == game.ActionIncreaseAttribute || b.action == game.ActionDecreaseAttribute || strings.HasPrefix(string(b.action), "interior-")
		if a.state.Screen != before || !keep {
			a.mouseCard.open = false
		}
	default:
		if strings.HasPrefix(b.id, "char:") {
			a.state = game.AppendCharacterNameDraft(a.state, []rune(strings.TrimPrefix(b.id, "char:")))
		}
		var index int
		if _, err := fmt.Sscanf(b.id, "slot:%d", &index); err == nil && index >= 0 && index < len(prototypeLibrarySlots) {
			a.selectLibrarySlot(index)
		}
		if _, err := fmt.Sscanf(b.id, "file:%d", &index); err == nil && index >= 0 && index < len(a.mouseCard.files) {
			file := a.mouseCard.files[index]
			if file.directory {
				a.readMouseDirectory(file.path)
			} else if a.applyDisk(file.path) {
				a.mouseCard.open = false
				a.mouseCard.mode = ""
			} else {
				a.mouseCard.errorKey = "ui.options.disk_failed"
			}
		}
		if _, err := fmt.Sscanf(b.id, "look:%d", &index); err == nil && index >= 0 && index < len(theme.Presentations()) {
			a.chooseLook(theme.Presentations()[index])
			a.mouseCard.open = false
		}
	}
}

func (a *app) openMouseDiskBrowser() {
	a.mouseCard.roots, _ = mouseDriveRoots()
	directory := filepath.Dir(a.options.draft)
	if directory == "." {
		directory, _ = os.Getwd()
	}
	if stat, err := os.Stat(directory); err != nil || !stat.IsDir() {
		directory, _ = os.UserHomeDir()
	}
	a.mouseCard.open = true
	a.mouseCard.mode = "disk"
	a.readMouseDirectory(directory)
}

func (a *app) readMouseDrives() {
	a.mouseCard.directory = ""
	a.mouseCard.page = 0
	a.mouseCard.errorKey = ""
	a.mouseCard.files = nil
	for _, root := range a.mouseCard.roots {
		a.mouseCard.files = append(a.mouseCard.files, mouseFile{root, root, true})
	}
}

func (a *app) readMouseDirectory(directory string) {
	a.mouseCard.directory = directory
	a.mouseCard.page = 0
	a.mouseCard.errorKey = ""
	a.mouseCard.files = nil
	entries, err := os.ReadDir(directory)
	if err != nil {
		a.mouseCard.errorKey = "ui.mouse.directory_error"
		return
	}
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		stat, err := os.Stat(path)
		if err != nil {
			continue
		}
		if stat.IsDir() || strings.EqualFold(filepath.Ext(path), ".st") {
			a.mouseCard.files = append(a.mouseCard.files, mouseFile{entry.Name(), path, stat.IsDir()})
		}
	}
	sort.Slice(a.mouseCard.files, func(i, j int) bool {
		l, r := a.mouseCard.files[i], a.mouseCard.files[j]
		if l.directory != r.directory {
			return l.directory
		}
		return strings.ToLower(l.name) < strings.ToLower(r.name)
	})
}

func (a *app) drawMouseControls(screen *ebiten.Image) {
	scale := a.viewScale()
	fg := color.RGBA{240, 244, 250, 255}
	blue := color.RGBA{20, 47, 80, 255}
	rect := func(x, y, w, h int, c color.RGBA) {
		vector.DrawFilledRect(screen, float32(x*scale+a.originX), float32(y*scale+a.originY), float32(w*scale), float32(h*scale), c, false)
	}
	text := func(value string, x, y, w, lines int) {
		for i, line := range localization.WrapText(a.displayFace(), value, w*scale, lines) {
			ebitentext.Draw(screen, line, a.displayFace(), x*scale+a.originX, (y+8+i*8)*scale+a.originY, fg)
		}
	}
	if a.mouseCard.open {
		rect(18, 16, 284, 184, color.RGBA{4, 12, 24, 255})
		rect(18, 16, 284, 1, color.RGBA{74, 164, 240, 255})
		text(a.lookup("ui.mouse.menu"), 24, 20, 224, 1)
		info := screenLabel(a.labels, a.state.Screen)
		if a.state.Screen == game.ScreenAttributeAllocation {
			info = ""
			for i, name := range game.AttributeLabels {
				value := fmt.Sprintf("%s %d", name, a.state.Attributes[i])
				if i == a.state.AttributeSelected {
					value = "[" + value + "]"
				}
				info += value + "  "
			}
			info += fmt.Sprintf("(%d)", a.state.AttributePoints)
		} else if a.state.Screen == game.ScreenNavigation {
			info = localization.Destination(a.labels, a.state.NavigationTargetID, a.state.NavigationTarget)
		} else if a.state.Screen == game.ScreenParts {
			if lines := a.partsCounterLines(); len(lines) > 0 {
				info = lines[0]
			}
		} else if a.state.Screen == game.ScreenInterior {
			info = fmt.Sprintf("%s  %d, %d", info, a.state.Interior.Position.X, a.state.Interior.Position.Y)
		}
		if a.mouseCard.mode == "name" {
			info = a.state.CharacterDraft
		}
		if a.mouseCard.mode == "disk" {
			info = a.mouseCard.directory
			if info == "" {
				info = a.lookup("ui.mouse.drives")
			}
		}
		if a.mouseCard.errorKey != "" {
			info = a.lookup(a.mouseCard.errorKey)
		}
		text(info, 24, 37, 272, 1)
		if a.mouseCard.mode != "name" {
			count := (len(a.mouseChoices()) + mousePageSize - 1) / mousePageSize
			if count < 1 {
				count = 1
			}
			text(fmt.Sprintf("%d / %d", a.mouseCard.page+1, count), 133, 171, 64, 1)
		}
	}
	for _, b := range a.mouseButtons() {
		rect(b.x, b.y, b.w, b.h, blue)
		text(b.label, b.x+3, b.y+1, b.w-6, 2)
	}
}
