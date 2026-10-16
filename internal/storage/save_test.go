package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wicanr2/sundog-remake/internal/game"
	"github.com/wicanr2/sundog-remake/internal/world"
)

func TestPaidPurchaseDestinationSurvivesSaveAndLibrary(t *testing.T) {
	for _, library := range []bool{false, true} {
		for _, choice := range []game.Action{game.ActionTradePick1, game.ActionTradePick2, game.ActionBack} {
			r, err := game.NewReducer()
			if err != nil {
				t.Fatal(err)
			}
			s := game.NewState()
			s.Cash, s.Bank = 100000, 0
			for _, a := range []game.Action{game.ActionPlay, game.ActionOpenGround, game.ActionOpenTrade} {
				s, err = r.Step(s, a)
				if err != nil {
					t.Fatal(err)
				}
			}
			s.GroundMode = game.GroundInPod
			quote, _ := game.CurrentQuote(s)
			before := game.TradeFunds(s)
			warehouseSlot := -1
			for i, lot := range game.WarehouseLotsOf(s, s.CityID) {
				if lot.Goods == 0 {
					warehouseSlot = i
					break
				}
			}
			s, err = r.Step(s, game.ActionBuyGems)
			if err != nil {
				t.Fatal(err)
			}
			if s.TradePurchaseLot == nil || s.TradePage != game.TradePageBuyDestination || game.TradeFunds(s) != before-game.Credits(quote.Price) {
				t.Fatal("待選槽前付款缺失")
			}
			paid := game.TradeFunds(s)
			rolls := s.TradeRolls
			pending := *s.TradePurchaseLot
			for _, a := range []game.Action{game.ActionBuyGems, game.ActionTradePass, game.ActionTradeSellPage, game.ActionTradeExchange, game.ActionTradeWarehousePage} {
				again, e := r.Step(s, a)
				if e != nil || !reflect.DeepEqual(again, s) {
					t.Fatalf("待選槽動作%s改動狀態", a)
				}
			}
			path := filepath.Join(t.TempDir(), "paid.json")
			if library {
				err = MakeLibrary(path)
				if err == nil {
					err = StoreGame(path, "slot-1", s)
				}
				if err == nil {
					s, err = LoadGame(path, "slot-1")
				}
			} else {
				err = Save(path, s)
				if err == nil {
					s, err = Load(path)
				}
			}
			if err != nil || s.TradePurchaseLot == nil || *s.TradePurchaseLot != pending || game.TradeFunds(s) != paid || s.TradeRolls != rolls || s.TradePage != game.TradePageBuyDestination {
				t.Fatalf("待選槽重開 library%v: %v", library, err)
			}
			s, err = r.Step(s, choice)
			if err != nil {
				t.Fatal(err)
			}
			if s.TradePurchaseLot != nil || game.TradeFunds(s) != paid || s.TradePage != game.TradePageMain {
				t.Fatal("放貨再扣款或未清待選槽")
			}
			got := s.Holds[0].Lot()
			if choice == game.ActionTradePick1 {
				got = game.WarehouseLotsOf(s, s.CityID)[warehouseSlot]
			}
			if got != pending {
				t.Fatalf("重開後貨物改變 %+v want%+v", got, pending)
			}
		}
	}
}

func TestSchema39PurchaseSaveIsRefused(t *testing.T) {
	s := game.NewState()
	s.Schema = 39
	b, err := json.Marshal(Envelope{Format: formatV39, State: s})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "schema39.json")
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(path); !errors.Is(err, ErrOldSave) {
		t.Fatalf("舊39保存未拒絕: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(b) {
		t.Fatal("拒絕時改寫舊保存")
	}
}

