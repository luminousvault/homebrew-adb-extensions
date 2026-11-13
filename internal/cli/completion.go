package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/luminousvault/homebrew-adb-extensions/internal/adb"
	"github.com/luminousvault/homebrew-adb-extensions/internal/ui"
)

// Shell completion is generated from the command table, so every shell
// completes the same commands, options and argument kinds:
//
//	ak completion zsh|bash|fish|powershell
//
// completions/_ak is the zsh output checked in for archives and make install
// (a test keeps it equal to `ak completion zsh`).

var completionShells = []string{"zsh", "bash", "fish", "powershell"}

var cmdCompletion = &Command{
	Name: "completion", Summary: "Print a shell completion script", Args: "<shell>",
	Long: "Print the tab completion script for zsh, bash, fish or PowerShell.\n" +
		"Homebrew installs bash, zsh and fish completion; 'ak doctor' shows how to set up the others.",
	ArgHelp: [][2]string{{"shell", "zsh, bash, fish or powershell"}},
	Examples: []string{
		"source <(ak completion zsh)          # in ~/.zshrc, after compinit",
		"eval \"$(ak completion bash)\"         # in ~/.bashrc",
		"ak completion fish > ~/.config/fish/completions/ak.fish",
		"ak completion powershell | Out-String | Invoke-Expression   # in $PROFILE",
	},
	NoADB: true,
}

// cmdSerials prints ready device serials for the completion scripts, using
// ak's own adb lookup (adb need not be on PATH).
var cmdSerials = &Command{
	Name: "__serials", Summary: "Print ready device serials, one per line (used by the completion scripts)", Hidden: true,
	Run: func(c *Ctx) error {
		for _, s := range adb.ReadySerials() {
			ui.Out.Println(s)
		}
		return nil
	},
}

func init() { cmdCompletion.Run = runCompletion } // Run reads Commands: set after init

func runCompletion(c *Ctx) error {
	if len(c.Args) != 1 {
		return usageErr(c.Cmd, "Pass one shell: %s", strings.Join(completionShells, ", "))
	}
	script, ok := completionScript(c.Args[0])
	if !ok {
		return usageErr(c.Cmd, "Unknown shell: %s (use %s)", q(c.Args[0]), strings.Join(completionShells, ", "))
	}
	_, err := io.WriteString(ui.Out.W, script)
	return err
}

func completionScript(shell string) (string, bool) {
	switch shell {
	case "zsh":
		return zshCompletion(), true
	case "bash":
		return bashCompletion(), true
	case "fish":
		return fishCompletion(), true
	case "powershell", "pwsh":
		return powershellCompletion(), true
	}
	return "", false
}

// ── What each argument and option value takes ────────

type argKind int

const (
	argPkg   argKind = iota // package name: nothing to offer
	argAPK                  // APK files and directories
	argDir                  // directories only (a place to save into)
	argWords                // fixed words
	argShell                // completion shells
)

type compArg struct {
	desc  string
	kind  argKind
	words []string
	many  bool
}

// positional arguments per command, in order. A command without an entry
// takes none.
var compArgs = map[string][]compArg{
	"install":     {{desc: "APK files or directories", kind: argAPK, many: true}},
	"pull":        {{desc: "destination directory", kind: argDir}},
	"extract":     {{desc: "package name or save file name", kind: argPkg}, {desc: "save file name or package name", kind: argDir}},
	"info":        {{desc: "package name", kind: argPkg}},
	"permissions": {{desc: "package name", kind: argPkg}},
	"uninstall":   {{desc: "package name", kind: argPkg}},
	"clear":       {{desc: "package name", kind: argPkg, many: true}},
	"kill":        {{desc: "package name", kind: argPkg, many: true}},
	"launch":      {{desc: "package name", kind: argPkg}},
	"restart":     {{desc: "package name", kind: argPkg}},
	"signature":   {{desc: "package name or APK file", kind: argAPK}},
	"logcat":      {{desc: "package name", kind: argPkg}},
	"screenshot":  {{desc: "output directory", kind: argDir}},
	"record":      {{desc: "output directory", kind: argDir}},
	"wifi":        {{desc: "action", kind: argWords, words: []string{"off"}}},
	"completion":  {{desc: "shell", kind: argShell}},
}

