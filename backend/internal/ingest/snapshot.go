package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Source identifies the upstream input recorded by the offline exporter.
type Source struct {
	Package        string `json:"package"`
	Version        string `json:"version"`
	GameSet        string `json:"gameSet"`
	Locale         string `json:"locale"`
	LockfileSHA256 string `json:"lockfileSha256"`
}

// Catalog retains upstream records and localized text without projecting fields.
type Catalog struct {
	Scope   string                     `json:"scope"`
	Records []json.RawMessage          `json:"records"`
	Text    map[string]json.RawMessage `json:"text"`
}

// Snapshot retains upstream JSON without imposing a domain or database schema.
type Snapshot struct {
	FormatVersion int
	Source        Source
	Catalogs      map[string]Catalog
	// Records and Text alias the Pokémon catalog for format 1 callers.
	Records []json.RawMessage
	Text    map[string]json.RawMessage
}

// LoadSnapshot checks the offline export's structure and file integrity.
// Hashes detect corruption relative to the manifest; they do not establish trust
// in the manifest or the upstream data.
func LoadSnapshot(dir string) (Snapshot, error) {
	var manifest struct {
		FormatVersion int               `json:"formatVersion"`
		Source        Source            `json:"source"`
		Files         map[string]string `json:"files"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read manifest.json: %w", err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Snapshot{}, fmt.Errorf("decode manifest.json: %w", err)
	}
	catalogNames := []string{"pokemon"}
	switch manifest.FormatVersion {
	case 1:
	case 2:
		catalogNames = []string{"pokemon", "moves", "abilities", "items", "battle-states", "types", "natures"}
	default:
		return Snapshot{}, fmt.Errorf("unsupported snapshot formatVersion %d; want 1 or 2", manifest.FormatVersion)
	}
	source := manifest.Source
	if source.Package != "@pokepc/dataset" || source.GameSet != "champions" || source.Locale != "eng" {
		return Snapshot{}, fmt.Errorf("unsupported source: require package @pokepc/dataset, gameSet champions, locale eng")
	}
	if strings.TrimSpace(source.Version) == "" {
		return Snapshot{}, fmt.Errorf("source version must be nonempty")
	}
	if !validSHA256(source.LockfileSHA256) {
		return Snapshot{}, fmt.Errorf("source lockfileSha256 must be 64 hexadecimal characters")
	}
	fileNames := make([]string, 0, len(catalogNames)+1)
	for _, name := range catalogNames {
		fileNames = append(fileNames, name+".json")
	}
	fileNames = append(fileNames, "LICENSE")
	filesError := fmt.Errorf("manifest files must contain exactly %s and LICENSE with SHA256 hex hashes", strings.Join(fileNames[:len(fileNames)-1], ", "))
	if len(manifest.Files) != len(fileNames) {
		return Snapshot{}, filesError
	}
	for _, name := range fileNames {
		if !validSHA256(manifest.Files[name]) {
			return Snapshot{}, filesError
		}
	}

	contents := make(map[string][]byte, len(fileNames))
	// Only these fixed paths may be read, never arbitrary manifest entries.
	for _, name := range fileNames {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return Snapshot{}, fmt.Errorf("read %s: %w", name, err)
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), manifest.Files[name]) {
			return Snapshot{}, fmt.Errorf("SHA256 mismatch for %s", name)
		}
		contents[name] = data
	}

	snapshot := Snapshot{
		FormatVersion: manifest.FormatVersion,
		Source:        source,
		Catalogs:      make(map[string]Catalog, len(catalogNames)),
	}
	for _, name := range catalogNames {
		var catalog Catalog
		if err := json.Unmarshal(contents[name+".json"], &catalog); err != nil {
			return Snapshot{}, fmt.Errorf("decode %s.json: %w", name, err)
		}
		if manifest.FormatVersion == 2 {
			scope := "champions"
			if name == "types" || name == "natures" {
				scope = "base"
			}
			if catalog.Scope != scope {
				return Snapshot{}, fmt.Errorf("%s.json scope must be %q", name, scope)
			}
		}
		if err := validateCatalog(name, catalog); err != nil {
			return Snapshot{}, err
		}
		snapshot.Catalogs[name] = catalog
	}
	snapshot.Records = snapshot.Catalogs["pokemon"].Records
	snapshot.Text = snapshot.Catalogs["pokemon"].Text
	return snapshot, nil
}

func validateCatalog(name string, payload Catalog) error {
	if len(payload.Records) == 0 {
		return fmt.Errorf("%s.json must contain nonempty records", name)
	}
	label := name
	if name == "pokemon" {
		label = "Pokémon"
	}
	seen := make(map[string]bool, len(payload.Records))
	for i, raw := range payload.Records {
		var record struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &record); err != nil {
			return fmt.Errorf("decode %s record %d: %w", label, i, err)
		}
		if strings.TrimSpace(record.ID) == "" {
			return fmt.Errorf("%s record %d must have a nonempty id", label, i)
		}
		if seen[record.ID] {
			return fmt.Errorf("duplicate %s id %q", label, record.ID)
		}
		seen[record.ID] = true
		var text struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(payload.Text[record.ID], &text); err != nil || strings.TrimSpace(text.Name) == "" {
			return fmt.Errorf("%s %q must have text with a nonempty name", label, record.ID)
		}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
