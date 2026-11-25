# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- `info`: more rows from the same `dumpsys package`: `system`, `enabled` (`yes`, `no`, `no (by user)`, `no (until used)`), `installed` and `updated` times, `path` (with `(+N splits)` for split APKs), `uid`, and `pid` while the app is running. For an updated system app the values come from the current package, not the hidden system one. `--json` adds `system`, `enabled`, `firstInstallTime`, `lastUpdateTime`, `path`, `splits`, `uid` and `pid`; existing keys are unchanged
- `ak sysinfo`: a device's "About phone" in one place: model, manufacturer and device name; Android version, API level, security patch and build; hardware serial, IMEI (and the second IMEI on dual-SIM devices), Android ID; Wi-Fi IP, Wi-Fi and Bluetooth MACs; CPU ABI and screen size (with the size and density in use when changed). The first row is the adb `serial`, so saved output says which device it is. Values a device doesn't expose are `-`, and a hint shows how to open `*#06#` when the IMEI can't be read. `-s` repeated or `-a` for several devices; `--json` always prints an array

### Changed
- `info`: the package name is now the first line of the result, so it stays when messages are hidden (`2>/dev/null`), and the title no longer repeats it, saying only where (`==> App info on <device> (foreground app)`). versionName and versionCode are shown together as `version`, SDK levels get their Android version (`34 (Android 14)`), and `installerPackageName` is now `installer`, as in `--json` (`--json` is unchanged)
- `mac` is hidden from the help and Tab completion; its MACs are now part of `ak sysinfo`. `ak mac` still works; its device line is now a `==>` title on stderr and a missing MAC is `-` instead of `n/a` (`--json` is unchanged)
- Messages name an app the same way everywhere: `'pkg' on <device>`, or `'pkg' (foreground app) on <device>` when no package was given. This covers the titles of `permissions` and `logcat`, the spinner of `extract`, and the `✓` lines of `uninstall`, `kill` and `clear` (which now name the device too). Where the first row of the result already names the app, the title says only where: `==> App info on <device>`, `==> Signature on <device>` (`==> Signature of the APK file` for a local APK) and `✓ Restarted on <device>`, with ` (foreground app)` after the device when no package was given. `activities` gets a title, `==> Activity stack on <device>`
- The result on stdout says whose it is in its first row: `restart` starts with `package`, and `signature` with `package` (or `file` for a local APK) before `Signer #1`
- `ak <Tab>` no longer offers `completion`, since Tab working means it is already set up; `ak --help` still lists it and `ak completion <Tab>` still offers the shells
- `info`: `debuggable` and `system` read `yes`/`no` instead of `true`/`false` (`--json` keeps booleans)
- `extract` and `pull` print the path of each saved file on stdout, and `push` the path on the device, one per line, as `screenshot` and `record` do
- The details after a `✓` line are always `(device, size, time)`, with the parts a command has: `extract`, `pull`, `push` and `record` (e.g. `✓ Pulled 'a.pdf' (Pixel 9 Pro, 2 KB, 0.4s)`). `install` keeps `✓ Installed 'app.apk' on <device> (3.2s)`, since that line also appears after a `[device]` prefix. `pull` no longer repeats the device folder there (the picker shows it), and `push` names the device path as `'/sdcard/Download/a.png'` instead of `<device>:/sdcard/Download/a.png`
- `permissions` with nothing granted says `No granted permissions` instead of `(none)`
- `--json` of `info`, `permissions` and `restart` adds `serial` and `device`, and `signature` of an installed app adds `device`; existing keys are unchanged
- Errors name the same target as the success line would, device included (`Error: Couldn't stop 'pkg' on <device>`, `Couldn't extract …`, `Couldn't pull the APK of …`), and put it first: `'x' is not installed on <device>`, `'x' is not a valid package name`, `'x' is not a directory`, `'x' was not found`. Every failure reads `Couldn't <verb> …`, including the totals (`Couldn't pull 1 of 3 files`, `Couldn't install on 1 of 2 devices`), and names, typed values and serials are in quotes (`Unknown option '-x'`, `Unknown command 'x'`, `Unknown argument 'on' (use off)`, `Unexpected argument 'x'`)
- Every `Hint:` is one instruction that starts with a verb, with commands in quotes and `-s <serial>` when several devices are connected. A usage error ends with `Hint: Run 'ak <cmd> --help' to see the options` instead of `Run 'ak <cmd> --help' for usage.`, and more errors get a hint: a package that isn't installed or valid, `install -f` and `pull -f` matching nothing, a missing APK file in `signature`, `ak wifi off -s` on a USB device, an empty screen in `activities`
- `pull` with no recent files on the device and `activities` with no activities say `No …` and exit 0 instead of failing (`activities --json` prints `[]`). Declining to create the `pull` directory says `Skipped: '<dir>' was not created` instead of `Aborted`, and the question is one sentence: `Create the missing directory '<dir>'? [y/N]`
- Progress lines say where: `Capturing the screen on <device>`, `Pulling 'a.pdf' from <device>`, `Recording the screen on <device> (0:12, Ctrl+C to stop)`. Without a terminal, a step that changes mid-way (`Stopping the recording …`, `Saving the recording …`, `Connecting to …`) is printed as its own `==>` line, while the recording clock is not. `restart -r 1` drops the `(1/1)` counter, and the install board numbers APKs `(1/2)` instead of `[1/2]` and writes `Installing…`
- Times are `120ms` under a second and `1.2s` above it, never `0.0s`, in `install`, `extract`, `pull`, `push` and the install board; `screenshot` adds its time to the `✓` line, and `restart` shows launch times as `105ms` instead of `105 ms`. Counts read `1 APK` / `3 APKs` instead of `APK(s)`
- `push` and `pull` print the "already exists, saving as …" warning after the step it is about, and `screenshot -c` with a file says `Copied the screenshot to the clipboard` like `-c` alone. Ctrl+C during a multi-device install ends with a `Warning:` about incomplete installs instead of an `Error:` line (exit 130 as before)