// option values that can be offered; other values are free text.
var compValues = map[string]compArg{
	"--serial": {desc: "serial"},
	"--level":  {desc: "level", kind: argWords, words: []string{"V", "D", "I", "W", "E", "F"}},
}

// options of one command that exclude each other.
var compExclusive = map[string][][]string{
	"install": {{"--latest", "--every-apk", "--filter"}},
}

func visibleCommands() []*Command {
	var cs []*Command
	for _, c := range Commands {
		if !c.Hidden {
			cs = append(cs, c)
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].Name < cs[j].Name })
	return cs
}

// shortHelp cuts "Target device (default: …)" to "Target device".
func shortHelp(f Flag) string {
	h := f.Help
	for _, sep := range []string{" (", "; "} {
		if i := strings.Index(h, sep); i > 0 {
			h = h[:i]
		}
	}
	if f == flagHelp {
		return "Show help for this command"
	}
	return h
}

func repeatable(cmd *Command, f Flag) bool { return cmd.Device == manyDevices && f.Long == "--serial" }

func names(f Flag) []string {
	var n []string
	for _, s := range []string{f.Short, f.Long} {
		if s != "" {
			n = append(n, s)
		}
	}
	return n
}

// ── zsh ──────────────────────────────────────────────

func zshQuote(s string) string { // inside '…' with [ ] : as zsh _arguments syntax
	r := strings.NewReplacer(`'`, `'\''`, `[`, `\[`, `]`, `\]`, `:`, `\:`)
	return r.Replace(s)
}

func zshAction(a compArg) string {
	switch a.kind {
	case argAPK:
		return `_path_files -g "*.apk(-.)" -/` // _path_files: no fallback to every file
	case argDir:
		return `_path_files -/`
	case argWords:
		return "(" + strings.Join(a.words, " ") + ")"
	case argShell:
		return "(" + strings.Join(completionShells, " ") + ")"
	}
	return `_message "` + a.desc + `"`
}

func zshCompletion() string {
	var b strings.Builder
	b.WriteString(`#compdef ak
# zsh completion for ak — generated by 'ak completion zsh', do not edit.

# Serials of connected devices for -s/--serial (ak finds adb itself).
_ak_serials() {
  local -a serials
  serials=(${(f)"$(ak __serials 2>/dev/null)"})
  _describe 'device serial' serials
}

_ak() {
  local -a commands
  commands=(
`)
	for _, c := range visibleCommands() {
		fmt.Fprintf(&b, "    '%s:%s'\n", c.Name, zshQuote(c.Summary))
	}
	b.WriteString(`  )

  _arguments -C \
    '(- *)'{-h,--help}'[Show help message]' \
    '(- *)'{-v,--version}'[Show version]' \
    '--no-color[Disable colored output]' \
    '1:command:->command' \
    '*::arg:->args' && return 0

  case $state in
    command)
      _describe 'ak commands' commands
      ;;
    args)
      case $words[1] in
`)
	for _, c := range visibleCommands() {
		specs := []string{`'(- *)'{-h,--help}'[Show help for this command]'`}
		for _, f := range c.AllFlags() {
			if f == flagHelp {
				continue
			}
			prefix := ""
			for _, group := range compExclusive[c.Name] {
				var others []string
				in := false
				for _, long := range group {
					if long == f.Long {
						in = true
						continue
					}
					if o := c.flag(long); o != nil {
						others = append(others, names(*o)...)
					}
				}
				if in {
					prefix = "'(" + strings.Join(others, " ") + ")'"
				}
			}
			if repeatable(c, f) {
				prefix += "'*'"
			}
			body := "[" + zshQuote(shortHelp(f)) + "]"
			if f.Value != "" {
				v, ok := compValues[f.Long]
				switch {
				case !ok:
					body += ":" + strings.ToLower(f.Value)
				case f.Long == "--serial":
					body += ":serial:_ak_serials"
				default:
					body += ":" + v.desc + ":" + zshAction(v)
				}
			}
			n := names(f)
			if len(n) == 2 {
				specs = append(specs, prefix+"{"+n[0]+","+n[1]+"}'"+body+"'")
			} else {
				specs = append(specs, prefix+"'"+n[0]+body+"'")
			}
		}
		for i, a := range compArgs[c.Name] {
			pos := fmt.Sprint(i + 1)
			if a.many {
				pos = "*"
			}
			specs = append(specs, "'"+pos+":"+zshQuote(a.desc)+":"+strings.ReplaceAll(zshAction(a), `'`, `'\''`)+"'")
		}
		fmt.Fprintf(&b, "        %s)\n          _arguments -s \\\n            %s\n          ;;\n",
			c.Name, strings.Join(specs, " \\\n            "))
	}
	b.WriteString(`        *)
          _arguments -s '(- *)'{-h,--help}'[Show help for this command]'
          ;;
      esac
      ;;
  esac
}

# Autoloaded from fpath it runs _ak; sourced (source <(ak completion zsh)) it registers it.
if [ "$funcstack[1]" = "_ak" ]; then
  _ak "$@"
else
  compdef _ak ak
fi
`)
	return b.String()
}

