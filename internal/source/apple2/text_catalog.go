package apple2

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// TextEntry is a typed, evidence-preserving Apple II source-text record. The
// offset and length remain part of the runtime input so a translated label
// cannot silently lose its source identity.
type TextEntry struct {
	Key               string `json:"key"`
	Source            string `json:"source"`
	Translation       string `json:"translation"`
	RawOffset         int    `json:"-"`
	Offset            string `json:"offset"`
	Length            int    `json:"length"`
	TranslationStatus string `json:"translationStatus"`
}

type textCatalogDocument struct {
	Entries []TextEntry `json:"entries"`
}

// TextCatalog is the typed view of the public Apple II source catalog. It is
// deliberately narrower than a filesystem parser: it validates the catalog's
// own record metadata, not the original disk image or its live consumer.
type TextCatalog struct {
	Entries []TextEntry
	byKey   map[string]TextEntry
}

// LoadTextCatalog validates source identity, byte length and raw offset
// metadata before the catalog can be used by a player-facing adapter.
func LoadTextCatalog(reader io.Reader) (TextCatalog, error) {
	var document textCatalogDocument
	if err := json.NewDecoder(reader).Decode(&document); err != nil {
		return TextCatalog{}, fmt.Errorf("decode Apple II text catalog: %w", err)
	}
	if len(document.Entries) == 0 {
		return TextCatalog{}, fmt.Errorf("Apple II text catalog contains no entries")
	}

	entries := make([]TextEntry, 0, len(document.Entries))
	byKey := make(map[string]TextEntry, len(document.Entries))
	for index, entry := range document.Entries {
		if !strings.HasPrefix(entry.Key, "apple2.") {
			return TextCatalog{}, fmt.Errorf("entry %d has non-Apple-II key %q", index, entry.Key)
		}
		if entry.Source == "" || entry.Translation == "" {
			return TextCatalog{}, fmt.Errorf("entry %q has empty source or translation", entry.Key)
		}
		if entry.Length <= 0 || len([]byte(entry.Source)) != entry.Length {
			return TextCatalog{}, fmt.Errorf("entry %q source byte length %d does not match declared length %d", entry.Key, len([]byte(entry.Source)), entry.Length)
		}
		offset, err := strconv.ParseInt(entry.Offset, 0, 64)
		if err != nil || offset < 0 {
			return TextCatalog{}, fmt.Errorf("entry %q has invalid raw offset %q", entry.Key, entry.Offset)
		}
		if _, exists := byKey[entry.Key]; exists {
			return TextCatalog{}, fmt.Errorf("duplicate Apple II text key %q", entry.Key)
		}
		entry.RawOffset = int(offset)
		entries = append(entries, entry)
		byKey[entry.Key] = entry
	}

	return TextCatalog{Entries: entries, byKey: byKey}, nil
}

// Entry returns a source record by its stable catalog key.
func (c TextCatalog) Entry(key string) (TextEntry, bool) {
	entry, ok := c.byKey[key]
	return entry, ok
}

// Localized returns the translated source label while retaining a boolean for
// callers that need an explicit missing-key fallback.
func (c TextCatalog) Localized(key string) (string, bool) {
	entry, ok := c.Entry(key)
	if !ok {
		return "", false
	}
	return entry.Translation, true
}

// KeysWithPrefix returns stable raw-offset order for a semantic source group,
// such as apple2.cargo. or apple2.equipment.
func (c TextCatalog) KeysWithPrefix(prefix string) []string {
	entries := make([]TextEntry, 0)
	for _, entry := range c.Entries {
		if strings.HasPrefix(entry.Key, prefix) {
			entries = append(entries, entry)
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].RawOffset < entries[j].RawOffset
	})
	keys := make([]string, len(entries))
	for index, entry := range entries {
		keys[index] = entry.Key
	}
	return keys
}
