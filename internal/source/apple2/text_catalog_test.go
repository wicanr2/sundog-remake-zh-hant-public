package apple2

import (
	"os"
	"strings"
	"testing"
)

func TestLoadTextCatalogRealManifest(t *testing.T) {
	data, err := os.ReadFile("../../../docs/i18n/apple2-text-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadTextCatalog(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Entries) != 46 {
		t.Fatalf("entry count = %d, want 46", len(catalog.Entries))
	}
	if got := len(catalog.KeysWithPrefix("apple2.cargo.")); got != 16 {
		t.Fatalf("cargo entry count = %d, want 16", got)
	}
	if got := len(catalog.KeysWithPrefix("apple2.equipment.")); got != 16 {
		t.Fatalf("equipment entry count = %d, want 16", got)
	}
	entry, ok := catalog.Entry("apple2.cargo.gems_crystals")
	if !ok {
		t.Fatal("gems/crystals entry missing")
	}
	if entry.RawOffset != 0x5130 || entry.Length != 13 || entry.Translation != "寶石／晶體" {
		t.Fatalf("unexpected gems entry: %+v", entry)
	}
	if _, ok := catalog.Localized("apple2.cargo.nothing"); !ok {
		t.Fatal("nothing translation missing")
	}
}

func TestLoadTextCatalogRejectsSourceDrift(t *testing.T) {
	const document = `{"entries":[{"key":"apple2.cargo.example","source":"cargo","translation":"貨物","offset":"0x100","length":99}]}`
	if _, err := LoadTextCatalog(strings.NewReader(document)); err == nil {
		t.Fatal("expected source length error")
	}
}

func TestKeysWithPrefixAreRawOffsetOrdered(t *testing.T) {
	const document = `{"entries":[{"key":"apple2.cargo.late","source":"late","translation":"晚","offset":"0x200","length":4},{"key":"apple2.cargo.early","source":"early","translation":"早","offset":"0x100","length":5}]}`
	catalog, err := LoadTextCatalog(strings.NewReader(document))
	if err != nil {
		t.Fatal(err)
	}
	got := catalog.KeysWithPrefix("apple2.cargo.")
	want := []string{"apple2.cargo.early", "apple2.cargo.late"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("keys = %#v, want %#v", got, want)
	}
}
