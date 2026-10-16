package atarist

import (
	"encoding/json"
	"fmt"
	"io"
)

// The enemy ships are not in the game's data area. They sit on the disk's
// copy-protection track, reached through a heap block the main program fills
// with three physically-addressed sector reads; see
// tools/atari_st_enemy_ships.py for how that was traced, and
// docs/atari-st/ship-rules.md for the byte signature of each read site.
//
// Four fields per ship, one byte each, and the meaning of every one of them is
// fixed by where the game reads it rather than by a name in the file.

// EnemyShip is one enemy ship type.
type EnemyShip struct {
	Index       int
	Name        string
	WeaponPower int
	Armour      int
	Accuracy    int
	FireRate    int
	Outline     [][2]int
}

// EnemyShipData is the validated view of the enemy ship tables.
type EnemyShipData struct {
	ships []EnemyShip
}

type enemyShipDocument struct {
	Schema int `json:"schema"`
	Ships  []struct {
		Index       int    `json:"index"`
		Name        string `json:"name"`
		WeaponPower int    `json:"weaponPower"`
		Armour      int    `json:"armour"`
		Accuracy    int    `json:"accuracy"`
		FireRate    int    `json:"fireRate"`
	} `json:"ships"`
	Outlines []struct {
		Index  int     `json:"index"`
		Points [][]int `json:"points"`
	} `json:"outlines"`
	Coverage struct {
		Total int `json:"total"`
	} `json:"coverage"`
	Region struct {
		Size int `json:"size"`
	} `json:"region"`
}

// LoadEnemyShipData validates the document. Two checks earn their keep here.
// The first is that the decoded regions cover the source region exactly: the
// table has no length field anywhere, so total coverage is the only thing that
// says the boundaries were read right. The second is that the two fields the
// original divides by are never zero, because a zero there is not a weak ship
// but a crash.
func LoadEnemyShipData(reader io.Reader) (EnemyShipData, error) {
	var document enemyShipDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return EnemyShipData{}, fmt.Errorf("decode Atari ST enemy ships: %w", err)
	}
	if document.Schema != 1 {
		return EnemyShipData{}, fmt.Errorf("unsupported Atari ST enemy ship schema %d", document.Schema)
	}
	if len(document.Ships) == 0 {
		return EnemyShipData{}, fmt.Errorf("enemy ship document carries no ships")
	}
	if document.Coverage.Total != document.Region.Size {
		return EnemyShipData{}, fmt.Errorf("the decoded regions cover %d of %d bytes",
			document.Coverage.Total, document.Region.Size)
	}
	if len(document.Outlines) != len(document.Ships) {
		return EnemyShipData{}, fmt.Errorf("%d ships carry %d outlines",
			len(document.Ships), len(document.Outlines))
	}

	outlines := make(map[int][][2]int, len(document.Outlines))
	for _, outline := range document.Outlines {
		points := make([][2]int, 0, len(outline.Points))
		for _, point := range outline.Points {
			if len(point) != 2 {
				return EnemyShipData{}, fmt.Errorf("outline %d has a point with %d coordinates", outline.Index, len(point))
			}
			points = append(points, [2]int{point[0], point[1]})
		}
		if len(points) == 0 {
			return EnemyShipData{}, fmt.Errorf("outline %d is empty", outline.Index)
		}
		outlines[outline.Index] = points
	}

	data := EnemyShipData{ships: make([]EnemyShip, 0, len(document.Ships))}
	for position, entry := range document.Ships {
		if entry.Index != position {
			return EnemyShipData{}, fmt.Errorf("ship %d is out of order at position %d", entry.Index, position)
		}
		if entry.Name == "" {
			return EnemyShipData{}, fmt.Errorf("ship %d has no name", entry.Index)
		}
		if entry.Armour <= 0 {
			return EnemyShipData{}, fmt.Errorf("ship %d has armour %d, which the original divides by", entry.Index, entry.Armour)
		}
		if entry.FireRate <= 0 {
			return EnemyShipData{}, fmt.Errorf("ship %d has fire rate %d, which would never fire", entry.Index, entry.FireRate)
		}
		outline, ok := outlines[entry.Index]
		if !ok {
			return EnemyShipData{}, fmt.Errorf("ship %d has no outline", entry.Index)
		}
		data.ships = append(data.ships, EnemyShip{
			Index:       entry.Index,
			Name:        entry.Name,
			WeaponPower: entry.WeaponPower,
			Armour:      entry.Armour,
			Accuracy:    entry.Accuracy,
			FireRate:    entry.FireRate,
			Outline:     outline,
		})
	}
	return data, nil
}

// Ships returns every enemy ship type in table order.
func (d EnemyShipData) Ships() []EnemyShip {
	result := make([]EnemyShip, len(d.ships))
	copy(result, d.ships)
	return result
}

// Ship returns one enemy ship type by its table index.
func (d EnemyShipData) Ship(index int) (EnemyShip, bool) {
	if index < 0 || index >= len(d.ships) {
		return EnemyShip{}, false
	}
	return d.ships[index], true
}

// Count returns how many enemy ship types the table holds.
func (d EnemyShipData) Count() int { return len(d.ships) }
