package cli

import (
	"os"
	"strings"
	"testing"
)

func TestDocsMatchCommands(t *testing.T) {
	data, err := os.ReadFile("../../docs/commands.md")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.ReplaceAll(string(data), "\r\n", "\n") // CRLF on a Windows checkout
	if got != CommandsMarkdown() {
		t.Error("docs/commands.md is out of date; run: make docs")
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
