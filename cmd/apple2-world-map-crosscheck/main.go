// Command apple2-world-map-crosscheck compares explicit Apple II disk-name
// candidates with the separately supplied SunDog map archive. It deliberately
// emits only names, disk offsets, hashes and evidence grades: neither Apple II
// RAM nor any original map image is a remake runtime input.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/wicanr2/sundog-remake/internal/source/apple2"
)

const (
	mapPlanetsHTML = "Sundog Maps/planets.htm"
	mapCitiesHTML  = "Sundog Maps/cities.htm"
)

var (
	imageSourcePattern = regexp.MustCompile(`(?i)src="([^"]+)"`)
	strongPattern      = regexp.MustCompile(`(?is)<strong>(.*?)</strong>`)
	tagPattern         = regexp.MustCompile(`(?is)<[^>]+>`)
)

type inputRecord struct {
	Name   string `json:"name"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

type inputs struct {
	Disk       inputRecord `json:"disk"`
	MapArchive inputRecord `json:"mapArchive"`
}

type nameRecord struct {
	Index     int    `json:"index"`
	RawOffset int    `json:"rawOffset"`
	Name      string `json:"name"`
}

type candidateTable struct {
	Name                string       `json:"name"`
	RawOffset           int          `json:"rawOffset"`
	RecordSize          int          `json:"recordSize"`
	RecordCount         int          `json:"recordCount"`
	Entries             []nameRecord `json:"entries"`
	ObservationEvidence string       `json:"observationEvidence"`
	SemanticLabel       string       `json:"semanticLabel"`
	SemanticEvidence    string       `json:"semanticEvidence"`
	ConsumerEvidence    string       `json:"consumerEvidence"`
	Notes               []string     `json:"notes"`
}

type diskCandidates struct {
	StarSystems candidateTable `json:"starSystems"`
	Planets     candidateTable `json:"planetNameCandidate"`
	Cities      candidateTable `json:"cityNameCandidateBlock1"`
}

type mapPlanetPair struct {
	System string `json:"system"`
	Planet string `json:"planet"`
}

type mapCityTriple struct {
	System string `json:"system"`
	Planet string `json:"planet"`
	City   string `json:"city"`
	Phase  string `json:"phase,omitempty"`
	Label  string `json:"label"`
}

type auxiliaryMap struct {
	PlanetPairs              []mapPlanetPair `json:"planetPairs"`
	CityTriples              []mapCityTriple `json:"cityTriples"`
	PlanetPairCount          int             `json:"planetPairCount"`
	CityTripleCount          int             `json:"cityTripleCount"`
	ObservationEvidence      string          `json:"observationEvidence"`
	RelationshipEvidence     string          `json:"relationshipEvidence"`
	OriginalConsumerEvidence string          `json:"originalConsumerEvidence"`
}

type nameCoverage struct {
	CandidateCount     int      `json:"candidateCount"`
	MapCount           int      `json:"mapCount"`
	MatchCount         int      `json:"matchCount"`
	MatchedCandidates  []string `json:"matchedCandidateNames"`
	CandidateOnlyNames []string `json:"candidateOnlyNames"`
	MapOnlyNames       []string `json:"mapOnlyNames"`
}

type crossChecks struct {
	Normalization string       `json:"normalization"`
	StarSystems   nameCoverage `json:"starSystemNames"`
	Planets       nameCoverage `json:"planetNames"`
	Cities        nameCoverage `json:"cityCandidateNames"`
}

type interpretation struct {
	Proven          []string `json:"proven"`
	StrongInference []string `json:"strongInference"`
	Unknown         []string `json:"unknown"`
	Correction      []string `json:"correction"`
}

type document struct {
	Schema          int            `json:"schema"`
	Tool            string         `json:"tool"`
	Inputs          inputs         `json:"inputs"`
	DiskCandidates  diskCandidates `json:"diskCandidates"`
	AuxiliaryMap    auxiliaryMap   `json:"auxiliaryMap"`
	CrossChecks     crossChecks    `json:"crossChecks"`
	Interpretation  interpretation `json:"interpretation"`
	RuntimeBoundary string         `json:"runtimeBoundary"`
}

type tableSpec struct {
	Name          string
	RawOffset     int
	RecordSize    int
	RecordCount   int
	SemanticLabel string
	SemanticLevel apple2.EvidenceLevel
	Notes         []string
}

func main() {
	diskPath := flag.String("disk", "SUNDOG1.DSK", "Apple II raw disk image supplied outside the repository")
	diskName := flag.String("disk-name", "SUNDOG1.DSK", "source disk name recorded in the evidence")
	mapArchivePath := flag.String("map-archive", "Sundog-Frozen-Legacy_Map_Atari-ST_EN.zip", "SunDog auxiliary map archive supplied outside the repository")
	outputPath := flag.String("output", "", "optional JSON output path; empty writes to stdout")
	flag.Parse()

	disk, err := os.ReadFile(*diskPath)
	if err != nil {
		fatal(err)
	}
	mapArchive, err := os.ReadFile(*mapArchivePath)
	if err != nil {
		fatal(err)
	}
	report, err := buildDocument(disk, *diskName, mapArchive, path.Base(*mapArchivePath))
	if err != nil {
		fatal(err)
	}
	if err := writeDocument(report, *outputPath); err != nil {
		fatal(err)
	}
}

func buildDocument(disk []byte, diskName string, mapArchive []byte, mapArchiveName string) (document, error) {
	candidates, err := parseDiskCandidates(disk)
	if err != nil {
		return document{}, err
	}
	mapCatalog, err := parseAuxiliaryMap(mapArchive)
	if err != nil {
		return document{}, err
	}

	systemRecords := uniqueRecords(candidates.StarSystems.Entries)
	planetRecords := uniqueRecords(candidates.Planets.Entries)
	cityRecords := uniqueRecords(candidates.Cities.Entries)
	mapSystems := make([]string, 0, len(mapCatalog.PlanetPairs))
	mapPlanets := make([]string, 0, len(mapCatalog.PlanetPairs))
	mapCities := make([]string, 0, len(mapCatalog.CityTriples))
	for _, pair := range mapCatalog.PlanetPairs {
		mapSystems = append(mapSystems, pair.System)
		mapPlanets = append(mapPlanets, pair.Planet)
	}
	for _, triple := range mapCatalog.CityTriples {
		mapCities = append(mapCities, triple.City)
	}

	return document{
		Schema: 1,
		Tool:   "cmd/apple2-world-map-crosscheck",
		Inputs: inputs{
			Disk:       identity(diskName, disk),
			MapArchive: identity(mapArchiveName, mapArchive),
		},
		DiskCandidates: candidates,
		AuxiliaryMap:   mapCatalog,
		CrossChecks: crossChecks{
			Normalization: "ASCII 大小寫不敏感；比較時移除空白、撇號與句點，只用於名稱交叉比對，不推定拼寫變體或關係。",
			StarSystems:   compareNames(systemRecords, mapSystems),
			Planets:       compareNames(planetRecords, mapPlanets),
			Cities:        compareNames(cityRecords, mapCities),
		},
		Interpretation: interpretation{
			Proven: []string{
				"三個 Apple II 固定 stride 候選的 raw offset、record size、record count 與長度前綴名稱 bytes。",
				"輔助地圖 archive 的雜湊，以及 planets.htm 中的 18 個影像名稱 pair、cities.htm 中的 58 個三層標籤。",
				"0x2f00 + 17 × 16 = 0x3010；第 18 個舊記錄會與城市候選區段第一筆重疊。",
			},
			StrongInference: []string{
				"12 個唯一 Apple II 星系名稱可在輔助地圖的 12 個星系標籤全部以正規化名稱對上。",
				"17 個 Apple II 行星候選中有 16 個可在輔助地圖的 18 個行星標籤對上；差異保持為資料版本或語意未知。",
				"20 個 Apple II 城市候選中有 19 個可在輔助地圖的 58 個城市標籤對上；這只支持候選名稱角色，不能證明完整城市表。",
				"輔助地圖的系統／行星／城市三層標籤可作 world catalog 研究線索，但不是 Apple II live consumer 證明。",
			},
			Unknown: []string{
				"輔助地圖與 Apple II v2.0 的精確版本關係、所有拼寫差異、完整城市覆蓋與城市座標。",
				"Apple II record tail bytes 的欄位語意、星系／行星／城市的原版 runtime consumer、路線、價格、設施與規則。",
				"這些名稱是否能安全取代 remake prototype world；在原版導航 consumer 被證實前，答案仍為否。",
			},
			Correction: []string{
				"舊的 16-byte／18 筆『行星名稱候選』結論已撤回：第 18 筆從 0x3010 開始，與 12-byte 城市候選區段第一筆相同。",
				"保留原始 18 筆輸出作歷史證據，但後續 extractor 與新交叉檢查固定只讀 17 筆，不再以手冊的 18 顆行星數量作為 table count 證明。",
			},
		},
		RuntimeBoundary: "此文件只保存持久磁碟 offset、名稱、雜湊與證據分級；不讀取、不模擬、不保存 Apple II RAM／HGR／6502 CPU state，也不把它們帶入 remake runtime、存檔或發行內容。",
	}, nil
}

func parseDiskCandidates(disk []byte) (diskCandidates, error) {
	specs := []tableSpec{
		{
			Name:          "star-system-name-candidate",
			RawOffset:     0x2100,
			RecordSize:    20,
			RecordCount:   13,
			SemanticLabel: "12 個唯一星系名稱候選；首尾 Jondd 重複可能是循環表邊界",
			SemanticLevel: apple2.EvidenceStrongInference,
			Notes: []string{
				"13 筆固定 record 的名稱 bytes 已證實；首尾重複的 table 邊界與 live consumer 未證實。",
			},
		},
		{
			Name:          "planet-name-candidate",
			RawOffset:     0x2f00,
			RecordSize:    16,
			RecordCount:   17,
			SemanticLabel: "17 筆固定 stride 的行星名稱候選；不證明完整行星表",
			SemanticLevel: apple2.EvidenceStrongInference,
			Notes: []string{
				"第 18 個 16-byte span 會從 0x3010 開始，與城市候選區段第一筆重疊，故排除。",
			},
		},
		{
			Name:          "city-name-candidate-block-1",
			RawOffset:     0x3010,
			RecordSize:    12,
			RecordCount:   20,
			SemanticLabel: "第一個城市名稱候選區段；完整城市表與尾端欄位語意未知",
			SemanticLevel: apple2.EvidenceStrongInference,
			Notes: []string{
				"這 20 筆名稱與輔助地圖的城市標籤有交叉，但不代表完整城市總表或原版 runtime consumer。",
			},
		},
	}

	parsed := make(map[string]candidateTable, len(specs))
	for _, spec := range specs {
		table, err := apple2.ParseFixedNameTable(disk, spec.Name, spec.RawOffset, spec.RecordSize, spec.RecordCount)
		if err != nil {
			return diskCandidates{}, err
		}
		entries := make([]nameRecord, 0, len(table.Entries))
		for _, entry := range table.Entries {
			entries = append(entries, nameRecord{Index: entry.Index, RawOffset: entry.RawOffset, Name: entry.Name})
		}
		parsed[spec.Name] = candidateTable{
			Name:                spec.Name,
			RawOffset:           spec.RawOffset,
			RecordSize:          spec.RecordSize,
			RecordCount:         spec.RecordCount,
			Entries:             entries,
			ObservationEvidence: string(apple2.EvidenceProven),
			SemanticLabel:       spec.SemanticLabel,
			SemanticEvidence:    string(spec.SemanticLevel),
			ConsumerEvidence:    string(apple2.EvidenceUnknown),
			Notes:               spec.Notes,
		}
	}
	return diskCandidates{
		StarSystems: parsed["star-system-name-candidate"],
		Planets:     parsed["planet-name-candidate"],
		Cities:      parsed["city-name-candidate-block-1"],
	}, nil
}

func parseAuxiliaryMap(data []byte) (auxiliaryMap, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return auxiliaryMap{}, fmt.Errorf("open map archive: %w", err)
	}
	planetsHTML, err := readArchiveFile(reader, mapPlanetsHTML)
	if err != nil {
		return auxiliaryMap{}, err
	}
	citiesHTML, err := readArchiveFile(reader, mapCitiesHTML)
	if err != nil {
		return auxiliaryMap{}, err
	}
	pairs, err := parsePlanetPairs(string(planetsHTML))
	if err != nil {
		return auxiliaryMap{}, err
	}
	triples, err := parseCityTriples(string(citiesHTML))
	if err != nil {
		return auxiliaryMap{}, err
	}
	return auxiliaryMap{
		PlanetPairs:              pairs,
		CityTriples:              triples,
		PlanetPairCount:          len(pairs),
		CityTripleCount:          len(triples),
		ObservationEvidence:      string(apple2.EvidenceProven),
		RelationshipEvidence:     string(apple2.EvidenceStrongInference),
		OriginalConsumerEvidence: string(apple2.EvidenceUnknown),
	}, nil
}

func readArchiveFile(reader *zip.Reader, name string) ([]byte, error) {
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		stream, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("open %q: %w", name, err)
		}
		defer stream.Close()
		data, err := io.ReadAll(stream)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", name, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("map archive lacks %q", name)
}

func parsePlanetPairs(source string) ([]mapPlanetPair, error) {
	result := make([]mapPlanetPair, 0)
	seen := make(map[string]struct{})
	for _, match := range imageSourcePattern.FindAllStringSubmatch(source, -1) {
		decoded, err := url.PathUnescape(html.UnescapeString(match[1]))
		if err != nil {
			return nil, fmt.Errorf("decode planet image source %q: %w", match[1], err)
		}
		fileName := path.Base(decoded)
		extension := path.Ext(fileName)
		if !strings.EqualFold(extension, ".gif") {
			continue
		}
		stem := strings.TrimSuffix(fileName, extension)
		if stem == "Star_Map" {
			continue
		}
		parts := strings.Split(stem, "_")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("planet image %q does not encode exactly one system/planet pair", fileName)
		}
		pair := mapPlanetPair{System: parts[0], Planet: parts[1]}
		key := pair.System + "\x00" + pair.Planet
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate planet pair %q/%q", pair.System, pair.Planet)
		}
		seen[key] = struct{}{}
		result = append(result, pair)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("planets HTML contains no named map images")
	}
	return result, nil
}

func parseCityTriples(source string) ([]mapCityTriple, error) {
	result := make([]mapCityTriple, 0)
	seen := make(map[string]struct{})
	for _, match := range strongPattern.FindAllStringSubmatch(source, -1) {
		label := strings.TrimSpace(html.UnescapeString(tagPattern.ReplaceAllString(match[1], "")))
		if strings.Count(label, "/") != 2 {
			continue
		}
		parts := strings.Split(label, "/")
		if len(parts) != 3 {
			return nil, fmt.Errorf("city label %q is not a three-level path", label)
		}
		city, phase := splitPhase(strings.TrimSpace(parts[2]))
		triple := mapCityTriple{
			System: strings.TrimSpace(parts[0]),
			Planet: strings.TrimSpace(parts[1]),
			City:   city,
			Phase:  phase,
			Label:  label,
		}
		if triple.System == "" || triple.Planet == "" || triple.City == "" {
			return nil, fmt.Errorf("city label %q has an empty component", label)
		}
		key := triple.System + "\x00" + triple.Planet + "\x00" + triple.City
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("duplicate city triple %q", label)
		}
		seen[key] = struct{}{}
		result = append(result, triple)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("cities HTML contains no labeled system/planet/city triples")
	}
	return result, nil
}

func splitPhase(value string) (string, string) {
	start := strings.LastIndex(value, " (")
	if start < 0 || !strings.HasSuffix(value, ")") {
		return value, ""
	}
	return strings.TrimSpace(value[:start]), strings.TrimSpace(value[start+2 : len(value)-1])
}

func uniqueRecords(entries []nameRecord) []nameRecord {
	result := make([]nameRecord, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if _, exists := seen[entry.Name]; exists {
			continue
		}
		seen[entry.Name] = struct{}{}
		result = append(result, entry)
	}
	return result
}

func compareNames(candidates []nameRecord, mapNames []string) nameCoverage {
	candidateByNormalized := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		candidateByNormalized[normalizeName(candidate.Name)] = candidate.Name
	}
	mapByNormalized := make(map[string]string, len(mapNames))
	for _, mapName := range mapNames {
		mapByNormalized[normalizeName(mapName)] = mapName
	}
	matched := make([]string, 0)
	candidateOnly := make([]string, 0)
	for _, candidate := range candidates {
		if _, ok := mapByNormalized[normalizeName(candidate.Name)]; ok {
			matched = append(matched, candidate.Name)
			continue
		}
		candidateOnly = append(candidateOnly, candidate.Name)
	}
	mapOnly := make([]string, 0)
	for _, mapName := range stableMapValues(mapByNormalized) {
		if _, ok := candidateByNormalized[normalizeName(mapName)]; !ok {
			mapOnly = append(mapOnly, mapName)
		}
	}
	return nameCoverage{
		CandidateCount:     len(candidates),
		MapCount:           len(mapByNormalized),
		MatchCount:         len(matched),
		MatchedCandidates:  matched,
		CandidateOnlyNames: candidateOnly,
		MapOnlyNames:       mapOnly,
	}
}

func stableMapValues(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}

func normalizeName(value string) string {
	var builder strings.Builder
	for _, character := range strings.ToLower(value) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			builder.WriteRune(character)
		}
	}
	return builder.String()
}

func identity(name string, data []byte) inputRecord {
	return inputRecord{Name: name, Size: len(data), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}
}

func writeDocument(report document, outputPath string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	data = append(data, '\n')
	if outputPath == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("write %q: %w", outputPath, err)
	}
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