func TestCryoWarehouseSaveAndLibraryDoNotReplenish(t *testing.T) {
	for _, library := range []bool{false, true} {
		s := game.NewState()
		s.Screen = game.ScreenShipboard
		lot := s.Warehouses["tuie"][0]
		if lot != (game.Lot{Goods: 31, Quality: 7, SourceCode: 17}) {
			t.Fatal("fixed original batch missing")
		}
		// Controlled post-Fetch state. Actual Fetch and normal player travel
		// are separately checked by the native adapter and GUI receipt.
		r, err := game.NewReducer()
		if err != nil {
			t.Fatal(err)
		}
		kind, ok := r.World.CargoKindByCode(31)
		if !ok {
			t.Fatal("missing original cargo31")
		}
		s.Warehouses["tuie"][0] = game.Lot{}
		s.Holds[0] = game.Hold{Kind: game.HoldGoods, Goods: lot.Goods, Quality: lot.Quality, Price: lot.Price, SourceCode: lot.SourceCode, ItemKey: kind.Key}
		path := filepath.Join(t.TempDir(), "cryo.json")
		if library {
			err = MakeLibrary(path)
			if err == nil {
				err = StoreGame(path, "slot-1", s)
			}
		} else {
			err = Save(path, s)
		}
		if err != nil {
			t.Fatal(err)
		}
		// Loading must use persisted typed state even when a later new game
		// would have a completely different entropy source.
		seed := game.NewGameSeed
		game.NewGameSeed = func() int64 { panic("load attempted new-game random initialization") }
		var loaded game.State
		if library {
			loaded, err = LoadGame(path, "slot-1")
		} else {
			loaded, err = Load(path)
		}
		game.NewGameSeed = seed
		if err != nil || !reflect.DeepEqual(loaded.Warehouses, s.Warehouses) || loaded.Holds != s.Holds {
			t.Fatalf("cold lots changed after load library%v: %v", library, err)
		}
	}
}

func TestSchema40AndMissingWarehouseInitializationAreRefused(t *testing.T) {
	s := game.NewState()
	s.Schema = 40
	b, err := json.Marshal(Envelope{Format: formatV40, State: s})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "old.json")
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(path); !errors.Is(err, ErrOldSave) {
		t.Fatalf("schema40 was accepted: %v", err)
	}
	s.Schema = game.CurrentSchema
	b, _ = json.Marshal(s)
	for _, missing := range []bool{true, false} {
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(b, &fields); err != nil {
			t.Fatal(err)
		}
		if missing {
			delete(fields, "warehouses")
		} else {
			fields["warehouses"] = json.RawMessage("null")
		}
		raw, _ := json.Marshal(fields)
		if _, err = decodeCurrentState(raw); !errors.Is(err, ErrOldSave) {
			t.Fatalf("uninitialized warehouses accepted missing%v: %v", missing, err)
		}
	}
}

// Banville規格的商品入口保存鏈；不是原版玩家路徑收據。
func TestCargo25And31SaveAndLibraryRoundTrip(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := game.NewState()
	for i, goods := range []int{25, 31} {
		kind, ok := r.World.CargoKindByCode(goods)
		if !ok {
			t.Fatal("missing source cargo")
		}
		s.Holds[i] = game.Hold{Kind: game.HoldGoods, Goods: goods, Quality: 7 - i, Price: 115 - i*114, ItemKey: kind.Key, SourceCode: uint8(255 - i)}
	}
	s.Warehouses = map[string][]game.Lot{s.CityID: {s.Holds[0].Lot(), s.Holds[1].Lot(), {}, {}}}
	path := filepath.Join(t.TempDir(), "cargo.json")
	if err = Save(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Holds, s.Holds) || !reflect.DeepEqual(loaded.Warehouses, s.Warehouses) {
		t.Fatal("cargo metadata lost on save")
	}
	library := filepath.Join(t.TempDir(), "library.json")
	if err = MakeLibrary(library); err != nil {
		t.Fatal(err)
	}
	if err = StoreGame(library, "slot-1", s); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadGame(library, "slot-1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Holds, s.Holds) || !reflect.DeepEqual(loaded.Warehouses, s.Warehouses) {
		t.Fatal("cargo metadata lost in library")
	}
}

func TestTradeClassShiftIsMandatoryAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "class.json")
	for _, shift := range []int{0, 17} {
		s := game.NewState()
		s.TradeClassShift = shift
		if err := Save(path, s); err != nil {
			t.Fatal(err)
		}
		got, err := Load(path)
		if err != nil || got.TradeClassShift != shift {
			t.Fatalf("class round trip: %d %v", got.TradeClassShift, err)
		}
		library := filepath.Join(t.TempDir(), "library.json")
		if err := MakeLibrary(library); err != nil {
			t.Fatal(err)
		}
		if err := StoreGame(library, "slot-1", s); err != nil {
			t.Fatal(err)
		}
		got, err = LoadGame(library, "slot-1")
		if err != nil || got.TradeClassShift != shift {
			t.Fatalf("library class round trip: %d %v", got.TradeClassShift, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var envelope map[string]json.RawMessage
		if err = json.Unmarshal(data, &envelope); err != nil {
			t.Fatal(err)
		}
		var state map[string]json.RawMessage
		if err = json.Unmarshal(envelope["state"], &state); err != nil {
			t.Fatal(err)
		}
		for _, null := range []bool{false, true} {
			delete(state, "tradeClassShift")
			if null {
				state["tradeClassShift"] = json.RawMessage("null")
			}
			envelope["state"], _ = json.Marshal(state)
			bad, _ := json.Marshal(envelope)
			if err = os.WriteFile(path, bad, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = Load(path); !errors.Is(err, ErrOldSave) {
				t.Fatalf("missing/null class accepted: %v", err)
			}
		}
	}
	for _, shift := range []int{-1, 18} {
		s := game.NewState()
		s.TradeClassShift = shift
		if err := Save(path, s); err == nil {
			t.Fatal("invalid class saved")
		}
		library := filepath.Join(t.TempDir(), "library.json")
		if err := MakeLibrary(library); err != nil {
			t.Fatal(err)
		}
		if err := StoreGame(library, "slot-1", s); err == nil {
			t.Fatal("invalid class library saved")
		}
	}
}

// 入口：docs/atari-st/banville-restoration-spec.md。保存與續行是 remake 契約。
func TestTradePurchaseAndSettlementContinueAcrossSaveAndLibrary(t *testing.T) {
	for _, bank := range []int{100000, -9} {
		for _, library := range []bool{false, true} {
			r, err := game.NewReducer()
			if err != nil {
				t.Fatal(err)
			}
			s := game.NewState()
			s.Cash, s.Bank = 100000, bank
			s.BankBalances[s.SystemID] = bank
			step := func(a game.Action) {
				t.Helper()
				s, err = r.Step(s, a)
				if err != nil {
					t.Fatal(err)
				}
			}
			for _, a := range []game.Action{game.ActionPlay, game.ActionOpenGround, game.ActionOpenTrade, game.ActionBuyGems} {
				step(a)
				if a == game.ActionOpenTrade {
					s.GroundMode = game.GroundInPod
				}
			}
			step(game.ActionTradePick2)
			if !s.Holds[0].Occupied() || s.Holds[0].SourceCode == 0 || !s.TradeSession.Active {
				t.Fatalf("purchase missing: %+v", s)
			}
			path := filepath.Join(t.TempDir(), "trade.json")
			roundTrip := func() {
				t.Helper()
				want := s
				if library {
					// 圖書館既有契約會清掉一次性顯示訊息。
					want.MessageKey = "ui.message.ready"
					want.Saved, want.QuitRequested = false, false
					if err = MakeLibrary(path); err == nil {
						err = StoreGame(path, "slot-1", s)
					}
					if err == nil {
						s, err = LoadGame(path, "slot-1")
					}
				} else {
					if err = Save(path, s); err == nil {
						s, err = Load(path)
					}
				}
				if err != nil || !reflect.DeepEqual(s, want) {
					t.Fatalf("bank=%d library=%t round trip: %v", bank, library, err)
				}
			}
			roundTrip()
			lot := s.Holds[0].Lot()
			warehouseSlot := -1
			for i, slot := range game.WarehouseLotsOf(s, s.CityID) {
				if slot.Goods == 0 {
					warehouseSlot = i
					break
				}
			}
			step(game.ActionTradeStorePage)
			step(game.ActionTradePick1)
			if s.Holds[0] != (game.Hold{}) || s.Warehouses[s.CityID][warehouseSlot] != lot {
				t.Fatal("warehouse lost lot metadata")
			}
			roundTrip()
			step(game.ActionTradeFetchPage)
			step([]game.Action{game.ActionTradePick1, game.ActionTradePick2, game.ActionTradePick3, game.ActionTradePick4}[warehouseSlot])
			if s.Holds[0].Lot() != lot {
				t.Fatal("fetch changed source or price")
			}
			step(game.ActionTradeSellPage)
			step(game.ActionTradePick1)
			roundTrip()
			step(game.ActionTradePick1)
			funds, cash := game.TradeFunds(s), s.TradeSession.InitialCash
			if s.Cash != cash || s.Bank != min(bank, 0) {
				t.Fatal("settled during trade")
			}
			roundTrip()
			for i := 0; s.Screen == game.ScreenTrade && i < 4; i++ {
				step(game.ActionBack)
			}
			if s.TradeSession.Active || s.Screen != game.ScreenGround {
				t.Fatal("session survived exchange exit")
			}
			if bank < 0 {
				if s.Bank != bank || s.Cash != funds {
					t.Fatal("loan settlement changed debt")
				}
			} else if s.Cash != min(cash, funds) || s.Bank != funds-s.Cash {
				t.Fatal("positive bank settlement mismatch")
			}
			roundTrip()
		}
	}
}

func TestTradeSaveCannotDefaultMissingSessionOrLotOrigin(t *testing.T) {
	s := game.NewState()
	s.Quotes = []game.Lot{{Goods: 8, Quality: 3, Price: 100}}
	s.Warehouses = map[string][]game.Lot{s.CityID: {{Goods: 8, Quality: 3, Price: 100}}}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"tradeSession", "active", "cityId", "systemId", "initialCash", "initialBank", "funds", "quotes", "warehouses"} {
		for _, missing := range []bool{true, false} {
			var object map[string]json.RawMessage
			if err := json.Unmarshal(raw, &object); err != nil {
				t.Fatal(err)
			}
			remove := func(m map[string]json.RawMessage, key string) {
				if missing {
					delete(m, key)
				} else {
					m[key] = json.RawMessage("null")
				}
			}
			switch path {
			case "tradeSession":
				remove(object, path)
			case "quotes", "warehouses":
				lot := map[string]json.RawMessage{"goods": json.RawMessage("8"), "quality": json.RawMessage("3"), "price": json.RawMessage("100"), "sourceCode": json.RawMessage("1")}
				remove(lot, "sourceCode")
				if path == "quotes" {
					object[path], _ = json.Marshal([]map[string]json.RawMessage{lot})
				} else {
					object[path], _ = json.Marshal(map[string][]map[string]json.RawMessage{s.CityID: {lot}})
				}
			default:
				var session map[string]json.RawMessage
				if err := json.Unmarshal(object["tradeSession"], &session); err != nil {
					t.Fatal(err)
				}
				remove(session, path)
				object["tradeSession"], _ = json.Marshal(session)
			}
			data, _ := json.Marshal(object)
			if _, err := decodeCurrentState(data); !errors.Is(err, ErrOldSave) {
				t.Fatalf("accepted %s missing=%t: %v", path, missing, err)
			}
		}
	}
}

func TestSaveRoundTripUsesWritableOverlay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slot-1.json")
	want := game.NewState()
	want.Screen = game.ScreenGround
	want.Cash = 19
	want.Gems = 1
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("save round trip mismatch: got=%+v want=%+v", got, want)
	}
}

