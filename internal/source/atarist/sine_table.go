package atarist

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// The projection the original uses for in-system flight has one input beyond
// the position itself: a quarter sine table. SHIPLIB:0x0c polar_to_rect folds
// any angle into that quarter and reads a single byte from it, so the table is
// the whole of the trigonometry the game does.

// SineTableData is the validated quarter sine table.
type SineTableData struct {
	table     []int
	turnSteps int
	scale     int
}

type sineTableDocument struct {
	Schema    int   `json:"schema"`
	TurnSteps int   `json:"turnSteps"`
	Scale     int   `json:"scale"`
	Table     []int `json:"table"`
}

// LoadSineTableData validates the document. The check that earns its keep is
// the same one the decoder makes: every entry has to equal the rounded sine of
// its own angle. That is a comparison against a fact outside the document, so a
// table read from the wrong address cannot pass it.
func LoadSineTableData(reader io.Reader) (SineTableData, error) {
	var document sineTableDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return SineTableData{}, fmt.Errorf("decode Atari ST sine table: %w", err)
	}
	if document.Schema != 2 {
		return SineTableData{}, fmt.Errorf("unsupported Atari ST sine table schema %d", document.Schema)
	}
	if document.TurnSteps <= 0 || document.Scale <= 0 {
		return SineTableData{}, fmt.Errorf("sine table declares turn %d and scale %d", document.TurnSteps, document.Scale)
	}
	if len(document.Table) != document.TurnSteps/8+1 {
		return SineTableData{}, fmt.Errorf("sine table has %d entries for a %d-step turn",
			len(document.Table), document.TurnSteps)
	}
	for index, value := range document.Table {
		want := int(math.Round(float64(document.Scale) *
			math.Sin(float64(index)*math.Pi/(float64(document.TurnSteps)/4))))
		if value != want {
			return SineTableData{}, fmt.Errorf("sine table entry %d is %d, not %d", index, value, want)
		}
	}
	return SineTableData{
		table:     append([]int(nil), document.Table...),
		turnSteps: document.TurnSteps,
		scale:     document.Scale,
	}, nil
}

// TurnSteps is how many angle units make a full revolution.
func (d SineTableData) TurnSteps() int { return d.turnSteps }

// Scale is the value the table reaches at a quarter turn, and the divisor a
// projected component is divided by.
func (d SineTableData) Scale() int { return d.scale }

// Sine returns the table's value for an angle, folding it the way
// polar_to_rect does: halve, wrap into a half turn's worth of steps, then mirror
// anything past the quarter. The result is always the unsigned magnitude; the
// sign is decided by the caller, as it is in the original.
//
// The original's angle is a byte field, so it never sees a value outside one
// turn. Normalizing first is what makes that the same thing here rather than a
// difference nobody notices until an angle goes negative.
func (d SineTableData) Sine(angle int) int {
	if len(d.table) == 0 || d.turnSteps <= 0 {
		return 0
	}
	angle %= d.turnSteps
	if angle < 0 {
		angle += d.turnSteps
	}
	quarter := len(d.table) - 1
	index := (angle / 2) % (quarter * 2)
	if index > quarter {
		index = quarter*2 - index
	}
	return d.table[index]
}

// Table returns the raw quarter table.
func (d SineTableData) Table() []int {
	result := make([]int, len(d.table))
	copy(result, d.table)
	return result
}
