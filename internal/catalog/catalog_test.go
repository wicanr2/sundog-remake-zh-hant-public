package catalog

import (
	"strings"
	"testing"
)

func TestLoadAndMerge(t *testing.T) {
	base, err := Load(strings.NewReader(`{"schema":1,"locale":"zh-Hant","entries":[{"key":"a","translation":"甲"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	overlay, err := Load(strings.NewReader(`{"schema":1,"locale":"zh-Hant","entries":[{"key":"b","translation":"乙"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	merged, err := base.Merge(overlay)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := merged.Lookup("b"); !ok || got != "乙" {
		t.Fatalf("Lookup(b) = %q, %v", got, ok)
	}
}

func TestDuplicateKeyRejected(t *testing.T) {
	_, err := Load(strings.NewReader(`{"schema":1,"locale":"zh-Hant","entries":[{"key":"a","translation":"甲"},{"key":"a","translation":"乙"}]}`))
	if err == nil {
		t.Fatal("expected duplicate key error")
	}
}
