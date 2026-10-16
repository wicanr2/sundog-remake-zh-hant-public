package storage

// Library implements a remake-owned local replacement for the manual's
// Library Disk operations. It deliberately does not claim compatibility with
// the original disk format; original media remains outside the save overlay.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/wicanr2/sundog-remake/internal/game"
)

const libraryFormatV1 = "sundog-remake-prototype-library-v1"
const libraryFormatV2 = "sundog-remake-prototype-library-v2"

var (
	ErrLibraryNotFound = errors.New("remake library does not exist")
	ErrSlotNotFound    = errors.New("remake library slot does not exist")
)

type LibraryEnvelope struct {
	Format string                `json:"format"`
	Slots  map[string]game.State `json:"slots"`
}

// SlotSummary is what a slot list shows without loading the game: who is in
// it, where they are and what the ship's clock says. Enough to tell two saves
// apart, which is the whole point of having more than one.
type SlotSummary struct {
	ID          string `json:"id"`
	Character   string `json:"character"`
	Location    string `json:"location"`
	TimeMinutes int    `json:"timeMinutes"`
}

// MakeLibrary creates an empty writable library. An existing valid library is
// left unchanged so the operation is safe to repeat from the UI.
func MakeLibrary(path string) error {
	if path == "" {
		return fmt.Errorf("library path is empty")
	}
	if _, err := os.Stat(path); err == nil {
		_, err = readLibrary(path)
		return err
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect library: %w", err)
	}
	return writeLibrary(path, LibraryEnvelope{Format: libraryFormatV2, Slots: map[string]game.State{}})
}

func ListLibrary(path string) ([]SlotSummary, error) {
	library, err := readLibrary(path)
	if err != nil {
		return nil, err
	}
	result := make([]SlotSummary, 0, len(library.Slots))
	for id, state := range library.Slots {
		result = append(result, SlotSummary{
			ID:          id,
			Character:   state.Character,
			Location:    state.Location,
			TimeMinutes: state.TimeMinutes,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// StoreGame stores a copy of state in one named slot. The original state is
// not mutated. Menu-only screens are normalized to shipboard so a loaded
// stored game returns to a playable screen.
func StoreGame(path, slotID string, state game.State) error {
	if err := game.ValidateEnemyCourseState(state); err != nil {
		return err
	}
	if err := game.ValidateAwakeClockState(state); err != nil {
		return err
	}
	if err := game.ValidateSleepState(state); err != nil {
		return err
	}
	slotID, err := normalizeSlotID(slotID)
	if err != nil {
		return err
	}
	if state.Schema != game.CurrentSchema {
		return fmt.Errorf("library requires state schema %d, got %d", game.CurrentSchema, state.Schema)
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
	library, err := readLibrary(path)
	if err != nil {
		return err
	}
	stored := state
	if stored.Screen == game.ScreenCharacterLibrary && stored.LibraryReturn != "" {
		stored.Screen = stored.LibraryReturn
		stored.LibraryReturn = ""
	}
	if stored.Screen == game.ScreenTitle || stored.Screen == game.ScreenCharacterLibrary || stored.Screen == game.ScreenCharacterCreation {
		stored.Screen = game.ScreenShipboard
	}
	stored.Saved = false
	stored.QuitRequested = false
	stored.MessageKey = "ui.message.ready"
	if library.Slots == nil {
		library.Slots = make(map[string]game.State)
	}
	library.Slots[slotID] = stored
	return writeLibrary(path, library)
}

func LoadGame(path, slotID string) (game.State, error) {
	slotID, err := normalizeSlotID(slotID)
	if err != nil {
		return game.State{}, err
	}
	library, err := readLibrary(path)
	if err != nil {
		return game.State{}, err
	}
	state, ok := library.Slots[slotID]
	if !ok {
		return game.State{}, fmt.Errorf("%w: %s", ErrSlotNotFound, slotID)
	}
	state, err = game.Migrate(state)
	if err != nil {
		return game.State{}, fmt.Errorf("migrate library slot %s: %w", slotID, err)
	}
	return state, nil
}

func DeleteGame(path, slotID string) error {
	slotID, err := normalizeSlotID(slotID)
	if err != nil {
		return err
	}
	library, err := readLibrary(path)
	if err != nil {
		return err
	}
	if _, ok := library.Slots[slotID]; !ok {
		return fmt.Errorf("%w: %s", ErrSlotNotFound, slotID)
	}
	delete(library.Slots, slotID)
	return writeLibrary(path, library)
}

func readLibrary(path string) (LibraryEnvelope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LibraryEnvelope{}, ErrLibraryNotFound
		}
		return LibraryEnvelope{}, fmt.Errorf("read remake library: %w", err)
	}
	var raw struct {
		Format string                     `json:"format"`
		Slots  map[string]json.RawMessage `json:"slots"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return LibraryEnvelope{}, fmt.Errorf("decode remake library: %w", err)
	}
	if raw.Format != libraryFormatV2 {
		return LibraryEnvelope{}, fmt.Errorf("%w: %q", ErrOldSave, raw.Format)
	}
	library := LibraryEnvelope{Format: raw.Format, Slots: make(map[string]game.State)}
	for slotID, encoded := range raw.Slots {
		if _, err := normalizeSlotID(slotID); err != nil {
			return LibraryEnvelope{}, fmt.Errorf("invalid library slot %q: %w", slotID, err)
		}
		state, err := decodeCurrentState(encoded)
		if err != nil {
			return LibraryEnvelope{}, fmt.Errorf("library slot %s: %w", slotID, err)
		}
		library.Slots[slotID] = state
	}
	return library, nil
}

func writeLibrary(path string, library LibraryEnvelope) error {
	if library.Format == "" {
		library.Format = libraryFormatV2
	}
	if library.Slots == nil {
		library.Slots = make(map[string]game.State)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create library directory: %w", err)
	}
	data, err := json.MarshalIndent(library, "", "  ")
	if err != nil {
		return fmt.Errorf("encode remake library: %w", err)
	}
	data = append(data, '\n')
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write temporary remake library: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("commit remake library: %w", err)
	}
	return nil
}

func normalizeSlotID(slotID string) (string, error) {
	slotID = strings.TrimSpace(slotID)
	if slotID == "" {
		return "", fmt.Errorf("library slot is empty")
	}
	if len([]rune(slotID)) > 24 {
		return "", fmt.Errorf("library slot is too long")
	}
	for _, value := range slotID {
		if unicode.IsLetter(value) || unicode.IsDigit(value) || value == '-' || value == '_' {
			continue
		}
		return "", fmt.Errorf("library slot contains unsupported character %q", value)
	}
	return slotID, nil
}
