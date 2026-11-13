package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

// These run the generated scripts in real shells when the shell is installed
// (CI: bash and pwsh on ubuntu, pwsh on windows, bash and zsh on macos).

var (
	akBinOnce sync.Once
	akBinDir  string
	akBinErr  error
)

// akOnPath builds ak once and returns a PATH with it first.
func akOnPath(t *testing.T) string {
	t.Helper()
	akBinOnce.Do(func() {
		akBinDir, akBinErr = os.MkdirTemp("", "ak-bin-")
		if akBinErr != nil {
			return
		}
		name := "ak"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		out, err := exec.Command("go", "build", "-o", filepath.Join(akBinDir, name), "../../cmd/ak").CombinedOutput()
		if err != nil {
			akBinErr = err
			t.Logf("%s", out)
		}
	})
	if akBinErr != nil {
		t.Fatal(akBinErr)
	}
	return akBinDir + string(os.PathListSeparator) + os.Getenv("PATH")
}

// fixture: a folder with a text file, an APK and a subdirectory.
func completionFixture(t *testing.T) string {
	dir := t.TempDir()
	for _, f := range []string{"notes.txt", "app.apk"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func shellLines(out string) []string {
	var l []string
	for _, s := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if s = strings.TrimSpace(s); s != "" {
			s, _, _ = strings.Cut(s, "\t") // fish: "candidate<TAB>description"
			l = append(l, strings.TrimSuffix(strings.TrimPrefix(strings.ReplaceAll(s, `\`, "/"), "./"), "/"))
		}
	}
	sort.Strings(l)
	return l
}

type completionCase struct {
	line string // what is typed before Tab
	want []string
}

var completionCases = []completionCase{
	{"ak scr", []string{"screenshot"}},
	{"ak screenshot ", []string{"sub"}},                       // directories only
	{"ak install ", []string{"app.apk", "sub"}},               // APKs and directories
	{"ak signature ", []string{"app.apk", "sub"}},             // any path, not only ./*.apk
	{"ak extract com.example.app ", []string{"sub"}},          // the save name: directories
	{"ak info ", nil},                                         // a package name: nothing
	{"ak wifi ", []string{"off"}},                             // fixed words
	{"ak logcat -l ", []string{"D", "E", "F", "I", "V", "W"}}, // option values
	{"ak completion ", []string{"bash", "fish", "powershell", "zsh"}},
}

func runCompletionCases(t *testing.T, complete func(dir, path, line string) string) {
	dir := completionFixture(t)
	path := akOnPath(t)
	for _, c := range completionCases {
		got := shellLines(complete(dir, path, c.line))
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Errorf("%q: got %q, want %q", c.line, got, c.want)
		}
	}
}

func TestBashCompletion(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil || runtime.GOOS == "windows" {
		t.Skip("bash not available")
	}
	runCompletionCases(t, func(dir, path, line string) string {
		words := strings.Split(line, " ") // the last element is the word being typed ("" after a space)
		var quoted []string
		for _, w := range words {
			quoted = append(quoted, "'"+w+"'")
		}
		script := `eval "$(ak completion bash)"; COMP_WORDS=(` + strings.Join(quoted, " ") + `); COMP_CWORD=` +
			itoa(len(words)-1) + `; _ak; printf '%s\n' "${COMPREPLY[@]}"`
		cmd := exec.Command("bash", "-c", script)
		cmd.Dir, cmd.Env = dir, append(os.Environ(), "PATH="+path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%q: %v\n%s", line, err, out)
		}
		return string(out)
	})
}

func TestFishCompletion(t *testing.T) {
	if _, err := exec.LookPath("fish"); err != nil {
		t.Skip("fish not installed")
	}
	runCompletionCases(t, func(dir, path, line string) string {
		cmd := exec.Command("fish", "--no-config", "-c", `ak completion fish | source; complete -C "`+line+`"`)
		cmd.Dir, cmd.Env = dir, append(os.Environ(), "PATH="+path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%q: %v\n%s", line, err, out)
		}
		return string(out)
	})
}

func TestPowerShellCompletion(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh not installed")
	}
	runCompletionCases(t, func(dir, path, line string) string {
		script := `ak completion powershell | Out-String | Invoke-Expression; ` +
			`(TabExpansion2 -inputScript '` + line + `' -cursorColumn ` + itoa(len(line)) + `).CompletionMatches | ForEach-Object { $_.CompletionText }`
		cmd := exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-Command", script)
		cmd.Dir, cmd.Env = dir, append(os.Environ(), "PATH="+path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%q: %v\n%s", line, err, out)
		}
		return string(out)
	})
}

func itoa(n int) string { return strings.TrimSpace(strings.Repeat(" ", 0) + fmtInt(n)) }

func fmtInt(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
