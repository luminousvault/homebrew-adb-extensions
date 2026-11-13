package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docs/commands.md and every docs/commands.<lang>.md (one per
// docs/i18n/<lang>.json) are generated; regenerate with: make docs
func TestDocsMatchCommands(t *testing.T) {
	files, err := filepath.Glob("../../docs/i18n/*.json")
	if err != nil {
		t.Fatal(err)
	}
	translations := map[string][]byte{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		translations[strings.TrimSuffix(filepath.Base(f), ".json")] = data
	}
	docs, err := CommandsDocs(translations) // also fails when a translation misses an entry
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range docs {
		data, err := os.ReadFile("../../docs/" + name)
		if err != nil {
			t.Errorf("%s: %v; run: make docs", name, err)
			continue
		}
		if strings.ReplaceAll(string(data), "\r\n", "\n") != want { // CRLF on a Windows checkout
			t.Errorf("docs/%s is out of date; run: make docs", name)
		}
	}
	// A translation removed from i18n/ must take its page with it.
	pages, _ := filepath.Glob("../../docs/commands*.md")
	for _, p := range pages {
		if _, ok := docs[filepath.Base(p)]; !ok {
			t.Errorf("docs/%s has no docs/i18n JSON; delete it or add the translation", filepath.Base(p))
		}
	}
}

// A translation that misses an entry is refused, with the missing keys.
func TestDocsTranslationMustBeComplete(t *testing.T) {
	_, err := CommandsDocs(map[string][]byte{"xx": []byte(`{"language": "X", "title": "T"}`)})
	if err == nil || !strings.Contains(err.Error(), "commands.install\n") {
		t.Errorf("incomplete translation accepted: %v", err)
	}
}

func TestDocsGroupEveryCommand(t *testing.T) {
	grouped := map[string]int{}
	for _, g := range docGroups {
		for _, name := range g.Commands {
			grouped[name]++
			if c := find(name); c == nil || c.Hidden {
				t.Errorf("docGroups: %q is not a visible command", name)
			}
		}
	}
	for _, c := range visibleCommands() {
		switch grouped[c.Name] {
		case 0:
			t.Errorf("docGroups: %q is in no group", c.Name)
		case 1:
		default:
			t.Errorf("docGroups: %q is in more than one group", c.Name)
		}
	}
}