func (cmd *Command) flag(long string) *Flag {
	for _, f := range cmd.AllFlags() {
		if f.Long == long {
			return &f
		}
	}
	return nil
}

// ── bash ─────────────────────────────────────────────

func bashWords(ws []string) string { return "'" + strings.Join(ws, " ") + "'" }

func bashAction(a compArg) string {
	switch a.kind {
	case argAPK:
		return `COMPREPLY=($(compgen -d -- "$cur") $(compgen -f -X '!*.apk' -- "$cur"))`
	case argDir:
		return `COMPREPLY=($(compgen -d -- "$cur"))`
	case argWords: // the word list splits on IFS, which _ak sets to newline
		return `IFS=' ' COMPREPLY=($(compgen -W ` + bashWords(a.words) + ` -- "$cur"))`
	case argShell:
		return `IFS=' ' COMPREPLY=($(compgen -W ` + bashWords(completionShells) + ` -- "$cur"))`
	}
	return ":" // package name: nothing to offer
}

func bashCompletion() string {
	var b strings.Builder
	var cmds []string
	for _, c := range visibleCommands() {
		cmds = append(cmds, c.Name)
	}
	b.WriteString(`# bash completion for ak — generated by 'ak completion bash', do not edit.
# Use: eval "$(ak completion bash)"   (e.g. in ~/.bashrc; macOS bash 3.2 cannot source <(...))

# _ak_argn VALUE_OPTIONS: how many arguments come before the cursor, not
# counting options and the values of VALUE_OPTIONS.
_ak_argn() {
  local i w skip=0 n=0
  for ((i = 2; i < COMP_CWORD; i++)); do
    w="${COMP_WORDS[i]}"
    if [ "$skip" = 1 ]; then skip=0; continue; fi
    case " $1 " in *" $w "*) skip=1; continue ;; esac
    case "$w" in -*) continue ;; esac
    n=$((n + 1))
  done
  echo "$n"
}

_ak() {
  local IFS=$'\n' cur prev n
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  COMPREPLY=()
  if [ "$COMP_CWORD" -eq 1 ]; then
    IFS=' ' COMPREPLY=($(compgen -W '` + strings.Join(cmds, " ") + ` -h --help -v --version --no-color' -- "$cur"))
    return
  fi
  case "${COMP_WORDS[1]}" in
`)
	for _, c := range visibleCommands() {
		var flagNames, valueFlags []string
		var valueCases []string
		for _, f := range c.AllFlags() {
			flagNames = append(flagNames, names(f)...)
			if f.Value == "" {
				continue
			}
			valueFlags = append(valueFlags, names(f)...)
			act := "return"
			if v, ok := compValues[f.Long]; ok {
				if f.Long == "--serial" {
					act = `COMPREPLY=($(compgen -W "$(ak __serials 2>/dev/null)" -- "$cur")); return`
				} else {
					act = bashAction(v) + "; return"
				}
			}
			valueCases = append(valueCases, fmt.Sprintf("        %s) %s ;;", strings.Join(names(f), "|"), act))
		}
		fmt.Fprintf(&b, "    %s)\n", c.Name)
		if len(valueCases) > 0 {
			fmt.Fprintf(&b, "      case \"$prev\" in\n%s\n      esac\n", strings.Join(valueCases, "\n"))
		}
		fmt.Fprintf(&b, "      if [[ \"$cur\" == -* ]]; then IFS=' ' COMPREPLY=($(compgen -W %s -- \"$cur\")); return; fi\n", bashWords(flagNames))
		args := compArgs[c.Name]
		if len(args) > 0 {
			fmt.Fprintf(&b, "      n=$(_ak_argn %s)\n      case \"$n\" in\n", bashWords(valueFlags))
			for i, a := range args {
				pat := fmt.Sprint(i)
				if a.many {
					pat = "*"
				}
				fmt.Fprintf(&b, "        %s) %s ;;\n", pat, bashAction(a))
			}
			b.WriteString("      esac\n")
		}
		b.WriteString("      ;;\n")
	}
	b.WriteString(`  esac
}

complete -o filenames -F _ak ak
`)
	return b.String()
}