## [2.4.1] - 2026-11-20

### Added
- `files`: Tab in Go to (`:`) completes a folder name as a shell does: one match is filled in, several are filled to the part they share and listed in columns over the list. Tab again picks from them as zsh's menu does: Tab and Shift+Tab go to the next and previous folder, the arrows move in the list, Enter goes there and Esc puts the text back

### Changed
- `files`: copying or moving several items shows each one in the folder as it finishes, instead of all at the end; a finished item is kept even if a later one fails or the copy is canceled

## [2.4.0] - 2026-11-20

### Added
- `ak push [file]...` copies files to the device, into `/sdcard/Download` unless `-d, --dest DIR` gives another folder. It never overwrites: a name already there gets a ` (N)` suffix. Without files it picks from the current directory; `-s` repeated or `-a` pushes to several devices
- `ak files [path]`: a file manager for the device and this computer, side by side in the terminal. Copy, cut and paste between them or within one side, send to the other side with one key, rename, make folders and delete, with a recent-files view, progress bars and Esc to cancel. Nothing is overwritten without asking, and an overwrite keeps the old file until the new one is complete. Number keys like mc's; `?` shows every key ([docs](docs/commands.md#ak-files))
- Tab completion offers files for `push`
- ak adds a skill for AI coding agents the first time it runs: `~/.claude/skills/ak/SKILL.md` and `~/.agents/skills/ak/SKILL.md`, only where `~/.claude` or `~/.agents` exists, with one `✓ Added the ak skill for AI agents` line per folder. The skill tells agents to use ak instead of raw adb and to read `ak --help` first. ak updates its copy after upgrades, leaves a copy alone once its `managed-by: ak` line is removed, and does not add a deleted one back (it remembers in `~/.ak/skills-installed`). `AK_NO_SKILL=1` turns it off; `ak completion` never adds it. `ak doctor` has a new "AI agent skill" category (never an issue) and `agentSkill` in `--json`

