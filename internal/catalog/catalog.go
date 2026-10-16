// Package catalog loads player-visible localization overlays.
package catalog

import (
	"encoding/json"
	"fmt"
	"io"
)

type Entry struct {
	Key         string `json:"key"`
	Source      string `json:"source,omitempty"`
	Translation string `json:"translation"`
	Status      string `json:"translationStatus,omitempty"`
}

type Document struct {
	Schema  int     `json:"schema"`
	Locale  string  `json:"locale"`
	Entries []Entry `json:"entries"`
}

type Catalog struct {
	Locale  string
	Entries map[string]Entry
}

func Load(r io.Reader) (Catalog, error) {
	var document Document
	if err := json.NewDecoder(r).Decode(&document); err != nil {
		return Catalog{}, fmt.Errorf("decode catalog: %w", err)
	}
	if document.Locale == "" {
		return Catalog{}, fmt.Errorf("catalog locale is empty")
	}
	result := Catalog{Locale: document.Locale, Entries: make(map[string]Entry, len(document.Entries))}
	for _, entry := range document.Entries {
		if entry.Key == "" {
			return Catalog{}, fmt.Errorf("catalog contains empty key")
		}
		if entry.Translation == "" {
			return Catalog{}, fmt.Errorf("catalog entry %q has empty translation", entry.Key)
		}
		if _, exists := result.Entries[entry.Key]; exists {
			return Catalog{}, fmt.Errorf("duplicate catalog key %q", entry.Key)
		}
		result.Entries[entry.Key] = entry
	}
	return result, nil
}

func (c Catalog) Merge(other Catalog) (Catalog, error) {
	if c.Locale != "" && other.Locale != "" && c.Locale != other.Locale {
		return Catalog{}, fmt.Errorf("catalog locale mismatch: %q versus %q", c.Locale, other.Locale)
	}
	locale := c.Locale
	if locale == "" {
		locale = other.Locale
	}
	entries := make(map[string]Entry, len(c.Entries)+len(other.Entries))
	for key, entry := range c.Entries {
		entries[key] = entry
	}
	for key, entry := range other.Entries {
		if _, exists := entries[key]; exists {
			return Catalog{}, fmt.Errorf("duplicate merged catalog key %q", key)
		}
		entries[key] = entry
	}
	return Catalog{Locale: locale, Entries: entries}, nil
}

func (c Catalog) Lookup(key string) (string, bool) {
	entry, ok := c.Entries[key]
	if !ok {
		return "", false
	}
	return entry.Translation, true
}
