package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Shell completion check for `ak doctor`: find the completion the user's
// shell loads and compare it with `ak completion <shell>`. A missing or old
// completion is a warning, never a problem (it does not change the exit code).

// completionEnv is where detection looks. runDoctor fills it from the system;
// tests point it at a temporary directory.
type completionEnv struct {
	goos     string
	shell    string // $SHELL
	home     string
	zdotdir  string   // $ZDOTDIR
	brew     []string // Homebrew prefixes
	zshFpath func() []string
	readFile func(string) ([]byte, error)
	isBrew   func(path string) bool // installed by Homebrew (links into a Cellar)
}

func systemCompletionEnv() completionEnv {
	home, _ := os.UserHomeDir()
	brew := []string{"/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew"}
	if p := os.Getenv("HOMEBREW_PREFIX"); p != "" {
		brew = append([]string{p}, brew...)
	}
	return completionEnv{
		goos: runtime.GOOS, shell: os.Getenv("SHELL"), home: home, zdotdir: os.Getenv("ZDOTDIR"),
		brew: unique(brew), zshFpath: zshFpath, readFile: os.ReadFile,
		isBrew: func(p string) bool {
			r, err := filepath.EvalSymlinks(p)
			return err == nil && strings.Contains(filepath.ToSlash(r), "/Cellar/")
		},
	}
}

// zshFpath asks zsh for its function path, without reading the user's rc files.
func zshFpath() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "zsh", "-fc", "print -l $fpath").Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

