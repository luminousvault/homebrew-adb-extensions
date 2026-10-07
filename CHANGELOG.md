# Changelog

All notable changes to this project will be documented in this file.

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
