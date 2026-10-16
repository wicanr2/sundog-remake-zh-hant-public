package main

import (
	"image"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
)

// The original's title screen, drawn from the player's own disk when the
// Atari ST chrome is on. XSTARTUP:0x46 sign_on with DONESOFA:0x04
// wipe_title_page (docs/atari-st/title-screen.md):
//
//   - image 0 (320x200) is the title picture, decoded at (0,0);
//   - image 1 (224x51) is the SUNDOG logo, decoded at (16,13);
//   - the menu window is at x 143-304, y 88-190 with a two-pixel frame;
//   - MAINLIB:0x32 title_color runs as background task 3 and cycles the
//     frame colour (palette entry 12) and the logo letters (entry 1);
//   - "About SunDog" shows image 2, the credits, until a button is pressed.
//
// The picture, logo and credits are the original's pixels and never leave the
// disk: they are decoded on the way in and dropped with it.

const (
	originalTitlePicture = 0
	originalLogoPicture  = 1
	originalCredits      = 2
	originalLogoX        = 16
	originalLogoY        = 13
	// The menu window, edges inclusive, as measured on the running original.
	originalTitleWindowX0 = 143
	originalTitleWindowY0 = 88
	originalTitleWindowX1 = 304
	originalTitleWindowY1 = 190
	originalTitleFrame    = 2
	// Rows inside the window: five actions and the message line.
	originalTitleRowX    = 150
	originalTitleRowW    = 150
	originalTitleRowY0   = 96
	originalTitleRowStep = 16
	// title_color advances one step every titleColorStepFrames frames; the
	// running original was sampled at 0.16 s per step (title-screen.md).
	titleColorStepFrames = 10
	titleColorSteps      = 6
	// The picture is decoded straight onto the screen, top to bottom, in
	// under a second on the original; then the logo and the notice window
	// come up, and the notice gives way to the menu on a timeout (0x2d0
	// ticks, about 12 s) or a button. The remake keeps the order and the
	// shapes and shortens the waits: they were disk-load time, not design.
	titleRevealRowsPerFrame = 8
	titleLogoFrame          = 40  // logo and notice appear
	titleNoticeFrames       = 240 // notice stays this long unless a key ends it
)

// originalTitleNotice is the sign_on notice, line for line (XSTARTUP:0x46).
var originalTitleNotice = []string{"SunDog™", "Frozen Legacy", "Version 3.0", "", "Copyright (c) 1985", "by FTL Games and", "Software Heaven Inc.", "All rights reserved"}

// originalTitleActions are the rows in the window, top to bottom.
var originalTitleActions = []game.Action{
	game.ActionPlay, game.ActionSelectGame, game.ActionLoadGame, game.ActionOpenOptions, game.ActionAboutSunDog,
}

var originalTitleLabels = map[game.Action]string{
	game.ActionPlay:        "ui.play_sundog",
	game.ActionSelectGame:  "ui.select_game",
	game.ActionLoadGame:    "ui.load_game",
	game.ActionOpenOptions: "ui.options",
	game.ActionAboutSunDog: "ui.about_sundog",
}

// originalTitleArt is what was decoded from the disk for the title screen.
type originalTitleArt struct {
	// enteredFrame is when the title came up; the reveal and the notice are
	// timed from it. noticeDone is set by a key so the menu comes early.
	enteredFrame int
	noticeDone   bool
	picture      *ebiten.Image
	// logo has one image per letter colour step, since the letters are a
	// palette animation and the picture itself is never redrawn.
	logo    [titleColorSteps]*ebiten.Image
	credits [titleColorSteps]*ebiten.Image
}

// titlePalette is the sixteen colours while the title is up: the start-up
// palette (XSTARTUP:0x02 set_palette) with entry 4 at 0x530 and entry 5 at
// 0x007 (sign_on:init_screen), and entries 1 and 12 wherever title_color has
// them this step. Checked against the running original entry by entry.
func titlePalette(step int) [16]color.RGBA {
	colours := originalmedia.PaletteRGBA()
	colours[1] = titleLetterColour(step)
	colours[4] = color.RGBA{5 * 255 / 7, 3 * 255 / 7, 0, 255}
	colours[5] = color.RGBA{0, 0, 255, 255}
	colours[12] = titleFrameColour(step)
	return colours
}