func TestSaveRoundTripPreservesCommittedCharacterAndEmptyDraft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "character-slot.json")
	want := game.NewState()
	want.Screen = game.ScreenShipboard
	want.Character = "陽犬"
	want.CharacterDraft = ""
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("committed character save mismatch: got=%+v want=%+v", got, want)
	}
}

func TestLoadRejectsPrototypeV1Save(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-slot.json")
	legacy := `{"format":"sundog-remake-prototype-save-v1","state":{"schema":1,"screen":"shipboard","character":"Zed","location":"Drahew spaceport","position":{"x":3,"y":2},"cash":10,"bank":20,"gems":1,"timeMinutes":5,"theme":"apple-ii-original","messageKey":"ui.message.ready"}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	assertOldSaveUnchanged(t, path, []byte(legacy))
}

func TestLoadRejectsPrototypeV2Save(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v2-slot.json")
	legacy := `{"format":"sundog-remake-prototype-save-v2","state":{"schema":2,"screen":"shipboard","character":"Zed","location":"Drahew spaceport","position":{"x":3,"y":2},"cash":10,"bank":20,"gems":1,"fuel":55,"weaponCharge":40,"shieldLevel":30,"hullCondition":20,"shieldsActive":false,"timeMinutes":5,"theme":"apple-ii-original","messageKey":"ui.message.ready"}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	assertOldSaveUnchanged(t, path, []byte(legacy))
}

func TestLoadRejectsPrototypeV4SaveToCharacterDraftSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v4-slot.json")
	legacy := `{"format":"sundog-remake-prototype-save-v4","state":{"schema":4,"screen":"shipboard","character":"Zed","location":"Drahew spaceport","cash":10,"bank":20,"theme":"apple-ii-original","messageKey":"ui.message.ready"}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	assertOldSaveUnchanged(t, path, []byte(legacy))
}

func TestLoadRejectsPrototypeV5SaveToSixShipSystems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v5-slot.json")
	legacy := `{"format":"sundog-remake-prototype-save-v5","state":{"schema":5,"screen":"shipboard","character":"Zed","location":"Drahew spaceport","cash":10,"bank":20,"theme":"apple-ii-original","messageKey":"ui.message.ready"}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	assertOldSaveUnchanged(t, path, []byte(legacy))
}

func TestSaveRoundTripPreservesWorldFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "world-slot.json")
	want := game.NewState()
	want.Screen = game.ScreenOutcome
	want.OutcomeKey = "ui.outcome.success"
	want.SystemID = "jond"
	want.PlanetID = "jond"
	want.CityID = "jond-banville"
	want.Location = "Jond[d]／Banville（候選目標）"
	want.Bank = 100
	want.BankBalances["jond"] = 100
	want.VisitedDestinations = append(want.VisitedDestinations, "jond-banville")
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("world save round trip mismatch: got=%+v want=%+v", got, want)
	}
}