// ── fish ─────────────────────────────────────────────

func fishQuote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

func fishCompletion() string {
	var b strings.Builder
	b.WriteString(`# fish completion for ak — generated by 'ak completion fish', do not edit.
# Use: ak completion fish > ~/.config/fish/completions/ak.fish

# APK files and directories only (__fish_complete_suffix also lists other files).
function __ak_apk_or_dir
    set -l token (commandline -ct)
    for f in $token*
        if test -d $f
            echo $f/
        else if string match -q -- '*.apk' $f
            echo $f
        end
    end
end

complete -c ak -f
complete -c ak -n __fish_use_subcommand -s h -l help -d 'Show help'
complete -c ak -n __fish_use_subcommand -s v -l version -d 'Show version'
complete -c ak -n __fish_use_subcommand -l no-color -d 'Disable colored output'
`)
	for _, c := range visibleCommands() {
		fmt.Fprintf(&b, "complete -c ak -n __fish_use_subcommand -a %s -d %s\n", c.Name, fishQuote(c.Summary))
	}
	for _, c := range visibleCommands() {
		cond := fmt.Sprintf("-n '__fish_seen_subcommand_from %s'", c.Name)
		b.WriteString("\n")
		for _, f := range c.AllFlags() {
			opt := ""
			if f.Short != "" {
				opt += " -s " + strings.TrimPrefix(f.Short, "-")
			}
			if f.Long != "" {
				opt += " -l " + strings.TrimPrefix(f.Long, "--")
			}
			if f.Value != "" {
				v, ok := compValues[f.Long]
				switch {
				case !ok:
					opt += " -x" // takes a value, nothing to offer
				case f.Long == "--serial":
					opt += " -x -a '(ak __serials 2>/dev/null)'"
				default:
					opt += " -x -a " + fishQuote(strings.Join(v.words, " "))
				}
			}
			fmt.Fprintf(&b, "complete -c ak %s%s -d %s\n", cond, opt, fishQuote(shortHelp(f)))
		}
		for _, a := range compArgs[c.Name] {
			switch a.kind {
			case argAPK:
				fmt.Fprintf(&b, "complete -c ak %s -a '(__ak_apk_or_dir)'\n", cond)
			case argDir:
				fmt.Fprintf(&b, "complete -c ak %s -a '(__fish_complete_directories (commandline -ct))'\n", cond)
			case argWords:
				fmt.Fprintf(&b, "complete -c ak %s -a %s\n", cond, fishQuote(strings.Join(a.words, " ")))
			case argShell:
				fmt.Fprintf(&b, "complete -c ak %s -a %s\n", cond, fishQuote(strings.Join(completionShells, " ")))
			}
		}
	}
	return b.String()
}

// ── PowerShell ───────────────────────────────────────

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func psKind(a compArg) string {
	switch a.kind {
	case argAPK:
		return "apk"
	case argDir:
		return "dir"
	case argWords:
		return "words:" + strings.Join(a.words, ",")
	case argShell:
		return "words:" + strings.Join(completionShells, ",")
	}
	return "none"
}

