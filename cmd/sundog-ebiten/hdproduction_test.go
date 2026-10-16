package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/localization"
	"github.com/wicanr2/sundog-remake/internal/source/atarist/originalmedia"
	"github.com/wicanr2/sundog-remake/internal/theme"
	"github.com/wicanr2/sundog-remake/internal/world"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

func hdProductionTestApp(t *testing.T) *app {
	t.Helper()
	previousPanelLayout := panelLayoutST
	t.Cleanup(func() { panelLayoutST = previousPanelLayout })
	t.Chdir("../..")
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	languages, err := loadLanguagePacks("docs/i18n/apple2-text-catalog.json", "docs/i18n/sundog-ui-zh-Hant.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("assets/fonts/NotoSansCJKtc-Regular.otf")
	if err != nil {
		t.Fatal(err)
	}
	f, err := opentype.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	newFace := func(size int) font.Face {
		face, e := opentype.NewFace(f, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingFull})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { face.Close() })
		return face
	}
	var model struct {
		State game.State `json:"state"`
	}
	raw, err = os.ReadFile("tools/atari_hd_full_engineering_player_model.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &model); err != nil {
		t.Fatal(err)
	}
	preset, _ := theme.PresentationByID(theme.AtariSTHDPresentation)
	a := &app{reducer: r, state: game.WithPresentation(model.State, preset), languages: languages, face: newFace(8), newFace: newFace, originalTiles: map[int]*originalTileCache{}}
	if disk := os.Getenv("HD_WORLD_DISK"); disk != "" {
		a.originalDisk, err = originalmedia.Open(disk)
		if err != nil {
			t.Fatal(err)
		}
	}
	a.applyLocale(localization.TraditionalChinese)
	return a
}

func TestHDCompleteAssetsAtNativeSizes(t *testing.T) {
	a := hdProductionTestApp(t)
	art := hdCompleteAssets()
	for _, scale := range []int{3, 4, 5, 6} {
		a.scale = scale
		for key, nodes := range art.Strips {
			if len(nodes) == 0 {
				t.Fatal("empty tile", key)
			}
			img := a.hdWorldSprite("strip/"+key, 32, 32, nodes)
			if img == nil || img.Bounds().Dx() != 32*scale || img.Bounds().Dy() != 32*scale {
				t.Fatal("tile dimensions", key, scale)
			}
		}
		for index := range art.Rooms {
			img := a.hdRoomImage(index)
			if img == nil || img.Bounds().Dx() != 160*scale || img.Bounds().Dy() != 100*scale {
				t.Fatal("room dimensions", index, scale)
			}
		}
		for code := 0; code < 32; code++ {
			if img := hdItemImage(code, scale); img == nil || img.Bounds().Dx() != 16*scale {
				t.Fatal("icon", code, scale)
			}
		}
		for _, key := range []string{"cabin-closed", "cabin-lit"} {
			if img := a.hdCabinRaster(key, 320, 200); img == nil || img.Bounds().Dx() != 320*scale {
				t.Fatal("cabin", key, scale)
			}
		}
	}
}

func TestHDOptionsLongPathsKeepFilenameAndFullDraft(t *testing.T) {
	a := hdProductionTestApp(t)
	a.scale = 4
	for _, prefix := range []string{"/a/very/long/folder/", "/資料/遊戲/原版/"} {
		path := strings.Repeat(prefix, 20) + "sundog.st"
		a.showOptions = true
		a.options = optionsCard{row: optionsRowDisk, draft: path}
		view := a.fitOptionsField(path+"＿", optionsValueW, true)
		p := hdReviewPainter{a: a, scale: 4}
		if !strings.HasSuffix(view, "sundog.st＿") || font.MeasureString(p.face(8), view).Ceil() > optionsValueW*4 {
			t.Fatalf("path view does not fit or lost its filename: %q", view)
		}
		image := ebiten.NewImage(1280, 800)
		a.Draw(image)
		image.Dispose()
		if a.options.draft != path {
			t.Fatal("display fitting changed the editable disk path")
		}
	}
}

