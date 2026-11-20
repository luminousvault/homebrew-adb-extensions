package cli

import (
	"os"
	"path/filepath"

	"github.com/luminousvault/homebrew-adb-extensions/internal/agentskill"
	"github.com/luminousvault/homebrew-adb-extensions/internal/debuglog"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// installAgentSkill puts ak's skill in ~/.claude and ~/.agents (where they
// exist) on the first run, and keeps it current after upgrades. Failures only
// go to the debug log: they must not get in the way of the command.
func installAgentSkill() {
	if os.Getenv("AK_NO_SKILL") != "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	created, err := agentskill.Ensure(home)
	if err != nil {
		debuglog.Logf("agent skill: %v", err)
	}
	for _, p := range created {
		ui.Success("Added the ak skill for AI agents: %s", tildeHome(filepath.Dir(p)))
	}
	if len(created) > 0 {
		ui.Detail("Delete it to remove it; set AK_NO_SKILL=1 to never add it")
	}
}

// agentSkillCategory reports the skill in ak doctor. It is never an issue.
func agentSkillCategory(off bool, home string, statuses []agentskill.Status) checkCategory {
	cat := checkCategory{name: "AI agent skill"}
	if off {
		cat.summary = "off"
		cat.items = []checkItem{{text: "AK_NO_SKILL is set, so ak does not add or update its skill"}}
		return cat
	}
	var where []string
	for _, s := range statuses {
		dir := tildeHome(filepath.Dir(s.Path))
		switch s.State {
		case agentskill.NoAgent:
			continue
		case agentskill.Current:
			where = append(where, dir)
			cat.items = append(cat.items, checkItem{text: dir + " is current"})
		case agentskill.Outdated, agentskill.Missing: // ak fixes these on every run; only seen when writing failed
			cat.items = append(cat.items, checkItem{level: levelWarn, text: "Could not write " + dir,
				hint: "Run with AK_DEBUG=1 to see why"})
		case agentskill.Edited:
			where = append(where, dir)
			cat.items = append(cat.items, checkItem{text: dir + " has your edits; ak leaves it as is"})
		case agentskill.Removed:
			cat.items = append(cat.items, checkItem{text: dir + " was removed; ak does not add it again",
				details: []string{"To add it back, delete " + tildeHome(agentskill.StatePath(home)) + " and run ak"}})
		}
	}
	switch {
	case len(cat.items) == 0:
		cat.summary = "no AI agent found"
		cat.items = []checkItem{{text: "ak adds its skill when ~/.claude or ~/.agents exists"}}
	case len(where) == 0:
		cat.summary = "not installed"
	default:
		cat.summary = where[0]
		if len(where) > 1 {
			cat.summary += " and " + where[1]
		}
	}
	return cat
}
