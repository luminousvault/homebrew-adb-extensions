// Package agentskill puts ak's skill (SKILL.md) where AI coding agents look
// for skills, so they use ak instead of raw adb.
package agentskill

import (
	"bytes"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Content is the skill ak installs.
//
//go:embed SKILL.md
var Content []byte

// marker marks a skill file as ak's own: ak updates it only while it is there.
const marker = "managed-by: ak"

// State is what one agent folder has.
type State string

const (
	NoAgent  State = "no-agent" // the agent folder (~/.claude) does not exist
	Missing  State = "missing"  // never installed
	Current  State = "current"
	Outdated State = "outdated" // ak's, from another version
	Edited   State = "edited"   // the marker was removed: the user's own
	Removed  State = "removed"  // installed once, deleted since: not added again
)

// Status is one agent folder.
type Status struct {
	Path  string `json:"path"` // the skill file
	State State  `json:"state"`
}

// roots are the agent folders, in the order they are reported.
func roots(home string) []string {
	return []string{filepath.Join(home, ".claude"), filepath.Join(home, ".agents")}
}

// StatePath records the skill files ak has written, so a deleted one is not
// written again.
func StatePath(home string) string { return filepath.Join(home, ".ak", "skills-installed") }

func installed(home string) map[string]bool {
	m := map[string]bool{}
	b, _ := os.ReadFile(StatePath(home))
	for _, l := range strings.Split(string(b), "\n") {
		if l != "" {
			m[l] = true
		}
	}
	return m
}

// Check reports each agent folder without changing anything.
func Check(home string) []Status {
	done := installed(home)
	var out []Status
	for _, root := range roots(home) {
		s := Status{Path: filepath.Join(root, "skills", "ak", "SKILL.md")}
		b, err := os.ReadFile(s.Path)
		switch {
		case err == nil && bytes.Equal(b, Content):
			s.State = Current
		case err == nil && bytes.Contains(b, []byte(marker)):
			s.State = Outdated
		case err == nil:
			s.State = Edited
		case done[s.Path]:
			s.State = Removed
		default:
			s.State = Missing
			if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
				s.State = NoAgent
			}
		}
		out = append(out, s)
	}
	return out
}

// Ensure writes the skill where it is missing and updates ak's outdated
// copies. It returns the skill files it created.
func Ensure(home string) ([]string, error) {
	var created []string
	var errs []error
	for _, s := range Check(home) {
		if s.State != Missing && s.State != Outdated {
			continue
		}
		if err := write(s.Path); err != nil {
			errs = append(errs, err)
			continue
		}
		if s.State == Missing {
			created = append(created, s.Path)
		}
	}
	if len(created) > 0 {
		errs = append(errs, record(home, created))
	}
	return created, errors.Join(errs...)
}

func write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, Content, 0o644)
}

func record(home string, paths []string) error {
	p := StatePath(home)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(strings.Join(paths, "\n") + "\n")
	return errors.Join(err, f.Close())
}
