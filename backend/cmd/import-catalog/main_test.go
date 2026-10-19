package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRunCheck(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			dir := writeCLISnapshot(t, version)
			var stdout, stderr bytes.Buffer
			if err := run([]string{"-check", "-snapshot", dir}, &stdout, &stderr); err != nil {
				t.Fatal(err)
			}
			counts := "1 Pokémon records"
			if version == 2 {
				counts += ", 2 moves records, 3 abilities records, 4 items records, 5 battle-states records, 6 types records, 7 natures records"
			}
			want := "Checked " + counts + "; source @pokepc/dataset version 8.2.0, gameSet champions, locale eng, lockfileSha256 " + strings.Repeat("a", 64) + ". No database writes performed.\n"
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("stdout = %q, stderr = %q; want %q", stdout.String(), stderr.String(), want)
			}
		})
	}
}

func TestRunRejectsInvalidInvocation(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"-snapshot", "unused"},
		{"-check"},
		{"-check", "-snapshot", " "},
		{"-check", "-snapshot", "unused", "positional"},
		{"-unknown"},
	} {
		var stdout, stderr bytes.Buffer
		err := run(args, &stdout, &stderr)
		if err == nil || !strings.Contains(err.Error(), "database import is not implemented") || stdout.Len() != 0 {
			t.Fatalf("run(%q) error = %v, stdout = %q", args, err, stdout.String())
		}
	}
}

func TestRunRejectsCorruptExpandedSnapshot(t *testing.T) {
	dir := writeCLISnapshot(t, 2)
	if err := os.WriteFile(filepath.Join(dir, "moves.json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := run([]string{"-check", "-snapshot", dir}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "check snapshot: SHA256 mismatch for moves.json") || stdout.Len() != 0 {
		t.Fatalf("error = %v, stdout = %q", err, stdout.String())
	}
}

func writeCLISnapshot(t *testing.T, version int) string {
	t.Helper()
	dir := t.TempDir()
	names := []string{"pokemon"}
	if version == 2 {
		names = append(names, "moves", "abilities", "items", "battle-states", "types", "natures")
	}
	contents := map[string][]byte{"LICENSE": []byte("Fixture license\n")}
	for i, name := range names {
		records := make([]map[string]string, 0, i+1)
		text := make(map[string]map[string]string)
		for j := 0; j <= i; j++ {
			id := strconv.Itoa(j)
			records = append(records, map[string]string{"id": id})
			text[id] = map[string]string{"name": "Example " + id}
		}
		payload := map[string]any{"records": records, "text": text}
		if version == 2 {
			payload["scope"] = "champions"
			if name == "types" || name == "natures" {
				payload["scope"] = "base"
			}
		}
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		contents[name+".json"] = data
	}
	files := make(map[string]string)
	for name, data := range contents {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		files[name] = hex.EncodeToString(sum[:])
	}
	manifest := map[string]any{
		"formatVersion": version,
		"source": map[string]string{
			"package": "@pokepc/dataset", "version": "8.2.0", "gameSet": "champions",
			"locale": "eng", "lockfileSha256": strings.Repeat("a", 64),
		},
		"files": files,
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
