package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validPokemonJSON = `{"records":[{"id":"charizard","upstream":{"untouched":true}}],"text":{"charizard":{"name":"Charizard","description":"Raw upstream text"}}}`

func TestLoadSnapshot(t *testing.T) {
	dir := writeSnapshot(t, validPokemonJSON, nil)
	snapshot, err := LoadSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Source.Package != "@pokepc/dataset" || snapshot.Source.Version != "8.2.0" || snapshot.Source.GameSet != "champions" || snapshot.Source.Locale != "eng" || snapshot.Source.LockfileSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("unexpected source: %+v", snapshot.Source)
	}
	if snapshot.FormatVersion != 1 || len(snapshot.Catalogs) != 1 || snapshot.Catalogs["pokemon"].Scope != "" {
		t.Fatalf("unexpected format 1 catalogs: %+v", snapshot)
	}
	if len(snapshot.Records) != 1 || string(snapshot.Records[0]) != `{"id":"charizard","upstream":{"untouched":true}}` {
		t.Fatalf("raw records not preserved: %s", snapshot.Records)
	}
	if string(snapshot.Text["charizard"]) != `{"name":"Charizard","description":"Raw upstream text"}` {
		t.Fatalf("raw text not preserved: %s", snapshot.Text["charizard"])
	}
}

func TestLoadSnapshotInvalidPayload(t *testing.T) {
	for _, tc := range []struct{ name, payload, want string }{
		{"empty records", `{"records":[],"text":{}}`, "nonempty records"},
		{"empty id", `{"records":[{"id":""}]}`, "nonempty id"},
		{"whitespace id", `{"records":[{"id":"  "}]}`, "nonempty id"},
		{"missing id", `{"records":[{}]}`, "nonempty id"},
		{"non-object record", `{"records":[42]}`, "decode Pokémon record"},
		{"duplicate ids", `{"records":[{"id":"a"},{"id":"a"}],"text":{"a":{"name":"A"}}}`, "duplicate Pokémon id"},
		{"missing text", `{"records":[{"id":"a"}]}`, "nonempty name"},
		{"missing name", `{"records":[{"id":"a"}],"text":{"a":{}}}`, "nonempty name"},
		{"empty name", `{"records":[{"id":"a"}],"text":{"a":{"name":" "}}}`, "nonempty name"},
		{"wrong name type", `{"records":[{"id":"a"}],"text":{"a":{"name":42}}}`, "nonempty name"},
		{"invalid json", `{`, "decode pokemon.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeSnapshot(t, tc.payload, nil)
			assertSnapshotError(t, dir, tc.want)
		})
	}
}

func TestLoadSnapshotInvalidManifest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"unsupported format", func(m map[string]any) { m["formatVersion"] = 3 }, "unsupported snapshot formatVersion"},
		{"unsupported package", func(m map[string]any) { m["source"].(map[string]any)["package"] = "other" }, "unsupported source"},
		{"unsupported game", func(m map[string]any) { m["source"].(map[string]any)["gameSet"] = "other" }, "unsupported source"},
		{"unsupported locale", func(m map[string]any) { m["source"].(map[string]any)["locale"] = "other" }, "unsupported source"},
		{"empty version", func(m map[string]any) { m["source"].(map[string]any)["version"] = " " }, "version must be nonempty"},
		{"short lockfile hash", func(m map[string]any) { m["source"].(map[string]any)["lockfileSha256"] = "abc" }, "lockfileSha256"},
		{"nonhex lockfile hash", func(m map[string]any) { m["source"].(map[string]any)["lockfileSha256"] = strings.Repeat("z", 64) }, "lockfileSha256"},
		{"missing file entry", func(m map[string]any) { delete(m["files"].(map[string]string), "LICENSE") }, "exactly pokemon.json and LICENSE"},
		{"extra path", func(m map[string]any) { m["files"].(map[string]string)["../outside"] = strings.Repeat("a", 64) }, "exactly pokemon.json and LICENSE"},
		{"replaced path", func(m map[string]any) {
			files := m["files"].(map[string]string)
			files["../outside"] = files["LICENSE"]
			delete(files, "LICENSE")
		}, "exactly pokemon.json and LICENSE"},
		{"invalid file hash", func(m map[string]any) { m["files"].(map[string]string)["pokemon.json"] = strings.Repeat("z", 64) }, "SHA256 hex hashes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeSnapshot(t, validPokemonJSON, tc.mutate)
			assertSnapshotError(t, dir, tc.want)
		})
	}
}

