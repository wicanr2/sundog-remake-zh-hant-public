package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/wicanr2/sundog-remake/internal/source/apple2"
)

type tableSpec struct {
	Name          string
	RawOffset     int
	RecordSize    int
	RecordCount   int
	SemanticLabel string
	SemanticLevel apple2.EvidenceLevel
}

type document struct {
	Schema         int                     `json:"schema"`
	Tool           string                  `json:"tool"`
	Disk           diskRecord              `json:"disk"`
	Tables         []apple2.FixedNameTable `json:"tables"`
	Interpretation interpretation          `json:"interpretation"`
}

type diskRecord struct {
	Name   string `json:"name"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

type interpretation struct {
	Proven          []string `json:"proven"`
	StrongInference []string `json:"strongInference"`
	Unknown         []string `json:"unknown"`
}

func main() {
	diskPath := flag.String("disk", "SUNDOG1.DSK", "Apple II raw disk image supplied outside the repository")
	diskName := flag.String("disk-name", "SUNDOG1.DSK", "source disk name recorded in the manifest")
	outputPath := flag.String("output", "", "optional JSON output path; empty writes to stdout")
	flag.Parse()

	data, err := os.ReadFile(*diskPath)
	if err != nil {
		fatal(err)
	}
	specs := []tableSpec{
		{
			Name:          "star-system-name-candidate",
			RawOffset:     0x2100,
			RecordSize:    20,
			RecordCount:   13,
			SemanticLabel: "12 星系名稱候選；首尾重複的 Jondd 可能是循環表邊界",
			SemanticLevel: apple2.EvidenceStrongInference,
		},
		{
			Name:          "planet-name-candidate",
			RawOffset:     0x2f00,
			RecordSize:    16,
			RecordCount:   17,
			SemanticLabel: "17 筆固定 stride 的行星名稱候選；不證明完整行星表",
			SemanticLevel: apple2.EvidenceStrongInference,
		},
		{
			Name:          "city-name-candidate-block-1",
			RawOffset:     0x3010,
			RecordSize:    12,
			RecordCount:   20,
			SemanticLabel: "城市名稱候選區段；完整城市數量與尾端欄位語意未知",
			SemanticLevel: apple2.EvidenceHypothesis,
		},
	}

	tables := make([]apple2.FixedNameTable, 0, len(specs))
	for _, spec := range specs {
		table, err := apple2.ParseFixedNameTable(data, spec.Name, spec.RawOffset, spec.RecordSize, spec.RecordCount)
		if err != nil {
			fatal(err)
		}
		table.SemanticLabel = spec.SemanticLabel
		table.SemanticLevel = spec.SemanticLevel
		tables = append(tables, table)
	}

	report := document{
		Schema: 1,
		Tool:   "cmd/apple2-system-data",
		Disk: diskRecord{
			Name:   *diskName,
			Size:   len(data),
			SHA256: fmt.Sprintf("%x", sha256.Sum256(data)),
		},
		Tables: tables,
		Interpretation: interpretation{
			Proven: []string{
				"固定 raw offset、record size、record count 下的長度前綴與可列印名稱 bytes",
				"每筆 record 的完整 rawHex 與名稱後 tail bytes",
			},
			StrongInference: []string{
				"20-byte table 的 13 筆名稱具 12 個唯一值，且首尾 Jondd 重複，與循環星系表候選形狀相符",
				"16-byte table 在 17 筆後正好到達 0x3010 的下一個候選區段；其中名稱的語意仍待 live consumer 證實",
			},
			Unknown: []string{
				"tail bytes 的欄位語意、座標、索引與 route consumer",
				"名稱表是否由 SYSTEM.STARTUP／遊戲 code live 消費",
				"行星候選是否涵蓋完整行星表，以及城市候選區段是否涵蓋完整城市表",
			},
		},
	}
	var output io.Writer = os.Stdout
	var outputFile *os.File
	if *outputPath != "" {
		outputFile, err = os.OpenFile(*outputPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			fatal(err)
		}
		defer outputFile.Close()
		output = outputFile
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