### Changed
- `record` has no time limit by default: it records until Ctrl+C, and `-t 0` asks for that explicitly. Devices whose `screenrecord` cannot drop the limit (its `--help` lacks "Set to 0 to remove the time limit") keep the old 180-second cap, and `-t 0` or over 180 fails there with a hint
- `pull` no longer overwrites a file already in the destination; it saves the new one with a ` (N)` suffix
- `pull` lists a file that was just written (before MediaStore scans it), with its real size
- `pull` shows a spinner while it reads the recent files
- Korean names stored as separate jamo (as macOS saves them) line up correctly in all output

### Fixed
- `permissions` listed each permission twice for updated system apps (e.g. Chrome): it also read the factory version under `Hidden system packages:`
- `launch` on a package without a launcher activity printed a Java stack trace; it now says `'<package>' has no launcher activity`
- `extract` warns when the app has split APKs, since the base APK alone may not install, and hints at `adb install-multiple`
- `activities` picks the foreground task from the resumed activity line; Android 16 prints the last paused activity first, which could show the previous app
- `restart` no longer counts a start that reused a running instance (`Activity not started`, `TotalTime: 0`) as a 0 ms launch, and an activity named like `ErrorActivity` no longer makes a successful start fail

## [2.3.0] - 2026-11-13

### Added
- `pull -l, --latest` pulls the newest file and `pull -f, --filter WORDS` keeps the files whose names contain all the words (like `install -f`); one match is pulled without asking, and with `-l` the newest match. `pull` can now run without a terminal; without these options the error says so
- `ak --help` has a short section for scripts and AI agents (name the package, no pickers without a terminal, `--json`); the README section is now "Scripts, CI and AI agents"

### Changed
- Tab after `-s, --serial` shows what each device is: in zsh and fish the serials are listed one per line with the model and Android version (`R3CW80E6Y8Z  -- Samsung SM-S928N, Android 15 (API 35)`), and PowerShell shows the same in its menu and tooltip. bash still offers the serials alone, since it cannot show descriptions
- `screenshot` warns when the device screen is off (asleep or dozing), since the image is then black or stale: `Warning: The screen of <device> is off; the image may be black` and a `Hint:` with the `adb shell input keyevent WAKEUP` command. The exit code stays 0
- Commands that take the foreground app when no package is given (`info`, `restart`, `kill`, `clear`, `uninstall`, `logcat`, …) warn when the screen is off: the app found is the one that was on screen before it turned off
- `record` fails right away when the screen is off, with a hint to turn it on. Before, it started `screenrecord`, waited about 10 seconds and then reported `UNASSIGNED_LAYER_STACK`

### Fixed
- `logcat` and `record` stop cleanly when ak is ended by `kill` or `timeout` (SIGTERM) or by closing the terminal (SIGHUP), as they do on Ctrl+C. Before, `adb logcat` kept running in the background, and `record` left the recording going on the device without saving it

## [2.2.0] - 2026-11-13

### Added
- `logcat` takes several packages: `ak logcat com.example.app com.example.store` merges their lines in time order, each marked with a bar in the app's color and its package name (in a terminal as much as fits in a fifth of the width, up to 16 columns, shortened from the front, e.g. `…onesdk.demo.android`)
- `logcat -t, --tag` shows only the given tags and `-i, --ignore-tag` hides tags (both repeatable, exact names, `*` matches any text)

### Changed
- `logcat` hides what Android framework code logs from inside the app (views, input method, graphics, class loading, ART's GC lines, Samsung's additions) except errors, and GPU driver lines (Adreno, gralloc) except fatal ones; crashes and `Choreographer` stay. `-e, --every-tag` shows them; a hint says so the first time a line is hidden
- `logcat` colors lines like Android Studio's Logcat: the level as a badge, the message in the level's color, and each tag bold in its own color (the same hue Android Studio picks for it). In a terminal tags line up in a column that grows to the longest tag so far (up to 23 columns; longer tags are shortened in the middle), and a long message wraps with its next lines indented to where the message starts. Output to a pipe or file is unchanged

## [2.1.1] - 2026-11-13

### Changed
- `screenshot --copy` is renamed `--clipboard`, the name screenshot tools use for this; `-c` is unchanged. `--copy` is no longer accepted

## [2.1.0] - 2026-11-13