func TestLoadSnapshotFiles(t *testing.T) {
	for _, name := range []string{"manifest.json", "pokemon.json", "LICENSE"} {
		t.Run("missing "+name, func(t *testing.T) {
			dir := writeSnapshot(t, validPokemonJSON, nil)
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			assertSnapshotError(t, dir, "read "+name)
		})
		t.Run("corrupt "+name, func(t *testing.T) {
			dir := writeSnapshot(t, validPokemonJSON, nil)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("corrupt"), 0600); err != nil {
				t.Fatal(err)
			}
			want := "SHA256 mismatch for " + name
			if name == "manifest.json" {
				want = "decode manifest.json"
			}
			assertSnapshotError(t, dir, want)
		})
	}
}

func TestLoadSnapshotUppercaseHashes(t *testing.T) {
	dir := writeSnapshot(t, validPokemonJSON, func(m map[string]any) {
		for name, hash := range m["files"].(map[string]string) {
			m["files"].(map[string]string)[name] = strings.ToUpper(hash)
		}
	})
	if _, err := LoadSnapshot(dir); err != nil {
		t.Fatal(err)
	}
}

var expandedCatalogNames = []string{"pokemon", "moves", "abilities", "items", "battle-states", "types", "natures"}

const expandedRecord = `{"id":"example","flags":{"contact":true,"protect":false},"unknownReference":"not-validated","upstream":{"untouched":true}}`
const expandedText = `{"name":"Example","description":"Raw upstream description","extra":[1,true]}`

func expandedContents() map[string]string {
	contents := map[string]string{"LICENSE": "Fixture license\n"}
	for _, name := range expandedCatalogNames {
		scope := "champions"
		if name == "types" || name == "natures" {
			scope = "base"
		}
		contents[name+".json"] = `{"scope":"` + scope + `","records":[` + expandedRecord + `],"text":{"example":` + expandedText + `}}`
	}
	return contents
}

func TestLoadSnapshotExpanded(t *testing.T) {
	dir := writeSnapshotFiles(t, 2, expandedContents(), func(m map[string]any) {
		for name, hash := range m["files"].(map[string]string) {
			m["files"].(map[string]string)[name] = strings.ToUpper(hash)
		}
	})
	snapshot, err := LoadSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.FormatVersion != 2 || len(snapshot.Catalogs) != len(expandedCatalogNames) {
		t.Fatalf("unexpected expanded snapshot: %+v", snapshot)
	}
	for _, name := range expandedCatalogNames {
		catalog := snapshot.Catalogs[name]
		wantScope := "champions"
		if name == "types" || name == "natures" {
			wantScope = "base"
		}
		if catalog.Scope != wantScope || len(catalog.Records) != 1 || string(catalog.Records[0]) != expandedRecord || string(catalog.Text["example"]) != expandedText {
			t.Errorf("%s scope or raw fields not preserved: %+v", name, catalog)
		}
	}
	if &snapshot.Records[0] != &snapshot.Catalogs["pokemon"].Records[0] {
		t.Fatal("Records does not alias the Pokémon catalog")
	}
	snapshot.Text["alias-check"] = json.RawMessage(`{}`)
	if _, ok := snapshot.Catalogs["pokemon"].Text["alias-check"]; !ok {
		t.Fatal("Text does not alias the Pokémon catalog")
	}
}

