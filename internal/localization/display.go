// Package localization 提供 remake 玩家可見文字的共用查詢輔助。
package localization

import (
	"strings"

	"github.com/wicanr2/sundog-remake/internal/catalog"
)

// Lookup 以穩定 key 查詢繁中 overlay；找不到時保留來源 fallback，避免靜默改寫未知資料。
func Lookup(labels catalog.Catalog, key, fallback string) string {
	if value, ok := labels.Lookup(key); ok {
		return value
	}
	return fallback
}

// Destination 回傳目的地的繁中顯示名稱。
func Destination(labels catalog.Catalog, id, fallback string) string {
	return Lookup(labels, "ui.destination."+id, fallback)
}

// System 回傳星系的繁中顯示名稱。
func System(labels catalog.Catalog, id, fallback string) string {
	return Lookup(labels, "ui.system."+id, fallback)
}

// Planet 回傳行星的繁中顯示名稱。
func Planet(labels catalog.Catalog, id, fallback string) string {
	return Lookup(labels, "ui.planet."+id, fallback)
}

// City 回傳城市的繁中顯示名稱。
func City(labels catalog.Catalog, id, fallback string) string {
	return Lookup(labels, "ui.city."+id, fallback)
}

// LibrarySlotLabel hides the storage-only slot-N prefix from player-facing text.
func LibrarySlotLabel(id string) string {
	return strings.TrimPrefix(id, "slot-")
}