### Added
- `ak doctor` looks like `flutter doctor`: a header box, one `[✓]` / `[!]` / `[✗]` line per category with the fix under it, `-v, --verbose` for every detail, and a closing "N/M checks passed" count; `-v` ends with an environment block to paste into bug reports (ak, OS, shell, adb, devices with their Android version, no serials). A missing apksigner is no longer an issue (it is only a fallback). New category: shell completion — whether ak's completion is set up for your shell and current (warning only, the exit code is unchanged)
- `ak completion zsh|bash|fish|powershell` prints a completion script generated from the command table; Homebrew installs bash, zsh and fish completion. Arguments complete only what they take (APK files and directories, directories only, fixed words) and `-s` lists connected serials through ak's own adb lookup
- `screenshot -c, --copy` copies the image to the clipboard. Alone it saves no file; with a file or directory argument, or with `-o`, the file is saved too. One device at a time (macOS puts PNG plus a compressed TIFF on the pasteboard so clipboard history apps such as Raycast record it; Linux `wl-copy` or `xclip`; Windows PowerShell)

### Changed
- Release archives are named as users see their machine: `ak_<version>_macos.tar.gz` holds one universal binary (Apple Silicon and Intel), Windows is `windows_x64` / `windows_arm64`, Linux is `linux_x86_64` / `linux_arm64` (was `darwin_amd64`, `darwin_arm64`, `windows_amd64`, `linux_amd64`, …). Homebrew picks the right one as before
- README rewritten (what ak saves you over plain adb, install incl. the macOS quarantine note, demo GIFs recorded with a fake adb), with a generated command reference (`docs/commands.md`, Korean `docs/commands.ko.md` from `docs/i18n/ko.json`; any `docs/i18n/<lang>.json` adds a language) and `docs/troubleshooting.md`. Help texts say what each command and `-y` do
- Personal project: CONTRIBUTING.md removed; pull requests are not accepted, issues are welcome

### Fixed
- Tab completion offers only what an argument takes: directories for `screenshot`, `record`, `pull` and the save name of `extract`; APK files and directories for `install` and `signature` (any path, not only the current folder). It never falls back to listing every file
- With one device, declining the uninstall-and-reinstall question stops the remaining APKs, as it does with several devices

## [2.0.0] - 2026-11-12

### Changed
- Rewritten in Go as a single binary; no Bash, `awk`, `sed`, `tput` or `stty` needed. Command names and Zsh completion stay; options and output changed as listed under "Changed (Breaking)"
- `signature` no longer needs Android SDK build-tools: certificates are read directly from the APK, printed as `Signer #N certificate ...` regardless of apksigner version (apksigner is used only as a fallback)
- `install` reads the package name for downgrade recovery from the APK itself, so `aapt` is no longer required
- adb is looked up in `AK_ADB`, `ANDROID_HOME`, `ANDROID_SDK_ROOT`, `PATH`, the Android Studio default SDK and `~/.ak/platform-tools`, so Android Studio users no longer need adb on `PATH`
- Prebuilt binaries for macOS, Linux and Windows (amd64/arm64) are attached to each GitHub release; the Homebrew formula installs them, so no Go toolchain is needed

### Changed (Breaking)
- New output design for every command (`docs/dev/cli-design.md`): results on stdout and messages on stderr; `==>`, `✓`, `Warning:`, `Error:`, `Hint:` messages; colors only on terminals (`NO_COLOR`, `--no-color`, `CLICOLOR_FORCE`). Scripts that parsed v1.x output need updating
- Exit codes: `0` success (including `--help`), `1` failure including partial failure (e.g. one of several packages, failed installs), `2` usage error, `130` interrupted. v1.x exited 1 for `--help` and 0 for several failures
- Help is generated from command definitions and printed to stdout; options may come before or after arguments; `--opt=value` and `--` are supported; short options can be combined (`-ea`, `-clW`)
- `devices` prints a table (`SERIAL MODEL ANDROID ABI STATE`) with bold headers and, on a terminal, a rule under them; piped output is plain columns; `activities`, `signature`, `mac` and `info` print aligned plain output
- Option names follow one rule (`docs/dev/cli-design.md`): `-a, --all` always means all devices, `-e, --every-…` means every item of the command
  - All devices: `-m` is now `-a, --all` (`install`, `mac`, `screenshot`)
  - `install -a, --all-apks` is now `-e, --every-apk`; `activities --all` is now `-e, --every-task`
  - `restart -n` is now `-r, --repeat`; `install -t`/`-d` gain `--test-only`/`--downgrade`
