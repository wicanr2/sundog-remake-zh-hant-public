// Package storage keeps remake saves separate from pristine source media.
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wicanr2/sundog-remake/internal/game"
)

type Envelope struct {
	Format string     `json:"format"`
	State  game.State `json:"state"`
}

const (
	formatV1  = "sundog-remake-prototype-save-v1"
	formatV2  = "sundog-remake-prototype-save-v2"
	formatV3  = "sundog-remake-prototype-save-v3"
	formatV4  = "sundog-remake-prototype-save-v4"
	formatV5  = "sundog-remake-prototype-save-v5"
	formatV6  = "sundog-remake-prototype-save-v6"
	formatV7  = "sundog-remake-prototype-save-v7"
	formatV8  = "sundog-remake-prototype-save-v8"
	formatV9  = "sundog-remake-prototype-save-v9"
	formatV10 = "sundog-remake-prototype-save-v10"
	formatV11 = "sundog-remake-prototype-save-v11"
	formatV12 = "sundog-remake-prototype-save-v12"
	formatV13 = "sundog-remake-prototype-save-v13"
	formatV14 = "sundog-remake-prototype-save-v14"
	formatV15 = "sundog-remake-prototype-save-v15"
	formatV16 = "sundog-remake-prototype-save-v16"
	formatV17 = "sundog-remake-prototype-save-v17"
	formatV18 = "sundog-remake-prototype-save-v18"
	formatV19 = "sundog-remake-prototype-save-v19"
	formatV20 = "sundog-remake-prototype-save-v20"
	formatV21 = "sundog-remake-prototype-save-v21"
	formatV22 = "sundog-remake-prototype-save-v22"
	formatV23 = "sundog-remake-prototype-save-v23"
	formatV24 = "sundog-remake-prototype-save-v24"
	formatV25 = "sundog-remake-prototype-save-v25"
	formatV26 = "sundog-remake-prototype-save-v26"
	formatV27 = "sundog-remake-prototype-save-v27"
	formatV28 = "sundog-remake-prototype-save-v28"
	formatV29 = "sundog-remake-prototype-save-v29"
	formatV30 = "sundog-remake-prototype-save-v30"
	formatV31 = "sundog-remake-prototype-save-v31"
	formatV32 = "sundog-remake-prototype-save-v32"
	formatV33 = "sundog-remake-prototype-save-v33"
	formatV34 = "sundog-remake-prototype-save-v34"
	formatV35 = "sundog-remake-prototype-save-v35"
	formatV36 = "sundog-remake-prototype-save-v36"
	formatV37 = "sundog-remake-prototype-save-v37"
	formatV38 = "sundog-remake-prototype-save-v38"
	formatV39 = "sundog-remake-prototype-save-v39"
	formatV40 = "sundog-remake-prototype-save-v40"
	formatV41 = "sundog-remake-prototype-save-v41"
	formatV42 = "sundog-remake-prototype-save-v42"
	formatV43 = "sundog-remake-prototype-save-v43"
	formatV44 = "sundog-remake-prototype-save-v44"
)

// Only the current envelope is supported. Older format constants above remain
// available to rejection tests and do not grant a migration path.
var saveFormats = map[string]int{formatV44: game.CurrentSchema}

// currentFormat is the envelope a new save is written with.
const currentFormat = formatV44

func Save(path string, state game.State) error {
	if err := game.ValidateAwakeClockState(state); err != nil {
		return err
	}
	if err := game.ValidateSleepState(state); err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("save path is empty")
	}
	if state.Schema != game.CurrentSchema {
		return fmt.Errorf("save requires state schema %d, got %d", game.CurrentSchema, state.Schema)
	}
	if err := game.ValidateFuelState(state); err != nil {
		return err
	}
	if err := game.ValidateTradeState(state); err != nil {
		return err
	}
	if err := game.ValidateItemState(state); err != nil {
		return err
	}
	if err := game.ValidateAttributeState(state); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create save directory: %w", err)
	}
	envelope := Envelope{Format: currentFormat, State: state}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("encode save: %w", err)
	}
	data = append(data, '\n')
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write temporary save: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("commit save: %w", err)
	}
	return nil
}

func Load(path string) (game.State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return game.State{}, fmt.Errorf("read save: %w", err)
	}
	var envelope struct {
		Format string          `json:"format"`
		State  json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return game.State{}, fmt.Errorf("decode save: %w", err)
	}
	if envelope.Format != currentFormat {
		return game.State{}, fmt.Errorf("%w: %q", ErrOldSave, envelope.Format)
	}
	return decodeCurrentState(envelope.State)
}