// usesOriginalTitle: once the player's disk is in, the title is always the
// Atari ST original. Chrome and tileset are in-game presentation choices;
// they no longer select a different startup owner.
func (a *app) usesOriginalTitle() bool {
	return a.state.Screen == game.ScreenTitle && a.originalDisk != nil &&
		!a.showHelp && !a.showOptions
}

// titleColorStep is where title_color is in its cycle this frame: a triangle
// over titleColorSteps steps, as sampled on the running original.
func (a *app) titleColorStep() int {
	return int(a.frames/titleColorStepFrames) % titleColorSteps
}

// titleFrameColour is palette entry 12 at a step: (7,7,b) with b 0,2,4,6,4,2.
func titleFrameColour(step int) color.RGBA {
	b := []int{0, 2, 4, 6, 4, 2}[step%titleColorSteps]
	return color.RGBA{255, 255, uint8(b * 255 / 7), 255}
}

// titleLetterColour is palette entry 1 at a step: (7,g,0) with g 4,5,6,7,6,5.
func titleLetterColour(step int) color.RGBA {
	g := []int{4, 5, 6, 7, 6, 5}[step%titleColorSteps]
	return color.RGBA{255, uint8(g * 255 / 7), 0, 255}
}

// pictureImage turns a decoded picture into an image under the given colours.
func pictureImage(picture originalmedia.Picture, colours [16]color.RGBA) *ebiten.Image {
	decoded := image.NewRGBA(image.Rect(0, 0, picture.Width, picture.Height))
	for y := 0; y < picture.Height; y++ {
		for x := 0; x < picture.Width; x++ {
			decoded.SetRGBA(x, y, colours[picture.Pixels[y*picture.Width+x]&0x0f])
		}
	}
	return ebiten.NewImageFromImage(decoded)
}

// originalTitle decodes the three pictures once per disk.
func (a *app) originalTitle() *originalTitleArt {
	if a.originalTitleRead {
		return a.originalTitleArt
	}
	a.originalTitleRead = true
	if a.originalDisk == nil {
		return nil
	}
	picture, err := a.originalDisk.DecodeImage(originalTitlePicture)
	if err != nil {
		log.Printf("原版標題畫面讀不出來，標題改用重製版的封面：%v", err)
		return nil
	}
	logo, err := a.originalDisk.DecodeImage(originalLogoPicture)
	if err != nil {
		log.Printf("原版標誌讀不出來，標題改用重製版的封面：%v", err)
		return nil
	}
	art := &originalTitleArt{enteredFrame: a.frames}
	// The title palette is the start-up palette with three entries changed
	// (sign_on:init_screen and title_color): 5 is 0x007, 1 and 12 cycle. The
	// picture uses none of the three; the logo uses all of them.
	colours := titlePalette(0)
	art.picture = pictureImage(picture, colours)
	for step := range art.logo {
		art.logo[step] = pictureImage(logo, titlePalette(step))
	}
	// The credits go up under the same palette, cycling included.
	if credits, err := a.originalDisk.DecodeImage(originalCredits); err == nil {
		for step := range art.credits {
			art.credits[step] = pictureImage(credits, titlePalette(step))
		}
	} else {
		log.Printf("原版製作名單讀不出來：%v", err)
	}
	a.originalTitleArt = art
	return art
}

// originalTitleRowY is the text row of the index-th window row.
func originalTitleRowY(index int) int { return originalTitleRowY0 + index*originalTitleRowStep }

