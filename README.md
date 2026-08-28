[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Version](https://img.shields.io/github/v/release/luminousvault/homebrew-adb-extensions?color=blue)](https://github.com/luminousvault/homebrew-adb-extensions/releases/latest)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20WSL-lightgrey.svg)
![Shell Script](https://img.shields.io/badge/shell_script-%23121011.svg?style=flat&logo=gnu-bash&logoColor=white)
![Android](https://img.shields.io/badge/Android-3DDC84?logo=android&logoColor=white)
![Homebrew](https://img.shields.io/badge/Homebrew-supported-orange.svg?logo=homebrew)

# ADB Extensions Kit (ak)

**Essential ADB utilities for Android development**

A unified CLI tool that simplifies Android Debug Bridge (ADB) operations including APK management, device control, and app inspection.

**Languages:** [🇺🇸 English](README.md) | [🇰🇷 한국어](README.ko.md)

## Features

- **Unified CLI** - Single command for all ADB operations
- **Multi-device Support** - Install APKs to multiple devices simultaneously
- **Interactive UI** - Intuitive selection interface with keyboard navigation
- **Auto Recovery** - Automatic error handling and recovery attempts
- **Tab Completion** - Zsh completion for commands and options
- **Rich Output** - Color-coded, structured information display

## Installation

### Homebrew (Recommended)

```bash
brew tap luminousvault/adb-extensions
brew trust luminousvault/adb-extensions
brew install ak
```

> **Note:** Since Homebrew 6.0, third-party taps must be explicitly trusted. If you see `Error: Refusing to load formula ... from untrusted tap`, run `brew trust luminousvault/adb-extensions` and try again.

### From Source

```bash
# Clone repository
git clone https://github.com/luminousvault/homebrew-adb-extensions.git
cd homebrew-adb-extensions

# Build and install
./build.sh
sudo ./build.sh --install
```

## Quick Start

```bash
# Install APK (interactive selection)
ak install

# Install specific APK
ak install app.apk

# Get app information
ak info com.example.app

# Launch app
ak launch com.example.app

# View connected devices
ak devices
```

## Usage

### Basic Syntax

```bash
ak <command> [options] [arguments...]
```

**Note:** Many commands support auto-detection of the foreground app when no package is specified. See [Examples](#examples) for detailed usage scenarios.

### Available Commands

#### APK Management

- `install [apk_files|directories...]` - Install APK files with interactive selection
  - APK Selection:
    - `-l` - Install latest APK file (from current directory or specified directory)
    - `-a` - Install all APK files (from current directory or specified directory)
    - `-f <filter>` - Filter APKs by pattern
  - Device Options:
    - `-m` - Install to all connected devices
  - ADB Options:
    - `-r` - Replace existing app (default)
    - `-t` - Allow test APKs
    - `-d` - Allow version downgrade
- `extract [package|filename] [filename|package]` - Extract APK from device (order flexible)

#### File Management

- `pull [directory]` - Pull recent device files (MediaStore 'Recents', interactive multi-select)
  - No argument: pull into the current directory
  - Directory argument: pull into it (created if needed)
  - Files are saved with their original names (duplicates get a ' (N)' suffix)

#### App Information

**Note:** Omitting `[package]` auto-detects the foreground app.

- `info [package]` - Display app information (version, SDK, debuggable status, installer)
- `permissions [package]` - List granted app permissions
- `signature [package|apk_file]` - Display app signature (supports interactive selection)
- `activities [--all]` - Display activity stack (`--all` for all tasks)

#### App Control

**Note:** Omitting `[package]` or `[packages...]` auto-detects the foreground app.

- `launch <package>` - Launch app (main activity)
- `kill [packages...]` - Force stop app(s)
- `clear [packages...]` - Clear app data
- `uninstall [package]` - Uninstall app

#### Device Management

- `devices` - List connected devices (brand, model, ID, Android version, CPU)

### Interactive UI Features

#### APK Selection

- **Arrow keys** (Up/Down) - Navigate through APKs
- **Space** - Toggle selection
- **A** - Select/deselect all
- **Number keys** (1-9) - Quick select (single item, 9 or fewer APKs)
- **Enter** - Confirm selection
- **Ctrl+C** - Cancel
- When selecting from multiple directories, each item shows its source folder name (e.g. `(directory)`) in dim color

#### Device Selection

- **Arrow keys** (Up/Down) - Navigate through devices
- **Number keys** (1-9) - Quick select (9 or fewer devices)
- **Enter** - Confirm selection
- **Ctrl+C** - Cancel

### Global Options

```bash
ak --version, -v                # Show version information
ak --help, -h                   # Show help message
ak <command> --help             # Show command-specific help
```

## Examples

### APK Installation

**Interactive selection from current directory:**
```bash
ak install
```

**Install specific APK:**
```bash
ak install app.apk
```

**Install latest APK:**
```bash
ak install -l
```

**Install latest APK from specific directory:**
```bash
ak install -l /path/to/dir
ak install /path/to/dir -l  # Same as above
```

**Install latest debug APK:**
```bash
ak install -l -f debug
```

**Install all APKs:**
```bash
ak install -a
```

**Install all APKs from specific directory:**
```bash
ak install -a /path/to/dir
ak install /path/to/dir -a  # Same as above
```

**Filter:**
```bash
ak install -f debug              # Current directory
ak install -f debug /path/to/dir # Specific directory
```

**Install to all connected devices:**
```bash
ak install -m app.apk
```

**Interactive selection from directory:**
```bash
ak install /path/to/dir
```

**Interactive selection from multiple directories:**
```bash
ak install /path/to/dir1 /path/to/dir2
```

### Pulling Recent Files

**Pick recent device files and pull into the current directory:**
```bash
ak pull
```

**Pull into a directory (created if it doesn't exist):**
```bash
ak pull /path/to/folder
```

### APK Extraction

**Extract foreground app:**
```bash
ak extract
```

**Extract foreground app with custom filename:**
```bash
ak extract myapp.apk
```

**Extract specific package:**
```bash
ak extract com.example.app
```

**Extract with package and filename (order flexible):**
```bash
ak extract com.example.app my.apk
ak extract my.apk com.example.app  # Same as above
```

### App Information

**Display app information (auto-detects foreground app):**
```bash
ak info
ak info com.example.app
```

**List app permissions:**
```bash
ak permissions
ak permissions com.example.app
```

**Check app signature (interactive selection):**
```bash
ak signature                   # Interactive: foreground apps + APK files
ak signature com.example.app   # Installed app
ak signature app.apk           # Local APK file
```

**View activity stack:**
```bash
ak activities                   # Foreground task
ak activities --all             # All tasks
```

### App Control

**Launch app:**
```bash
ak launch com.example.app
```

**Kill app(s):**
```bash
ak kill                         # Foreground app
ak kill com.app1 com.app2      # Multiple apps
```

**Clear app data:**
```bash
ak clear                        # Foreground app
ak clear com.app1 com.app2     # Multiple apps
```

**Uninstall app:**
```bash
ak uninstall                    # Foreground app (auto-detected)
ak uninstall com.example.app
```

### Device Management

**List connected devices:**
```bash
ak devices
```

### Workflow Examples

**Extract and check signature:**
```bash
ak extract com.example.app
ak signature com.example.app.apk
```

**Install, launch, and view info:**
```bash
ak install app.apk
ak launch com.example.app
ak info com.example.app
```

## Version History

See [CHANGELOG.md](CHANGELOG.md) for detailed version history.

## License

MIT License - See [LICENSE.md](LICENSE.md) for details.

## Author

Claude Hwang

## Contributing

Contributions are welcome! We appreciate bug fixes, new features, documentation improvements, and more.

Please see [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines on:

- Development setup
- Project structure
- Build system
- Adding new commands
- Code style guidelines
- Pull request process

Quick start for contributors:

```bash
# Fork and clone
git clone https://github.com/YOUR_USERNAME/homebrew-adb-extensions.git
cd homebrew-adb-extensions

# Test changes directly
./src/ak <command>

# Build and test
./build.sh
./build/ak <command>
```

## Acknowledgments

- Built with Bash for maximum compatibility
- Inspired by the need for efficient Android development workflows
- Special thanks to the Android development community

## Support

- **Issues**: [GitHub Issues](https://github.com/luminousvault/homebrew-adb-extensions/issues)
- **Discussions**: [GitHub Discussions](https://github.com/luminousvault/homebrew-adb-extensions/discussions)
