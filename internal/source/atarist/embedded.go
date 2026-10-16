package atarist

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"
)

// worldDataJSON is a byte-identical copy of docs/atari-st/world-data.json.
// The evidence document under docs/ stays the single source of truth; this copy
// exists only because go:embed cannot reach outside the package directory.
// tools/validate_atari_st_world_data.py fails closed if the two ever diverge.
//
//go:embed assets/world-data.json
var worldDataJSON []byte

var (
	embeddedOnce sync.Once
	embeddedData WorldData
	embeddedErr  error
)

// EmbeddedWorldData returns the validated Atari ST world tables shipped with
// the binary. It never reads the original disk image: the player is not asked
// to supply source media for the remake to start.
func EmbeddedWorldData() (WorldData, error) {
	embeddedOnce.Do(func() {
		if len(worldDataJSON) == 0 {
			embeddedErr = fmt.Errorf("embedded Atari ST world data is empty")
			return
		}
		embeddedData, embeddedErr = LoadWorldData(bytes.NewReader(worldDataJSON))
	})
	return embeddedData, embeddedErr
}

// dialogTextJSON is a byte-identical copy of docs/atari-st/dialog-text.json,
// for the same go:embed reason as the world tables above.
//
//go:embed assets/dialog-text.json
var dialogTextJSON []byte

var (
	dialogOnce sync.Once
	dialogData DialogData
	dialogErr  error
)

// EmbeddedDialogData returns the validated Atari ST dialog corpus shipped with
// the binary. Like the world tables, it never reads the original disk image.
func EmbeddedDialogData() (DialogData, error) {
	dialogOnce.Do(func() {
		if len(dialogTextJSON) == 0 {
			dialogErr = fmt.Errorf("embedded Atari ST dialog data is empty")
			return
		}
		dialogData, dialogErr = LoadDialogData(bytes.NewReader(dialogTextJSON))
	})
	return dialogData, dialogErr
}

// dialogTranslationJSON is a byte-identical copy of
// docs/i18n/atari-st-dialog-zh-Hant.json, for the same go:embed reason.
//
//go:embed assets/dialog-zh-Hant.json
var dialogTranslationJSON []byte

var (
	translatedOnce sync.Once
	translatedData DialogData
	translatedErr  error
)

// EmbeddedTranslatedDialogData returns the dialog corpus with every string
// replaced by its Traditional Chinese translation.
func EmbeddedTranslatedDialogData() (DialogData, error) {
	translatedOnce.Do(func() {
		source, err := EmbeddedDialogData()
		if err != nil {
			translatedErr = err
			return
		}
		translatedData, translatedErr = source.Translate(bytes.NewReader(dialogTranslationJSON))
	})
	return translatedData, translatedErr
}

// itemDataJSON is a byte-identical copy of docs/atari-st/item-data.json.
//
//go:embed assets/item-data.json
var itemDataJSON []byte

var (
	itemOnce sync.Once
	itemData ItemData
	itemErr  error
)

// EmbeddedItemData returns the validated item and cargo tables.
func EmbeddedItemData() (ItemData, error) {
	itemOnce.Do(func() {
		if len(itemDataJSON) == 0 {
			itemErr = fmt.Errorf("embedded Atari ST item data is empty")
			return
		}
		itemData, itemErr = LoadItemData(bytes.NewReader(itemDataJSON))
	})
	return itemData, itemErr
}

// shipSystemsJSON is a byte-identical copy of docs/atari-st/ship-systems.json.
//
//go:embed assets/ship-systems.json
var shipSystemsJSON []byte

var (
	shipSystemsOnce sync.Once
	shipSystems     ShipSystemData
	shipSystemsErr  error
)

// EmbeddedShipSystemData returns the validated ship system part composition.
func EmbeddedShipSystemData() (ShipSystemData, error) {
	shipSystemsOnce.Do(func() {
		if len(shipSystemsJSON) == 0 {
			shipSystemsErr = fmt.Errorf("embedded Atari ST ship system data is empty")
			return
		}
		shipSystems, shipSystemsErr = LoadShipSystemData(bytes.NewReader(shipSystemsJSON))
	})
	return shipSystems, shipSystemsErr
}

// loadoutJSON is a byte-identical copy of docs/atari-st/new-game-loadout.json.
//
//go:embed assets/new-game-loadout.json
var loadoutJSON []byte

