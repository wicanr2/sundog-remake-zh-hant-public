package storage

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/wicanr2/sundog-remake/internal/game"
)

var ErrOldSave = errors.New("不支援舊存檔，請開始新遊戲")

// A forged current envelope must not silently default the new state. False,
// zero and empty dialog values are valid, but missing/null fields are not.
func decodeCurrentState(raw json.RawMessage) (game.State, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return game.State{}, err
	}
	require := func(object map[string]json.RawMessage, keys ...string) error {
		for _, key := range keys {
			v, ok := object[key]
			if !ok || string(v) == "null" {
				return fmt.Errorf("%w：缺少 %s", ErrOldSave, key)
			}
		}
		return nil
	}
	if err := require(fields, "schema", "fuel", "weaponCharge", "shieldLevel", "shieldSetting",
		"shieldsActive", "shipControls", "shipPhase", "currentSpeed", "targetSpeed",
		"groundMode", "cargoAttached", "cargoCityId", "cargoPosition", "tractorActive",
		"fuelRescuePending", "fuelRescueDue", "fuelJettisonPending", "shipRefresh", "sleep", "sleepTheftProtected", "awakeClock",
		"enemyClosing", "fuelPurchase", "holds", "warehouses", "tradeSession", "tradeClassShift", "colony", "colonySite", "tradeSaleSlot", "tradeSalePrice", "stores", "parts", "handItem", "handOrigin", "attributes", "attributeMax"); err != nil {
		return game.State{}, err
	}
	var clock map[string]json.RawMessage
	if err := json.Unmarshal(fields["awakeClock"], &clock); err != nil {
		return game.State{}, err
	}
	if err := require(clock, "source", "background", "ground", "shipActive", "minutePause", "shipPause"); err != nil {
		return game.State{}, err
	}
	for name, keys := range map[string][]string{
		"source":     {"ticks", "enabled", "eventCountdown"},
		"background": {"waitEvents", "slots"},
		"ground":     {"deadline", "updates"},
	} {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(clock[name], &object); err != nil {
			return game.State{}, err
		}
		if err := require(object, keys...); err != nil {
			return game.State{}, err
		}
		if name == "background" {
			var slots []map[string]json.RawMessage
			if err := json.Unmarshal(object["slots"], &slots); err != nil {
				return game.State{}, err
			}
			if len(slots) != 8 {
				return game.State{}, fmt.Errorf("invalid clock slot count")
			}
			for _, slot := range slots {
				if err := require(slot, "callback", "remaining", "period"); err != nil {
					return game.State{}, err
				}
			}
		}
	}
	for _, name := range []string{"attributes", "attributeMax"} {
		var values []json.RawMessage
		if err := json.Unmarshal(fields[name], &values); err != nil {
			return game.State{}, err
		}
		if len(values) != game.AttributeCount {
			return game.State{}, fmt.Errorf("invalid %s count", name)
		}
		for _, value := range values {
			if string(value) == "null" {
				return game.State{}, fmt.Errorf("invalid null %s byte", name)
			}
		}
	}
	for _, name := range []string{"stores", "parts"} {
		var items []json.RawMessage
		if err := json.Unmarshal(fields[name], &items); err != nil {
			return game.State{}, err
		}
		for _, item := range items {
			if string(item) == "null" {
				return game.State{}, fmt.Errorf("invalid null item in %s", name)
			}
		}
	}
	for name, keys := range map[string][]string{
		"sleep":         {"active", "forced", "wakeRequested", "minuteCycles", "shipCycles", "elapsedMinutes", "shipActive"},
		"handOrigin":    {"kind", "index"},
		"colony":        {"initialized", "phase", "nextPhase", "remaining", "accumulated"},
		"colonySite":    {"pending", "position"},
		"tradeSession":  {"active", "cityId", "systemId", "initialCash", "initialBank", "funds"},
		"shipControls":  {"warp", "subc", "guns", "tactical", "pilotage", "secondary"},
		"cargoPosition": {"x", "y"},
		"fuelPurchase": {"kind", "baseFuel", "initialQuantity", "quantity", "unitPrice",
			"funds", "initialBank", "initialCash", "sourceCode"},
	} {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(fields[name], &object); err != nil {
			return game.State{}, err
		}
		if err := require(object, keys...); err != nil {
			return game.State{}, err
		}
		if name == "colonySite" {
			var position map[string]json.RawMessage
			if err := json.Unmarshal(object["position"], &position); err != nil {
				return game.State{}, err
			}
			if err := require(position, "x", "y"); err != nil {
				return game.State{}, err
			}
		}
		if name == "colony" {
			var counts []json.RawMessage
			if err := json.Unmarshal(object["remaining"], &counts); err != nil {
				return game.State{}, err
			}
			if len(counts) != 32 {
				return game.State{}, fmt.Errorf("invalid colony demand count")
			}
			for _, count := range counts {
				if string(count) == "null" {
					return game.State{}, fmt.Errorf("invalid null colony demand")
				}
			}
		}
		if name == "shipControls" {
			var elements []json.RawMessage
			if err := json.Unmarshal(object["secondary"], &elements); err != nil {
				return game.State{}, err
			}
			for _, element := range elements {
				if string(element) == "null" {
					return game.State{}, fmt.Errorf("%w：缺少次系統控制值", ErrOldSave)
				}
			}
			var secondary []bool
			if err := json.Unmarshal(object["secondary"], &secondary); err != nil {
				return game.State{}, err
			}
			if len(secondary) != 6 {
				return game.State{}, fmt.Errorf("invalid secondary control count")
			}
		}
	}
	var holds []map[string]json.RawMessage
	if err := json.Unmarshal(fields["holds"], &holds); err != nil {
		return game.State{}, err
	}
	if len(holds) != game.CargoLots {
		return game.State{}, fmt.Errorf("invalid hold count")
	}
	for _, hold := range holds {
		if err := require(hold, "sourceCode"); err != nil {
			return game.State{}, err
		}
	}
	for _, name := range []string{"quotes", "warehouses"} {
		if raw, ok := fields[name]; ok && string(raw) != "null" {
			var groups [][]map[string]json.RawMessage
			if name == "quotes" {
				var lots []map[string]json.RawMessage
				if err := json.Unmarshal(raw, &lots); err != nil {
					return game.State{}, err
				}
				groups = append(groups, lots)
			} else {
				var warehouses map[string][]map[string]json.RawMessage
				if err := json.Unmarshal(raw, &warehouses); err != nil {
					return game.State{}, err
				}
				for _, lots := range warehouses {
					groups = append(groups, lots)
				}
			}
			for _, lots := range groups {
				for _, lot := range lots {
					if err := require(lot, "sourceCode"); err != nil {
						return game.State{}, err
					}
				}
			}
		}
	}
	var s game.State
	if err := json.Unmarshal(raw, &s); err != nil {
		return game.State{}, err
	}
	if s.Schema != game.CurrentSchema {
		return game.State{}, fmt.Errorf("%w：schema %d", ErrOldSave, s.Schema)
	}
	if err := game.ValidateFuelState(s); err != nil {
		return game.State{}, err
	}
	if err := game.ValidateItemState(s); err != nil {
		return game.State{}, err
	}
	return game.Migrate(s)
}