func TestLoadRejectsPrototypeV6SaveToSourceBackedWorld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v6-slot.json")
	// A schema 6 save made on the retired prototype geography: the player is
	// at an invented city, holds money in two invented systems and has the
	// invented mission destination selected.
	legacy := `{"format":"sundog-remake-prototype-save-v6","state":{"schema":6,"screen":"shipboard",` +
		`"character":"陽犬","characterDraft":"陽犬","location":"Kessel／Vesta Station",` +
		`"systemId":"kessel","planetId":"vesta","cityId":"kessel-vesta","position":{"x":3,"y":2},` +
		`"cash":50,"bank":400,"bankBalances":{"kessel":400,"drahew":150},` +
		`"visitedDestinations":["drahew-spaceport","kessel-kesselport","kessel-vesta"],` +
		`"navigationTargetId":"jond-banville","navigationTarget":"Jond[d]／Banville（候選目標）",` +
		`"theme":"apple-ii-original","messageKey":"ui.message.ready"}}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	assertOldSaveUnchanged(t, path, []byte(legacy))
}

// TestCurrentFormatCarriesTheCurrentSchema keeps the envelope name and the
// state schema from drifting apart. When they do, a save written by one build
// is silently unreadable by the next.
func TestCurrentFormatCarriesTheCurrentSchema(t *testing.T) {
	schema, known := saveFormats[currentFormat]
	if !known {
		t.Fatalf("current save format %q is not listed in saveFormats", currentFormat)
	}
	if schema != game.CurrentSchema {
		t.Fatalf("current save format carries schema %d, want %d", schema, game.CurrentSchema)
	}
}

// The user chose a strict new-save boundary; old source identities are not migrated.
func TestPreviousFormatIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v7-slot.json")
	legacy := `{"format":"sundog-remake-prototype-save-v7","state":{"schema":7,"screen":"shipboard","character":"Zed","cash":10,"bank":20,"gems":1,"cargoItemKey":"apple2.cargo.gems_crystals","theme":"apple-ii-original","messageKey":"ui.message.ready"}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	assertOldSaveUnchanged(t, path, []byte(legacy))
}

