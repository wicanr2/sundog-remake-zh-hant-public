package atarist

import (
	"bytes"
	"testing"
)

func TestColonyPhaseSourceAndIsolation(t *testing.T) {
	phases, err := EmbeddedColonyPhases()
	if err != nil {
		t.Fatal(err)
	}
	if phases[0].Goods != [8]int{0, 0, 0, 0, 8, 9, 11, 13} || phases[2].RequiredCounts[31] != 4 || phases[8].Goods != [8]int{2, 7, 5, 24, 26, 28, 29, 31} {
		t.Fatal("原始首／中／末階段需求不符")
	}
	phases[2].RequiredCounts[31] = 0
	other, err := EmbeddedColonyPhases()
	if err != nil || other[2].RequiredCounts[31] != 4 {
		t.Fatal("來源被消費端修改")
	}
}

func TestColonyPhaseRejectsSourceDrift(t *testing.T) {
	for _, tc := range []struct{ name, old, new string }{
		{"schema", `"schema": 1`, `"schema": 2`},
		{"定位", `"rawOffset": "0x3cfd8"`, `"rawOffset": "0x3cfd9"`},
		{"原始字元", `084040404048494b4d`, `084040404048494b4e`},
		{"段", `"segment": "XSTARTUP"`, `"segment": "XMOVEONG"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := bytes.Replace(colonyPhasesJSON, []byte(tc.old), []byte(tc.new), 1)
			if bytes.Equal(changed, colonyPhasesJSON) {
				t.Fatal("測試未改到輸入")
			}
			if _, err := LoadColonyPhases(changed); err == nil {
				t.Fatal("接受了來源漂移")
			}
		})
	}
}