- `install` with APK files installs them all; the picker is used for directories or no argument
- Pickers open on the alternate screen: the terminal's earlier output and scrollback are kept (v1.x cleared them), and the choice is printed as `==> Selected …` after the picker closes
- `pull` asks before creating a missing directory (`-y` to skip)

### Added
- `doctor` command: checks the adb in use, other adb binaries with a different version (mixed versions restart the adb server and drop connections), connected devices (unauthorized, offline, `ANDROID_SERIAL` not connected) and optional tools; exits 1 when a problem is found
- Device targeting for all device commands: `-s, --serial`, `ANDROID_SERIAL`, and `-a, --all` for `install`, `mac` and `screenshot`. Without a terminal, ak asks for `-s` instead of opening a picker
- `install` on several devices runs in parallel with a status board; a device that needs confirmation is asked right away (`[Y/n/a]`) while the others keep installing, further questions queue up, and a summary follows
- `--json` for `devices`, `info`, `permissions`, `signature`, `mac`, `activities`, `doctor`
- `-y, --yes` for `install` and `pull`
- Spinners for installs and pulls; zsh completion of device serials for `-s`
- New commands that replace several adb steps each:
  - `screenshot`: capture with `exec-out screencap` straight to a PNG (no temp file on the device), one file per device with `--all`; `-o, --open` opens the file
  - `record`: `screenrecord` until Ctrl+C or the time limit; stops it with SIGINT so the MP4 is finalized, waits for the file, pulls it and deletes it from the device; `-o, --open` opens the video
  - `logcat`: logs of one app only, following new PIDs across restarts and announcing process start/kill/death
  - `restart`: force-stop + launch with `am start -W` timing; `-n` for min/avg/max, `--json`
  - `wifi`: `tcpip` + Wi-Fi IP lookup + `connect` in one step; `wifi off` switches back to USB
- `install -e` lists the APKs (size and time) before installing them
- `install` shows `(check the device screen)` when a step takes more than 10 seconds (Play Protect or a USB-install prompt waiting on the device)
- `make install` / `make uninstall` for local source installs (binary + zsh completion under `/usr/local`), replacing `./build.sh install`

### Fixed
- Ctrl+C during `install` stops at once: no further APK, retry or uninstall starts after it (v1.x kept going), and the summary marks the interrupted APK
- Pickers fit small terminals (never taller than the window) and redraw as soon as the window is resized
- `install -f <filter> <dir>` no longer installs a non-matching APK when the directory contains exactly one APK
- `activities` no longer mixes in activities of task 12 when showing task 2 (and similar)

## [1.4.0] - 2026-11-11

### Added
- `devices`: CPU column now shows 32/64-bit app support derived from `ro.product.cpu.abilist32`/`abilist64` (`(64/32)`, `(64only)`, `(32only)`); `(64only)` is highlighted since 32-bit apps cannot be installed

## [1.3.0] - 2026-10-07

### Added
- New `mac` command: show device network identifiers
  - Current Wi-Fi MAC with a fallback chain (`cmd wifi status` on API 30+ → `dumpsys wifi` → `ip link` → `/sys/class/net/wlan0/address` → `ifconfig`); only reported while connected, so a stale MAC or placeholder `02:00:00:00:00:00` is never shown
  - Randomized MAC detection via the locally administered bit
  - Factory Wi-Fi MAC (`wifi_sta_factory_mac_address`), available even when Wi-Fi is off; if not exposed, a hint on how to see it is shown
  - Connected SSID and Bluetooth MAC (`settings get secure bluetooth_address` → `dumpsys bluetooth_manager`), `n/a` when unavailable
  - `-m` shows all connected devices

## [1.2.0] - 2026-07-14