// 正式 typed 需求與待確認貨槽逐期保存，非正常玩家取得/九期GUI驗收。
func TestColonyAndPendingSaleSaveContracts(t *testing.T) {
	r, err := game.NewReducer()
	if err != nil {
		t.Fatal(err)
	}
	s := game.NewState()
	s.CityID = world.MissionDestinationID
	s.Screen = game.ScreenTrade
	s.TradeSession = game.TradeSession{Active: true, CityID: s.CityID, SystemID: s.SystemID, InitialCash: s.Cash, InitialBank: s.Bank, Funds: s.Cash + s.Bank}
	s.Bank = 0
	s.Quotes = make([]game.Lot, game.QuoteRing)
	s.Colony = game.ColonyProgress{Initialized: true, Phase: 0, NextPhase: 1}
	s.Colony.Remaining[8] = 1
	s.Holds[0] = game.Hold{Kind: game.HoldGoods, Goods: 8, Quality: 3, ItemKey: "fixture", SourceCode: 1, Price: 100}
	s.TradePage, s.TradeSaleSlot, s.TradeSalePrice = game.TradePageSellConfirm, 5, 100
	path := filepath.Join(t.TempDir(), "colony.json")
	if err = Save(path, s); err != nil {
		t.Fatal(err)
	}
	sameTrade := func(a, b game.State) bool {
		return a.Colony == b.Colony && a.TradeSaleSlot == b.TradeSaleSlot && a.TradeSalePrice == b.TradeSalePrice && a.TradePage == b.TradePage &&
			a.TradeSession == b.TradeSession && a.Holds == b.Holds && a.Cash == b.Cash && a.Bank == b.Bank && reflect.DeepEqual(a.Warehouses, b.Warehouses)
	}
	loaded, err := Load(path)
	if err != nil || !sameTrade(loaded, s) {
		t.Fatalf("pending sale save: %v", err)
	}
	library := filepath.Join(t.TempDir(), "library.json")
	if err = MakeLibrary(library); err == nil {
		err = StoreGame(library, "slot-1", s)
	}
	if err != nil {
		t.Fatal(err)
	}
	lib, err := LoadGame(library, "slot-1")
	if err != nil || lib.Colony != s.Colony || lib.TradeSaleSlot != 5 || lib.TradeSalePrice != 100 || lib.Holds != s.Holds {
		t.Fatalf("pending sale library: %v", err)
	}
	approved, err := r.Step(loaded, game.ActionTradePick1)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Colony.Phase != 1 || approved.Colony.Accumulated != 0 || game.TradeFunds(approved) != game.TradeFunds(s)+5000 {
		t.Fatal("saved approval failed phase payout")
	}
	if err = Save(path, approved); err != nil {
		t.Fatal(err)
	}
	after, err := Load(path)
	if err != nil || !sameTrade(after, approved) {
		t.Fatalf("phase payout save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var base map[string]any
	if err = json.Unmarshal(data, &base); err != nil {
		t.Fatal(err)
	}
	mutations := []func(map[string]any){
		func(d map[string]any) { delete(d["state"].(map[string]any), "colony") },
		func(d map[string]any) { delete(d["state"].(map[string]any), "tradeSaleSlot") },
		func(d map[string]any) { delete(d["state"].(map[string]any), "tradeSalePrice") },
		func(d map[string]any) { d["state"].(map[string]any)["colony"] = nil },
		func(d map[string]any) { d["state"].(map[string]any)["colony"].(map[string]any)["nextPhase"] = nil },
		func(d map[string]any) { d["state"].(map[string]any)["colony"].(map[string]any)["remaining"] = []int{0} },
		func(d map[string]any) {
			d["state"].(map[string]any)["colony"].(map[string]any)["remaining"].([]any)[0] = nil
		},
		func(d map[string]any) {
			d["state"].(map[string]any)["colony"].(map[string]any)["remaining"].([]any)[1] = 7.0
		},
		func(d map[string]any) { d["state"].(map[string]any)["colony"].(map[string]any)["phase"] = 10.0 },
		func(d map[string]any) { d["state"].(map[string]any)["colony"].(map[string]any)["accumulated"] = -1.0 },
		func(d map[string]any) { d["state"].(map[string]any)["tradeSaleSlot"] = 7.0 },
		func(d map[string]any) { d["format"] = formatV37 },
	}
	for i, mutate := range mutations {
		var d map[string]any
		if err = json.Unmarshal(data, &d); err != nil {
			t.Fatal(err)
		}
		mutate(d)
		raw, _ := json.Marshal(d)
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = Load(path); err == nil {
			t.Fatalf("invalid colony mutation %d accepted", i)
		}
	}
}

func TestColonyGrowthSurvivesSaveAndLibrary(t *testing.T) {
	for _, phase := range []int{0, 1, 4, 7, 8, 9} {
		for _, library := range []bool{false, true} {
			s := game.NewState()
			s.CityID = world.MissionDestinationID
			s.Position = game.Position{X: 9, Y: 6}
			s.Screen = game.ScreenGround
			for next := 0; next <= phase; next++ {
				s.Colony.Remaining = [32]int{}
				if _, err := s.Colony.AdvanceAtExchange(); err != nil {
					t.Fatal(err)
				}
			}
			s.MissionComplete = s.Colony.Complete()
			path := filepath.Join(t.TempDir(), "growth.json")
			var got game.State
			var err error
			if library {
				err = MakeLibrary(path)
				if err == nil {
					err = StoreGame(path, "slot-1", s)
				}
				if err == nil {
					got, err = LoadGame(path, "slot-1")
				}
			} else {
				err = Save(path, s)
				if err == nil {
					got, err = Load(path)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Colony != s.Colony || got.Position != s.Position || got.CombatRolls != s.CombatRolls {
				t.Fatal("重開改變成長或RNG")
			}
			w, h := game.GroundCitySize(s, s.CityID)
			gw, gh := game.GroundCitySize(got, got.CityID)
			if w != gw || h != gh {
				t.Fatal("重開改變城市尺寸")
			}
			for y := 0; y < 12; y++ {
				for x := 0; x < 20; x++ {
					before, _, _ := game.GroundTerrainCode(s, s.CityID, x, y)
					after, _, _ := game.GroundTerrainCode(got, got.CityID, x, y)
					if before != after {
						t.Fatal("重開改變城市圖塊")
					}
				}
			}
		}
	}
}
