package cli

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// zshSpec is what completions/_ak declares: command -> description and flags.
type zshSpec struct {
	desc  map[string]string
	flags map[string][]string
}

var (
	reCommand = regexp.MustCompile(`^\s+'([a-z]+):(.+)'$`)
	reCase    = regexp.MustCompile(`^\s{8}([a-z]+)\)$`)
	reBrace   = regexp.MustCompile(`\{(-[^}]+)\}'?\[`)
	reSingle  = regexp.MustCompile(`[')](-{1,2}[a-zA-Z][a-zA-Z-]*)\[`)
)

func loadZsh(t *testing.T) zshSpec {
	t.Helper()
	data, err := os.ReadFile("../../completions/_ak")
	if err != nil {
		t.Fatal(err)
	}
	spec := zshSpec{desc: map[string]string{}, flags: map[string][]string{}}
	current := ""
	inCases := false
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "case $words[1] in") {
			inCases = true
			continue
		}
		if !inCases {
			if m := reCommand.FindStringSubmatch(line); m != nil {
				spec.desc[m[1]] = m[2]
			}
			continue
		}
		if strings.TrimSpace(line) == "*)" {
			current = "" // fallback branch, not a command
			continue
		}
		if m := reCase.FindStringSubmatch(line); m != nil {
			current = m[1]
			spec.flags[current] = nil
			continue
		}
		if current == "" {
			continue
		}
		for _, m := range reBrace.FindAllStringSubmatch(line, -1) {
			spec.flags[current] = append(spec.flags[current], strings.Split(m[1], ",")...)
		}
		for _, m := range reSingle.FindAllStringSubmatch(line, -1) {
			spec.flags[current] = append(spec.flags[current], m[1])
		}
	}
	return spec
}

func sorted(s []string) []string {
	c := append([]string(nil), s...)
	sort.Strings(c)
	return c
}

// TestCompletionMatchesCommands keeps the hand-maintained zsh completion in
// sync with the Go command table, so existing completion keeps working.
func TestCompletionMatchesCommands(t *testing.T) {
	spec := loadZsh(t)
	names := map[string]bool{}
	for _, c := range Commands {
		names[c.Name] = true
		desc, ok := spec.desc[c.Name]
		if !ok {
			t.Errorf("%s: missing from completions/_ak command list", c.Name)
			continue
		}
		if desc != c.Summary {
			t.Errorf("%s: description %q, completion has %q", c.Name, c.Summary, desc)
		}
		var flags []string
		for _, f := range c.AllFlags() {
			for _, n := range []string{f.Short, f.Long} {
				if n != "" {
					flags = append(flags, n)
				}
			}
		}
		if got, want := sorted(spec.flags[c.Name]), sorted(flags); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s: flags %v, completion has %v", c.Name, want, got)
		}
	}
	for name := range spec.desc {
		if !names[name] {
			t.Errorf("%s: in completions/_ak but not implemented", name)
		}
	}
}