### Added
- New `pull` command: pull recent files from device
  - Lists the 300 most recently modified files via MediaStore (like the Files app 'Recents' tab, no filesystem recursion)
  - Duplicate file names among selected files are saved with a ' (N)' suffix instead of silently overwriting each other
  - Non-existent destination directories require confirmation before being created
  - Interactive multi-select with filename filter; each item shows its folder and size
  - Destination handling: no argument pulls into the current directory; a directory argument (created if needed) pulls into it; files keep their original names

### Changed
- **Breaking:** APK extraction command renamed from `pull` to `extract` (same usage and behavior); `pull` is now the recent-file puller

### Fixed
- Interactive multi-select UI returned wrong indices after toggling name sort (S key), causing the wrong items to be selected (affected `install` and the new `pull`)
- Number-key selection (1-9) in multi-select mode now maps through the current sort/filter order

## [1.1.6] - 2026-06-22

### Improved
- Filter mode (`/`) now uses substring matching instead of fuzzy (subsequence) matching
  - Only items containing the exact typed string consecutively are shown
  - Highlight now covers the matched block as a single contiguous span
- `install` command APK selection UI now shows source folder label per item
  - When APKs come from multiple directories, each item displays its source folder name (e.g. `(directory)`) in dim color after the filename
  - Label is omitted when all APKs share the same source directory
  - Direct APK file arguments use their parent directory as the source
  - Filter mode (`/`) matches only against the APK filename, not the folder label

## [1.1.5] - 2026-02-05

### Added
- Location information display in APK selection UI
  - Shows current directory or source directories when selecting APKs
  - Multiple directories displayed with bullet (•) separator
  - Home directory paths abbreviated with `~` for cleaner display

### Improved
- `devices` command output format
  - Changed to table format with Device ID as the first column
  - Status displayed as clear strings (Connected/Unauthorized/Offline) instead of symbols
  - Device ID always visible even when `getprop` fails, preventing confusion
  - Improved error handling: shows "Unknown" values when device properties cannot be retrieved
  - Subtle color scheme: dimmed headers, minimal data coloring for better readability
- Device selection UI error handling: shows "Unknown Device (device_id): device not responding" when `getprop` fails
- `extract_unique_directories` function for bash 3.x compatibility
  - Replaced associative array with array iteration for older macOS systems
  - Improved error handling for relative path conversion failures

## [1.1.4] - 2026-01-28

### Changed
- `install` command option renamed: `-p` → `-f` (filter)
  - Changed option name from `-p` to `-f` to avoid potential conflicts
  - Updated terminology: "pattern" → "filter" throughout codebase and documentation
  - All examples and help text updated to reflect new option name

### Improved
- `install` command option parsing
  - Replaced `getopts` with manual parsing to allow options anywhere in the argument list
  - Options now work correctly even when placed after positional arguments (directories/APK files)
  - Examples: `ak install /path/to/dir -f debug` and `ak install app.apk -m -t` now work as expected
  - All options (`-f`, `-l`, `-a`, `-m`, `-t`, `-d`, etc.) are now position-independent
  - Structurally separated option parsing from positional argument handling
- Interactive UI layout and stability
  - Filter box now appears naturally below help text instead of being pinned to the bottom
  - Implemented height stabilization in filter mode to prevent UI "jumping" when list shrinks
  - Added 1-line safety margin to terminal height calculations to prevent rendering breaks
  - Improved terminal resize (SIGWINCH) handling to correctly recalculate layouts in all modes

### Fixed
- UI rendering glitches during terminal resizing and filtering
- Incorrect padding calculation in filter mode for short lists
- Options not recognized when placed after positional arguments in `install` command

## [1.1.3] - 2026-01-15

### Improved
- Code quality improvements
  - Fixed all 61 ShellCheck lint warnings across the codebase
  - Separated variable declarations and assignments to prevent exit code masking
  - Added shellcheck directives for variables used across modules
  - Enhanced code maintainability and error handling
- Build process optimization
  - Debug logging code automatically removed from production builds
  - Debug functions remain available in development source files
  - Cleaner production code without development overhead
- Install command autocompletion enhanced
  - Added directory support alongside APK file completion
  - Allows easier navigation to APK files in subdirectories