var (
	loadoutOnce sync.Once
	loadout     LoadoutData
	loadoutErr  error
)

// EmbeddedLoadoutData returns the validated new-game loadout.
func EmbeddedLoadoutData() (LoadoutData, error) {
	loadoutOnce.Do(func() {
		if len(loadoutJSON) == 0 {
			loadoutErr = fmt.Errorf("embedded Atari ST loadout data is empty")
			return
		}
		loadout, loadoutErr = LoadLoadoutData(bytes.NewReader(loadoutJSON))
	})
	return loadout, loadoutErr
}

// shipRulesJSON is a byte-identical copy of docs/atari-st/ship-rules.json.
//
//go:embed assets/ship-rules.json
var shipRulesJSON []byte

var (
	shipRulesOnce sync.Once
	shipRules     RuleData
	shipRulesErr  error
)

// EmbeddedRuleData returns the validated ship combat and flight rules.
func EmbeddedRuleData() (RuleData, error) {
	shipRulesOnce.Do(func() {
		if len(shipRulesJSON) == 0 {
			shipRulesErr = fmt.Errorf("embedded Atari ST ship rules are empty")
			return
		}
		shipRules, shipRulesErr = LoadRuleData(bytes.NewReader(shipRulesJSON))
	})
	return shipRules, shipRulesErr
}

// cityMapsJSON is a byte-identical copy of docs/atari-st/city-maps.json.
//
//go:embed assets/city-maps.json
var cityMapsJSON []byte

var (
	cityMapsOnce sync.Once
	cityMaps     CityMapData
	cityMapsErr  error
)

// EmbeddedCityMapData returns the validated 60 city ground maps.
func EmbeddedCityMapData() (CityMapData, error) {
	cityMapsOnce.Do(func() {
		if len(cityMapsJSON) == 0 {
			cityMapsErr = fmt.Errorf("embedded Atari ST city maps are empty")
			return
		}
		cityMaps, cityMapsErr = LoadCityMapData(bytes.NewReader(cityMapsJSON))
	})
	return cityMaps, cityMapsErr
}

// enemyShipsJSON is a byte-identical copy of docs/atari-st/enemy-ships.json.
//
//go:embed assets/enemy-ships.json
var enemyShipsJSON []byte

var (
	enemyShipsOnce sync.Once
	enemyShips     EnemyShipData
	enemyShipsErr  error
)

// EmbeddedEnemyShipData returns the validated enemy ship table.
func EmbeddedEnemyShipData() (EnemyShipData, error) {
	enemyShipsOnce.Do(func() {
		if len(enemyShipsJSON) == 0 {
			enemyShipsErr = fmt.Errorf("embedded Atari ST enemy ship data is empty")
			return
		}
		enemyShips, enemyShipsErr = LoadEnemyShipData(bytes.NewReader(enemyShipsJSON))
	})
	return enemyShips, enemyShipsErr
}

// sineTableJSON is a byte-identical copy of docs/atari-st/sine-table.json.
//
//go:embed assets/sine-table.json
var sineTableJSON []byte

var (
	sineTableOnce sync.Once
	sineTable     SineTableData
	sineTableErr  error
)

// EmbeddedSineTableData returns the validated quarter sine table.
func EmbeddedSineTableData() (SineTableData, error) {
	sineTableOnce.Do(func() {
		if len(sineTableJSON) == 0 {
			sineTableErr = fmt.Errorf("embedded Atari ST sine table is empty")
			return
		}
		sineTable, sineTableErr = LoadSineTableData(bytes.NewReader(sineTableJSON))
	})
	return sineTable, sineTableErr
}

//go:embed assets/trade-classes.json
var tradeClassesJSON []byte

var (
	tradeClassesOnce sync.Once
	tradeClasses     TradeClassData
	tradeClassesErr  error
)

// EmbeddedTradeClassData returns the validated trade class table (logical
// block 7): one supply and demand class a goods, a planet a record.
func EmbeddedTradeClassData() (TradeClassData, error) {
	tradeClassesOnce.Do(func() {
		if len(tradeClassesJSON) == 0 {
			tradeClassesErr = fmt.Errorf("embedded Atari ST trade classes are empty")
			return
		}
		tradeClasses, tradeClassesErr = LoadTradeClassData(bytes.NewReader(tradeClassesJSON))
	})
	return tradeClasses, tradeClassesErr
}