func TestLocalizedFooterUsesRealEnglishAndKoreanControlWords(t *testing.T) {
	a := hdProductionTestApp(t)
	for _, locale := range []localization.Locale{localization.English, localization.Korean} {
		a.applyLocale(locale)
		for _, name := range []game.Screen{game.ScreenShipboard, game.ScreenCharacterCreation, game.ScreenDialog} {
			s := a.state
			s.Screen = name
			line := footerLabel(a.labels, s)
			for _, leaked := range []string{"左鍵", "右鍵", "輸入名稱", "保存終局"} {
				if strings.Contains(line, leaked) {
					t.Fatalf("%s/%s leaked Chinese controls: %s", locale, name, line)
				}
			}
			if strings.Contains(line, "ui.") {
				t.Fatalf("%s/%s has an unresolved footer key: %s", locale, name, line)
			}
		}
	}
}

func TestHDCitySceneUsesBoundedSourceAcrossEdges(t *testing.T) {
	a := hdProductionTestApp(t)
	if a.originalDisk == nil {
		t.Skip("HD_WORLD_DISK required for city-edge regression")
	}
	a.scale = 4
	a.state.GroundWorld = false
	for _, position := range []game.Position{{X: 2, Y: -2}, {X: -2, Y: 2}, {X: world.MapWidth() + 1, Y: 2}, {X: 2, Y: world.MapHeight() + 1}} {
		a.state.Position = position
		scene := a.buildSTGroundScene(a.state.CityID, position.X-stGroundOriginLeft, position.Y-stGroundOriginTop, 0)
		if scene == nil || scene.cityNumber <= 0 {
			t.Fatalf("original scene missing at edge %+v", position)
		}
		beforeState, _ := json.Marshal(a.state)
		beforeCollision := bytes.Clone(scene.indices)
		if image := a.hdGroundScene(scene); image == nil || image.Bounds().Dx() != 1280 {
			t.Fatalf("HD edge image missing at %+v", position)
		}
		afterState, _ := json.Marshal(a.state)
		if !bytes.Equal(beforeState, afterState) || !bytes.Equal(beforeCollision, scene.indices) {
			t.Fatalf("HD edge draw changed game or collision at %+v", position)
		}
	}
}

func TestHDSourceTileRoutingMatchesDecodedDisk(t *testing.T) {
	a := hdProductionTestApp(t)
	if a.originalDisk == nil {
		t.Skip("HD_WORLD_DISK required for original-data routing check")
	}
	for city := 1; city <= 60; city++ {
		strips, err := a.originalDisk.CityStripIndices(city)
		if err != nil {
			t.Fatal(err)
		}
		set, err := a.originalDisk.LoadCityTiles(city)
		if err != nil {
			t.Fatal(err)
		}
		for code := 0; code < 40; code++ {
			for roll := 0; roll < 4; roll++ {
				pixels, drawn := set.TileIndices(code, roll)
				strip, cell, hdDrawn := hdCityTile(strips, code, roll)
				if drawn != hdDrawn {
					t.Fatal("routing presence", city, code, roll)
				}
				if !drawn {
					continue
				}
				picture, err := a.originalDisk.DecodeImage(strip)
				if err != nil {
					t.Fatal(err)
				}
				for y := 0; y < 32; y++ {
					for x := 0; x < 32; x++ {
						if pixels[y*32+x] != picture.Pixels[y*picture.Width+cell*32+x]&15 {
							t.Fatal("routing source", city, code, roll, strip, cell)
						}
					}
				}
			}
		}
	}
	raw, err := os.ReadFile(os.Getenv("HD_WORLD_DISK"))
	if err != nil {
		t.Fatal(err)
	}
	for planet := 0; planet < 18; planet++ {
		header := raw[0xaa00+planet*256 : 0xaa00+planet*256+16]
		strips := [3]int{54 + int(header[2]), 46 + int(header[1]), 46 + int(header[0])}
		set, err := a.originalDisk.LoadPlanetTiles(planet)
		if err != nil {
			t.Fatal(err)
		}
		for code := 0; code < 43; code++ {
			pixels, drawn := set.SurfaceTileIndices(code)
			strip, cell, hdDrawn := hdSurfaceTile(strips, code)
			if drawn != hdDrawn {
				t.Fatal("surface routing presence", planet, code)
			}
			if !drawn {
				continue
			}
			picture, err := a.originalDisk.DecodeImage(strip)
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < 32; y++ {
				for x := 0; x < 32; x++ {
					if pixels[y*32+x] != picture.Pixels[y*picture.Width+cell*32+x]&15 {
						t.Fatal("surface routing source", planet, code)
					}
				}
			}
		}
	}
}

