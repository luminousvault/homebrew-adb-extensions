package agentskill

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func states(home string) []State {
	var out []State
	for _, s := range Check(home) {
		out = append(out, s.State)
	}
	return out
}

func TestEnsureWritesOnlyWhereAnAgentExists(t *testing.T) {
	home := t.TempDir()
	if created, err := Ensure(home); err != nil || len(created) != 0 {
		t.Fatalf("no agent folders: created %v, err %v", created, err)
	}
	if _, err := os.Stat(StatePath(home)); err == nil {
		t.Fatal("state file written with nothing installed")
	}
	if got := states(home); !slices.Equal(got, []State{NoAgent, NoAgent}) {
		t.Fatalf("states = %v", got)
	}

	if err := os.Mkdir(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	created, err := Ensure(home)
	want := filepath.Join(home, ".claude", "skills", "ak", "SKILL.md")
	if err != nil || len(created) != 1 || created[0] != want {
		t.Fatalf("created %v, err %v", created, err)
	}
	if b, _ := os.ReadFile(want); !bytes.Equal(b, Content) {
		t.Fatal("skill content differs")
	}
	if got := states(home); !slices.Equal(got, []State{Current, NoAgent}) {
		t.Fatalf("states = %v", got)
	}
	if created, _ := Ensure(home); len(created) != 0 {
		t.Fatalf("second run created %v", created)
	}
}

func TestEnsureUpdatesOnlyAksOwnCopy(t *testing.T) {
	home := t.TempDir()
	claude := filepath.Join(home, ".claude", "skills", "ak", "SKILL.md")
	agents := filepath.Join(home, ".agents", "skills", "ak", "SKILL.md")
	for _, p := range []string{claude, agents} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	old := "---\nname: ak\nmetadata:\n  managed-by: ak\n---\nold\n"
	mine := "---\nname: ak\n---\nmy own notes\n"
	_ = os.WriteFile(claude, []byte(old), 0o644)
	_ = os.WriteFile(agents, []byte(mine), 0o644)
	if got := states(home); !slices.Equal(got, []State{Outdated, Edited}) {
		t.Fatalf("states = %v", got)
	}

	created, err := Ensure(home)
	if err != nil || len(created) != 0 { // updated, not created: no message
		t.Fatalf("created %v, err %v", created, err)
	}
	if b, _ := os.ReadFile(claude); !bytes.Equal(b, Content) {
		t.Fatal("ak's outdated copy was not updated")
	}
	if b, _ := os.ReadFile(agents); string(b) != mine {
		t.Fatal("the user's edited copy was overwritten")
	}
}

func TestEnsureDoesNotBringBackADeletedSkill(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Ensure(home); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(home, ".claude", "skills", "ak")); err != nil {
		t.Fatal(err)
	}
	if created, _ := Ensure(home); len(created) != 0 {
		t.Fatalf("deleted skill written again: %v", created)
	}
	if got := states(home); !slices.Equal(got, []State{Removed, NoAgent}) {
		t.Fatalf("states = %v", got)
	}

	// Deleting the record brings it back.
	if err := os.Remove(StatePath(home)); err != nil {
		t.Fatal(err)
	}
	if created, _ := Ensure(home); len(created) != 1 {
		t.Fatalf("after deleting the record: created %v", created)
	}
}

func TestSkillIsAksOwn(t *testing.T) {
	s := string(Content)
	for _, want := range []string{"name: ak\n", "description: ", marker, "ak --help"} {
		if !strings.Contains(s, want) {
			t.Errorf("SKILL.md lacks %q", want)
		}
	}
}
