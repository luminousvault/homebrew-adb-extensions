# Changelog

All notable changes to this project will be documented in this file.

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