### Fixed
- Filter mode rendering bug when terminal window is resized
  - Fixed padding calculation to reset on terminal resize
  - Filter box now renders at correct position after window size changes
  - Normal mode continues to work correctly during resizing

## [1.1.2] - 2025-12-31

### Fixed
- Multi-selection state preserved correctly after sorting
  - Fixed incorrect index mapping when toggling sort order
  - Selection status now properly maintained across sort mode changes
- Display numbers now show sequential order (1, 2, 3...) after sorting
  - Fixed display index calculation to reflect current sort order
  - Previously showed original indices causing non-sequential numbering

## [1.1.1] - 2025-12-31

### Changed
- Interactive UI selection key changed from Tab to Space
  - More intuitive key binding aligned with common UI patterns
  - Updated help text to reflect new key binding

### Improved
- APK path extraction function (`get_apk_path` → `get_apk_path_for_package`)
  - Renamed for clarity: explicitly indicates package-to-APK-path resolution
  - Switched from `pm list packages -f` to `pm path` for better reliability

### Fixed
- Filter mode rendering bug when moving cursor left/right during search
  - Cursor position now properly maintained during filter input
- Filter box position not fixed during search in filter mode
  - Filter box now remains stable at its designated position
- Signature extraction failure on certain APKs with different signing schemes
  - Implemented automatic fallback mechanism for apksigner verify
  - First attempts without options (compatible with v2/v3 signature APKs)
  - Retries with --min-sdk-version 21 if signature info not extracted (compatible with legacy MD5 signature APKs)
  - Supports both modern and legacy APK signing methods

## [1.1.0] - 2025-12-30

### Added
- Interactive UI for signature command
  - Lists foreground apps from all connected devices with device info
  - Lists APK files in current directory
  - Arrow keys and number keys for quick selection
  - Multi-device support
- Filtering feature (`/` key)
  - Real-time incremental search with highlight
  - Case-insensitive matching
  - Bracketed paste mode for safe clipboard input
- Sorting feature (`S` key)
  - Toggle between original/time-newest/name-ascending order
  - Dynamic status display in help text
- Debug logging system
  - Detailed event logging with timestamps
  - Source file and line number tracking

### Improved
- Interactive UI help text
  - Condensed from 2 lines to 1 line with pipe separators
  - Color-highlighted keys (cyan) for better visibility
  - Semantic labels: "select" (single) vs "confirm" (multi)
- Filtering performance
  - Pre-computed lowercase transformations
  - Optimized highlight computation
  - Separated into dedicated `filter.sh` module
- Scrolling window for long lists
  - Auto-adjusts to terminal height
  - SIGWINCH handler for instant resize response
  - Position indicator (e.g., "Showing 5-15 / 30")

### Changed
- `detect_foreground_package()` now accepts optional device_id parameter
- Added `get_apk_list()` function for APK file management

### Fixed
- UI content preservation when exiting interactive mode
  - Proper cursor positioning before alternate screen exit
- Terminal size validation
  - Minimum 15 lines required with clear error message
  - Graceful degradation for small terminals

## [1.0.3] - 2025-12-22

### Fixed
- Fixed ADB and Android SDK detection when only platform-tools is installed via Homebrew
- Improved error messages for missing Android tools with installation guide

### Changed
- Release workflow now requires CHANGELOG entry (fails if missing)

## [1.0.2] - 2025-12-19

### Changed
- Switched from shc binary to shell script distribution for cross-platform compatibility
- Removed shc dependency from build process
- Supports all Unix-like systems (macOS, Linux, WSL) with single distribution

### Fixed
- Resolved file permission issues during Homebrew installation
- Fixed platform-specific binary execution errors

## [1.0.1] - 2025-12-19

### Fixed
- Homebrew installation failure due to missing build artifacts
- Included prebuilt files in release archive

## [1.0.0] - 2025-12-19

### First Public Release

**Core Features:**
- APK management: install, pull
- App information: info, permissions, signature, activities
- App control: launch, kill, clear, uninstall
- Device management: devices

**Architecture:**
- Modular source structure (src/lib/)
- Build system (single file merge)
- Binary compilation support (shc)
- Zsh completion support

**Installation:**
- Homebrew support
- Source build support