// originalTitleActionAt is the mouse's meaning on the original title: the
// window rows, nothing else.
func originalTitleActionAt(x, y int) game.Action {
	if x < originalTitleWindowX0 || x > originalTitleWindowX1 {
		return ""
	}
	for index, action := range originalTitleActions {
		top := originalTitleRowY(index) - 4
		if y >= top && y < top+originalTitleRowStep {
			return action
		}
	}
	return ""
}

// drawOriginalTitle paints the title the original's way onto the interface
// layer: picture, logo, then the window with the remake's rows in it.
func (a *app) drawOriginalTitle(screen *ebiten.Image, art *originalTitleArt) {
	step := a.titleColorStep()
	elapsed := a.frames - art.enteredFrame
	rows := elapsed * titleRevealRowsPerFrame
	if rows > logicalHeight {
		rows = logicalHeight
	}
	screen.Fill(color.RGBA{0, 0, 0, 255})
	if rows > 0 {
		screen.DrawImage(art.picture.SubImage(image.Rect(0, 0, logicalWidth, rows)).(*ebiten.Image), &ebiten.DrawImageOptions{})
	}
	if elapsed < titleLogoFrame {
		return
	}
	logoOptions := &ebiten.DrawImageOptions{}
	logoOptions.GeoM.Translate(originalLogoX, originalLogoY)
	screen.DrawImage(art.logo[step], logoOptions)

	x0, y0 := float32(originalTitleWindowX0), float32(originalTitleWindowY0)
	w, h := float32(originalTitleWindowX1-originalTitleWindowX0+1), float32(originalTitleWindowY1-originalTitleWindowY0+1)
	uiFillRect(screen, x0, y0, w, h, titleFrameColour(step), false)
	uiFillRect(screen, x0+originalTitleFrame, y0+originalTitleFrame, w-2*originalTitleFrame, h-2*originalTitleFrame, color.RGBA{0, 0, 0, 255}, false)

	white := color.RGBA{255, 255, 255, 255}
	if a.originalTitleShowsNotice(art) {
		for index, line := range originalTitleNotice {
			a.drawTextCentered(screen, line, originalTitleWindowX0+originalTitleFrame, originalTitleWindowX1-originalTitleFrame, originalTitleRowY0+index*10, white)
		}
		return
	}
	bar := originalmedia.PaletteRGBA()[9]
	for index, action := range originalTitleActions {
		y := originalTitleRowY(index)
		uiFillRect(screen, x0+originalTitleFrame, float32(y-3), w-2*originalTitleFrame, 2, bar, false)
		a.drawTextWithin(screen, a.lookup(originalTitleLabels[action]), originalTitleRowX, y, originalTitleRowW, white)
	}
	a.drawTextWithin(screen, a.lookup(a.state.MessageKey), originalTitleRowX, originalTitleRowY(len(originalTitleActions)), originalTitleRowW, titleFrameColour(step))
}

// drawOriginalCredits is "About SunDog": the credits picture over the whole
// frame until any key or button.
func (a *app) drawOriginalCredits(screen *ebiten.Image, art *originalTitleArt) {
	screen.DrawImage(art.credits[a.titleColorStep()], &ebiten.DrawImageOptions{})
}

// originalTitleShowsNotice: the notice is up between the logo's arrival and
// its timeout, unless a key already ended it.
func (a *app) originalTitleShowsNotice(art *originalTitleArt) bool {
	elapsed := a.frames - art.enteredFrame
	return !art.noticeDone && elapsed < titleLogoFrame+titleNoticeFrames
}

// enterOriginalTitle restarts the sequence: called when the title screen is
// reached with the original title in use.
func (a *app) enterOriginalTitle() {
	if art := a.originalTitle(); art != nil {
		art.enteredFrame = a.frames
		art.noticeDone = false
	}
}

// drawTextCentered centres one line between x0 and x1.
func (a *app) drawTextCentered(screen *ebiten.Image, value string, x0, x1, y int, c color.RGBA) {
	width := a.textWidth(value)
	x := x0 + (x1-x0-width)/2
	if x < x0 {
		x = x0
	}
	a.drawTextWithin(screen, value, x, y, x1-x0, c)
}