func powershellCompletion() string {
	var b strings.Builder
	b.WriteString(`# PowerShell completion for ak — generated by 'ak completion powershell', do not edit.
# Use (e.g. in $PROFILE): ak completion powershell | Out-String | Invoke-Expression

$global:__akCompletion = @{
`)
	for _, c := range visibleCommands() {
		var flags, args []string
		for _, f := range c.AllFlags() {
			value := ""
			if f.Value != "" {
				value = "free"
				if v, ok := compValues[f.Long]; ok {
					value = psKind(v)
					if f.Long == "--serial" {
						value = "serial"
					}
				}
			}
			for _, n := range names(f) {
				flags = append(flags, fmt.Sprintf("@{ Name = %s; Desc = %s; Value = %s }", psQuote(n), psQuote(shortHelp(f)), psQuote(value)))
			}
		}
		many := "$false"
		for _, a := range compArgs[c.Name] {
			args = append(args, psQuote(psKind(a)))
			if a.many {
				many = "$true"
			}
		}
		fmt.Fprintf(&b, "  %s = @{ Desc = %s; Many = %s\n    Flags = @(\n      %s\n    )\n    Args = @(%s)\n  }\n",
			psQuote(c.Name), psQuote(c.Summary), many, strings.Join(flags, "\n      "), strings.Join(args, ", "))
	}
	b.WriteString(`}

# PowerShell falls back to file names when a completer returns nothing, so an
# empty string stands in for "no candidates".
Register-ArgumentCompleter -Native -CommandName 'ak', 'ak.exe' -ScriptBlock {
  param($wordToComplete, $commandAst, $cursorPosition)
  $out = @(& $global:__akComplete $wordToComplete $commandAst $cursorPosition)
  if ($out.Count -eq 0) { '' } else { $out }
}

$global:__akComplete = {
  param($wordToComplete, $commandAst, $cursorPosition)
  $spec = $global:__akCompletion
  $before = @($commandAst.CommandElements |
    Where-Object { $_.Extent.EndOffset -lt $cursorPosition -or ($_.Extent.EndOffset -eq $cursorPosition -and $wordToComplete -eq '') } |
    ForEach-Object { $_.ToString() })
  $result = { param($text, $tip, $type) [System.Management.Automation.CompletionResult]::new($text, $text, $type, $tip) }
  $words = { param($list) $list | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object { & $result $_ $_ 'ParameterValue' } }
  $files = { param($kind)
    [System.Management.Automation.CompletionCompleters]::CompleteFilename($wordToComplete) |
      Where-Object { $_.ResultType -eq 'ProviderContainer' -or ($kind -eq 'apk' -and $_.CompletionText -like '*.apk*') }
  }
  $offer = { param($kind)
    if ($kind -eq 'dir' -or $kind -eq 'apk') { & $files $kind }
    elseif ($kind -like 'words:*') { & $words ($kind.Substring(6) -split ',') }
    elseif ($kind -eq 'serial') { & $words @(& ak __serials 2>$null) }
  }

  if ($before.Count -le 1) {
    $spec.Keys | Sort-Object | Where-Object { $_ -like "$wordToComplete*" } |
      ForEach-Object { & $result $_ $spec[$_].Desc 'ParameterValue' }
    return
  }
  $cmd = $spec[$before[1]]
  if (-not $cmd) { return }
  if ($before.Count -gt 2) {
    $prev = $cmd.Flags | Where-Object { $_.Name -ceq $before[-1] -and $_.Value -ne '' } | Select-Object -First 1
    if ($prev) { & $offer $prev.Value; return }
  }
  if ($wordToComplete -like '-*') {
    $cmd.Flags | Where-Object { $_.Name -like "$wordToComplete*" } |
      ForEach-Object { & $result $_.Name $_.Desc 'ParameterName' }
    return
  }
  # which argument: count earlier arguments, skipping options and their values
  $n = 0; $skip = $false
  foreach ($w in $before[2..($before.Count - 1)]) {
    if ($before.Count -le 2) { break }
    if ($skip) { $skip = $false; continue }
    $f = $cmd.Flags | Where-Object { $_.Name -ceq $w } | Select-Object -First 1
    if ($f) { if ($f.Value -ne '') { $skip = $true }; continue }
    if ($w -like '-*') { continue }
    $n++
  }
  if ($cmd.Args.Count -eq 0) { return }
  if ($n -ge $cmd.Args.Count) { if ($cmd.Many) { $n = $cmd.Args.Count - 1 } else { return } }
  & $offer $cmd.Args[$n]
}
`)
	return b.String()
}