func TestLoadSnapshotExpandedInvalidPayload(t *testing.T) {
	for _, name := range expandedCatalogNames {
		for _, tc := range []struct{ name, payload, want string }{
			{"empty records", `{"records":[],"text":{}}`, "nonempty records"},
			{"missing records", `{}`, "nonempty records"},
			{"missing id", `{"records":[{}]}`, "nonempty id"},
			{"empty id", `{"records":[{"id":""}]}`, "nonempty id"},
			{"blank id", `{"records":[{"id":"  "}]}`, "nonempty id"},
			{"wrong id type", `{"records":[{"id":42}]}`, "decode"},
			{"non-object record", `{"records":[42]}`, "decode"},
			{"duplicate ids", `{"records":[{"id":"a"},{"id":"a"}],"text":{"a":{"name":"A"}}}`, "duplicate"},
			{"missing text", `{"records":[{"id":"a"}]}`, "nonempty name"},
			{"missing name", `{"records":[{"id":"a"}],"text":{"a":{}}}`, "nonempty name"},
			{"empty name", `{"records":[{"id":"a"}],"text":{"a":{"name":""}}}`, "nonempty name"},
			{"blank name", `{"records":[{"id":"a"}],"text":{"a":{"name":"  "}}}`, "nonempty name"},
			{"wrong name type", `{"records":[{"id":"a"}],"text":{"a":{"name":42}}}`, "nonempty name"},
			{"invalid json", `{`, "decode " + name + ".json"},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				contents := expandedContents()
				var original map[string]json.RawMessage
				if err := json.Unmarshal([]byte(contents[name+".json"]), &original); err != nil {
					t.Fatal(err)
				}
				payload := tc.payload
				if tc.name != "invalid json" {
					var invalid map[string]json.RawMessage
					if err := json.Unmarshal([]byte(payload), &invalid); err != nil {
						t.Fatal(err)
					}
					invalid["scope"] = original["scope"]
					data, err := json.Marshal(invalid)
					if err != nil {
						t.Fatal(err)
					}
					payload = string(data)
				}
				contents[name+".json"] = payload
				assertSnapshotError(t, writeSnapshotFiles(t, 2, contents, nil), tc.want)
			})
		}
		for _, scope := range []string{"", "other", "champions", "base", " champions "} {
			if (name == "types" || name == "natures") && scope == "base" || name != "types" && name != "natures" && scope == "champions" {
				continue
			}
			t.Run(name+"/scope="+scope, func(t *testing.T) {
				contents := expandedContents()
				var payload map[string]any
				if err := json.Unmarshal([]byte(contents[name+".json"]), &payload); err != nil {
					t.Fatal(err)
				}
				if scope == "" {
					delete(payload, "scope")
				} else {
					payload["scope"] = scope
				}
				data, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				contents[name+".json"] = string(data)
				assertSnapshotError(t, writeSnapshotFiles(t, 2, contents, nil), name+".json scope must be")
			})
		}
	}
}

func TestLoadSnapshotExpandedFiles(t *testing.T) {
	for name := range expandedContents() {
		t.Run(name, func(t *testing.T) {
			for _, corrupt := range []bool{false, true} {
				dir := writeSnapshotFiles(t, 2, expandedContents(), nil)
				want := "read " + name
				if corrupt {
					if err := os.WriteFile(filepath.Join(dir, name), []byte("corrupt"), 0600); err != nil {
						t.Fatal(err)
					}
					want = "SHA256 mismatch for " + name
				} else if err := os.Remove(filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
				assertSnapshotError(t, dir, want)
			}
			for _, mutation := range []string{"missing", "replaced", "short hash", "nonhex hash"} {
				dir := writeSnapshotFiles(t, 2, expandedContents(), func(m map[string]any) {
					files := m["files"].(map[string]string)
					switch mutation {
					case "missing":
						delete(files, name)
					case "replaced":
						files["../outside"] = files[name]
						delete(files, name)
					case "short hash":
						files[name] = "abc"
					case "nonhex hash":
						files[name] = strings.Repeat("z", 64)
					}
				})
				assertSnapshotError(t, dir, "manifest files must contain exactly")
			}
		})
	}
	dir := writeSnapshotFiles(t, 2, expandedContents(), func(m map[string]any) {
		m["files"].(map[string]string)["../outside"] = strings.Repeat("a", 64)
	})
	assertSnapshotError(t, dir, "manifest files must contain exactly")
}

func writeSnapshot(t *testing.T, payload string, mutate func(map[string]any)) string {
	t.Helper()
	return writeSnapshotFiles(t, 1, map[string]string{"pokemon.json": payload, "LICENSE": "Fixture license\n"}, mutate)
}

func writeSnapshotFiles(t *testing.T, version int, contents map[string]string, mutate func(map[string]any)) string {
	t.Helper()
	dir := t.TempDir()
	files := make(map[string]string)
	for name, content := range contents {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(content))
		files[name] = hex.EncodeToString(sum[:])
	}
	manifest := map[string]any{
		"formatVersion": version,
		"source": map[string]any{
			"package": "@pokepc/dataset", "version": "8.2.0", "gameSet": "champions",
			"locale": "eng", "lockfileSha256": strings.Repeat("a", 64),
		},
		"files": files,
	}
	if mutate != nil {
		mutate(manifest)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertSnapshotError(t *testing.T, dir, want string) {
	t.Helper()
	_, err := LoadSnapshot(dir)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("LoadSnapshot error = %v; want %q", err, want)
	}
}