func TestHDAllScreensKeepTypedState(t *testing.T) {
	a := hdProductionTestApp(t)
	if a.originalDisk == nil {
		t.Skip("HD_WORLD_DISK required for full scene draw")
	}
	base, _ := json.Marshal(a.state)
	screens := []game.Screen{"ftl-boost", "depot", "fuel-purchase", "title", "character-library", "character-creation", "attribute-allocation", "character-status", "shipboard", "ground", "trade", "navigation", "uniteller", "tactical", "flight", "engineering", "equipment", "stores", "parts", "cargo", "dialog", "outcome", "interior", "slots", "arena", "score", "tube-station"}
	count := 0
	for _, locale := range localization.Locales() {
		a.applyLocale(locale)
		for _, scale := range []int{4, 5} {
			a.scale = scale
			for _, name := range screens {
				if err := json.Unmarshal(base, &a.state); err != nil {
					t.Fatal(err)
				}
				a.state.Screen = name
				a.uiLayer = nil
				a.textLayer = nil
				a.frames = 400
				img := ebiten.NewImage(320*scale, 200*scale)
				// Warm view caches and the same original collision scene used by
				// normal Draw. These are fixtures, not a normal player path.
				a.Draw(img)
				before, _ := json.Marshal(a.state)
				var collision []byte
				if a.stGroundScene != nil {
					collision = append(collision, a.stGroundScene.indices...)
				}
				a.Draw(img)
				after, _ := json.Marshal(a.state)
				if !bytes.Equal(before, after) {
					t.Fatal("HD Draw changed state", name, locale, scale)
				}
				if a.stGroundScene != nil && !bytes.Equal(collision, a.stGroundScene.indices) {
					t.Fatal("HD changed collision", name)
				}
				img.Dispose()
				count++
			}
		}
	}
	t.Log(fmt.Sprintf("%d queued native scenes; normal GPU/player verification is separate", count))
}

func TestHDDynamicPresentationVariantsKeepState(t *testing.T) {
	a := hdProductionTestApp(t)
	if a.originalDisk == nil {
		t.Skip("HD_WORLD_DISK required for dynamic scene checks")
	}
	base, _ := json.Marshal(a.state)
	count := 0
	for _, locale := range localization.Locales() {
		a.applyLocale(locale)
		for _, scale := range []int{4, 5} {
			a.scale = scale
			for variant := 0; variant < 24; variant++ {
				json.Unmarshal(base, &a.state)
				a.showCredits = false
				a.showHelp = false
				a.showOptions = false
				a.state.Screen = game.ScreenTactical
				if variant < 21 {
					a.state.CombatActive = true
					a.state.EnemyType = variant / 3
					a.state.EnemyBearing = 512
					a.state.EnemyPitch = 512
					a.state.CombatRange = 40
					a.frames = (variant % 3) * stEnemyFrameStep
				} else if variant == 21 {
					a.state.Screen = game.ScreenTitle
					a.showCredits = true
				} else if variant == 22 {
					a.showHelp = true
				} else {
					a.showOptions = true
				}
				a.uiLayer = nil
				a.textLayer = nil
				img := ebiten.NewImage(320*scale, 200*scale)
				before, _ := json.Marshal(a.state)
				a.Draw(img)
				after, _ := json.Marshal(a.state)
				if !bytes.Equal(before, after) {
					t.Fatal("variant changed state", variant, locale, scale)
				}
				img.Dispose()
				count++
			}
		}
	}
	t.Logf("%d queued dynamic cases: seven enemy types/three aspects, credits, help, options", count)
}