func unique(list []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range list {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// completionInput is what was found on disk.
type completionInput struct {
	goos   string
	shell  string // zsh, bash, fish, powershell; "" when unknown
	other  string // the unsupported $SHELL name, if any
	files  []foundFile
	rcPath string // rc/profile file that runs `ak completion <shell>`
}

type foundFile struct {
	path, content string
	brew          bool
}

// gatherCompletion reads the completion files and rc files of the user's shell.
func gatherCompletion(e completionEnv) completionInput {
	in := completionInput{goos: e.goos}
	base := strings.TrimSuffix(filepath.Base(e.shell), ".exe")
	switch {
	case e.goos == "windows":
		in.shell = "powershell"
	case base == "zsh" || base == "bash" || base == "fish":
		in.shell = base
	case base == "pwsh":
		in.shell = "powershell"
	default:
		if e.shell != "" {
			in.other = base
		}
		return in
	}
	home := func(p ...string) string { return filepath.Join(append([]string{e.home}, p...)...) }
	var files, rcs []string
	switch in.shell {
	case "zsh":
		dirs := e.zshFpath()
		if len(dirs) == 0 {
			for _, b := range e.brew {
				dirs = append(dirs, filepath.Join(b, "share/zsh/site-functions"))
			}
			dirs = append(dirs, "/usr/share/zsh/site-functions")
		}
		for _, d := range dirs {
			files = append(files, filepath.Join(d, "_ak"))
		}
		rc := home(".zshrc")
		if e.zdotdir != "" {
			rc = filepath.Join(e.zdotdir, ".zshrc")
		}
		rcs = []string{rc}
	case "bash":
		for _, b := range e.brew {
			files = append(files, filepath.Join(b, "etc/bash_completion.d/ak"))
		}
		files = append(files, "/usr/share/bash-completion/completions/ak", "/etc/bash_completion.d/ak",
			home(".local/share/bash-completion/completions/ak"))
		rcs = []string{home(".bashrc"), home(".bash_profile")}
	case "fish":
		files = append(files, home(".config/fish/completions/ak.fish"))
		for _, b := range e.brew {
			files = append(files, filepath.Join(b, "share/fish/vendor_completions.d/ak.fish"))
		}
		files = append(files, "/usr/share/fish/vendor_completions.d/ak.fish")
		rcs = []string{home(".config/fish/config.fish")}
	case "powershell":
		if e.goos == "windows" {
			rcs = []string{home("Documents/PowerShell/Microsoft.PowerShell_profile.ps1"),
				home("Documents/WindowsPowerShell/Microsoft.PowerShell_profile.ps1")}
		} else {
			rcs = []string{home(".config/powershell/Microsoft.PowerShell_profile.ps1")}
		}
	}
	for _, p := range unique(files) {
		if b, err := e.readFile(p); err == nil {
			in.files = append(in.files, foundFile{path: p, content: string(b), brew: e.isBrew(p)})
		}
	}
	for _, p := range rcs {
		if b, err := e.readFile(p); err == nil && strings.Contains(string(b), "ak completion "+in.shell) {
			in.rcPath = p
			break
		}
	}
	return in
}

// completionReport is the result, also printed by --json.
type completionReport struct {
	Shell  string           `json:"shell"`  // "" when unknown
	Status string           `json:"status"` // ok, outdated, missing, unknown
	Path   string           `json:"path"`   // the file or rc file the shell uses
	Files  []completionFile `json:"files"`  // completion files found, in load order
	goos   string
	other  string
}

type completionFile struct {
	Path     string `json:"path"`
	Current  bool   `json:"current"` // same as `ak completion <shell>`
	Homebrew bool   `json:"homebrew"`
}

// classifyCompletion decides whether the completion is current (pure).
func classifyCompletion(in completionInput) completionReport {
	r := completionReport{Shell: in.shell, Files: []completionFile{}, goos: in.goos, other: in.other}
	if in.shell == "" {
		r.Status = "unknown"
		return r
	}
	script, _ := completionScript(in.shell)
	for _, f := range in.files {
		r.Files = append(r.Files, completionFile{Path: f.path, Homebrew: f.brew,
			Current: strings.TrimSpace(f.content) == strings.TrimSpace(script)})
	}
	switch {
	case in.rcPath != "": // generated on every start: always current
		r.Status, r.Path = "ok", in.rcPath
	case len(r.Files) > 0: // the first one found is the one the shell loads
		r.Status, r.Path = "outdated", r.Files[0].Path
		if r.Files[0].Current {
			r.Status = "ok"
		}
	default:
		r.Status = "missing"
	}
	return r
}

// completionCategory renders the report as a doctor category.
func completionCategory(r completionReport) checkCategory {
	cat := checkCategory{name: "Shell completion"}
	switch r.Status {
	case "unknown":
		cat.summary = "unknown shell"
		text := "Could not tell which shell you use"
		if r.other != "" {
			cat.summary = r.other + " not supported"
			text = "ak completes zsh, bash, fish and PowerShell"
		}
		cat.items = append(cat.items, checkItem{level: levelWarn, text: text,
			hint: "See 'ak completion --help'"})
		return cat
	case "missing":
		cat.summary = r.Shell + ", not installed"
		cat.items = append(cat.items, checkItem{level: levelWarn,
			text: "No ak completion found for " + r.Shell, hint: completionInstallHint(r.Shell, r.goos)})
	case "outdated":
		cat.summary = r.Shell + ", outdated"
		f := r.Files[0]
		path := tildeHome(f.Path)
		write := " > " + path
		if !strings.HasPrefix(path, "~") { // system directory, e.g. a sudo make install
			write = " | sudo tee " + path + " > /dev/null"
		}
		hint := "Run this, then " + reopenShell(r.Shell) + ": ak completion " + r.Shell + write
		if f.Homebrew {
			hint = "Run 'brew reinstall ak', then " + reopenShell(r.Shell)
		}
		cat.items = append(cat.items, checkItem{level: levelWarn,
			text: tildeHome(f.Path) + " is from another ak version", hint: hint})
	default:
		cat.summary = r.Shell + ", " + tildeHome(r.Path)
	}
	for i, f := range r.Files {
		if i == 0 && r.Status == "outdated" {
			continue // already the warning above
		}
		state := "current"
		if !f.Current {
			state = "from another ak version"
		}
		cat.items = append(cat.items, checkItem{level: levelOK, text: tildeHome(f.Path) + " (" + state + ")"})
	}
	return cat
}

func completionInstallHint(shell, goos string) string {
	switch shell {
	case "zsh":
		return "Run this, then " + reopenShell(shell) + ": echo 'source <(ak completion zsh)' >> ~/.zshrc"
	case "bash":
		rc := "~/.bashrc"
		if goos == "darwin" { // Terminal opens login shells; bash 3.2 cannot source <(...)
			rc = "~/.bash_profile"
		}
		return "Run this, then " + reopenShell(shell) + ": echo 'eval \"$(ak completion bash)\"' >> " + rc
	case "fish":
		return "Run: mkdir -p ~/.config/fish/completions && ak completion fish > ~/.config/fish/completions/ak.fish"
	default:
		return "Run this in PowerShell, then " + reopenShell(shell) +
			": Add-Content $PROFILE 'ak completion powershell | Out-String | Invoke-Expression'"
	}
}

func reopenShell(shell string) string {
	switch shell {
	case "zsh", "bash", "fish":
		return "open a new terminal (or run 'exec " + shell + "')"
	}
	return "open a new terminal"
}
