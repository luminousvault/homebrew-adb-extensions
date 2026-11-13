// Command gendocs writes the command reference into a docs directory:
// commands.md from the command table, and commands.<lang>.md for every
// i18n/<lang>.json there (drop in a new JSON file and run make docs).
//
//	go run ./cmd/gendocs docs                 # what make docs runs
//	go run ./cmd/gendocs -skeleton > xx.json  # English text to start a translation
//
// Translations are read here, at doc-generation time; they are never built
// into ak.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/cli"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gendocs <docs dir> | -skeleton")
		os.Exit(2)
	}
	if os.Args[1] == "-skeleton" {
		fmt.Print(cli.DocSkeleton())
		return
	}
	if err := write(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
}

func write(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "i18n", "*.json"))
	if err != nil {
		return err
	}
	translations := map[string][]byte{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		translations[strings.TrimSuffix(filepath.Base(f), ".json")] = data
	}
	docs, err := cli.CommandsDocs(translations)
	if err != nil {
		return err
	}
	for name, md := range docs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(md), 0o644); err != nil {
			return err
		}
	}
	return nil
}
