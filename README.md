[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Version](https://img.shields.io/github/v/release/luminousvault/homebrew-adb-extensions?color=blue)](https://github.com/luminousvault/homebrew-adb-extensions/releases/latest)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey.svg)
![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)
![Homebrew](https://img.shields.io/badge/Homebrew-supported-orange.svg?logo=homebrew)

# ak — ADB Extensions Kit

**Languages:** English | [한국어](README.ko.md)

**The Android tasks you would otherwise script with adb, as one command each.**

Install APKs on every connected device at once, see an app's info without typing its package name, copy a screenshot to the clipboard, follow one app's logs across restarts. ak is a single binary for macOS, Linux and Windows; the only thing it needs is adb.

![Installing two APKs on two devices with ak](demo/hero.gif)

## Why ak

| You want to | With adb | With ak |
|---|---|---|
| Install the newest build on every device | find the APK, `adb -s … install -r` per device, retry with `-t` / `-d`, uninstall on a signature mismatch | `ak install -la` |
| See the version of the app on screen | `dumpsys activity` to find the package, then `dumpsys package` and grep | `ak info` |
| Share a screenshot | `screencap`, `pull`, `rm` on the device, open it | `ak screenshot -c` (straight to the clipboard) |
| Read one app's logs | `pidof`, `logcat --pid`, start over when the app restarts | `ak logcat` |
| Debug over Wi-Fi | `ip addr`, `tcpip 5555`, `connect` | `ak wifi` |

When you leave out the package name, ak uses the app in the foreground.

## Install

### Homebrew (macOS, Linux)

```bash
brew tap luminousvault/adb-extensions
brew trust luminousvault/adb-extensions
brew install ak
```

Homebrew 6.0+ refuses formulae from taps you have not trusted, hence `brew trust`. Tab completion for zsh, bash and fish is installed too; open a new terminal to use it.

### Prebuilt binaries

Download the archive for your machine from [Releases](https://github.com/luminousvault/homebrew-adb-extensions/releases/latest) and put `ak` on your `PATH`:

| Machine | Archive |
|---|---|
| Mac (Apple Silicon or Intel) | `ak_<version>_macos.tar.gz` |
| Windows | `ak_<version>_windows_x64.zip`, ARM: `…_windows_arm64.zip` |
| Linux | `ak_<version>_linux_x86_64.tar.gz`, ARM: `…_linux_arm64.tar.gz` (as `uname -m` shows) |

> [!NOTE]
> **macOS blocks a binary downloaded with a browser** ("cannot be opened because the developer cannot be verified"). Remove the quarantine flag once:
>
> ```bash
> xattr -d com.apple.quarantine ./ak
> ```
>
> Homebrew installs are not affected.

### From source (Go 1.27+)

```bash
git clone https://github.com/luminousvault/homebrew-adb-extensions.git
cd homebrew-adb-extensions
make install          # ak + zsh completion into /usr/local; make uninstall removes them
```

### adb

ak finds adb itself — `AK_ADB`, `ANDROID_HOME`, `ANDROID_SDK_ROOT`, `PATH`, then Android Studio's default SDK — so Android Studio users need nothing else. Without any adb: `brew install --cask android-platform-tools`. Run `ak doctor` to check the setup.

## Quick start

```bash
ak devices                 # connected devices
ak install                 # pick APKs in the current folder, then devices
ak install app.apk -a      # install on every connected device at once
ak info                    # the app on screen: version, SDK levels, installer
ak screenshot -co          # save, copy to the clipboard and open
ak logcat                  # logs of the app on screen, following restarts
ak doctor                  # is everything set up?
```

## What it does

### Install on many devices at once

Devices install in parallel while a status board shows each one. Common failures are handled for you: test-only APKs are retried with `-t`, downgrades with `-d`, and when a device needs the app uninstalled first (downgrade not allowed, different signature), ak asks right away while the other devices keep going.

![A signature conflict is asked about while the other device keeps installing](demo/confirm.gif)

- `-l` newest APK, `-e` every APK, `-f <words>` filter — or pick in the list
- `-a` every device, `-s <serial>` (repeatable) — or pick in the list
- `-y` answers yes in advance; `a` at the question answers yes for every device in this run

### Find out what is wrong

`ak doctor` checks the adb in use and any other adb with a different version, connected devices (unauthorized, offline), shell completion and optional tools. `ak doctor -v` adds every detail and an environment block to paste into an issue.

![ak doctor](demo/doctor.gif)

### Read your app's logs, not the framework's

`ak logcat` follows one or more apps across restarts and colors lines like Android Studio's Logcat. What Android's framework logs from inside your app (views, input method, graphics) is hidden unless it is an error; `-e` shows everything, `-t` / `-i` pick or hide tags.

![ak logcat merging two apps](demo/logcat.gif)

### Screen and more

![ak screenshot](demo/screenshot.gif)

- `ak screenshot` saves a PNG on this computer; `-c` copies it to the clipboard, `-o` opens it
- `ak record` records until Ctrl+C (up to 180 s) and pulls the MP4
- `ak restart -r 5` restarts the app five times and reports launch times
- `ak wifi` switches a USB device to Wi-Fi debugging; `ak wifi off` switches back

### Tab completion

Commands, options, device serials — and for arguments only what they take: APK files and folders for `install`, folders for `screenshot`.

![Tab completion](demo/completion.gif)

| Shell | Setup (Homebrew already did zsh, bash and fish) |
|---|---|
| zsh | `source <(ak completion zsh)` in `~/.zshrc` |
| bash | `eval "$(ak completion bash)"` in `~/.bashrc` |
| fish | `ak completion fish > ~/.config/fish/completions/ak.fish` |
| PowerShell | `ak completion powershell \| Out-String \| Invoke-Expression` in `$PROFILE` |

## Commands

| | Command | What it does |
|---|---|---|
| APK | [`install`](docs/commands.md#ak-install) | Install APKs on one or more devices, with automatic recovery |
| | [`extract`](docs/commands.md#ak-extract) | Save an installed app's APK |
| | [`signature`](docs/commands.md#ak-signature) | Signing certificate hashes of an app or APK file |
| App | [`info`](docs/commands.md#ak-info) · [`permissions`](docs/commands.md#ak-permissions) | Version, SDK levels, installer · granted permissions |
| | [`launch`](docs/commands.md#ak-launch) · [`kill`](docs/commands.md#ak-kill) · [`restart`](docs/commands.md#ak-restart) | Start · force-stop · restart and time the launch |
| | [`clear`](docs/commands.md#ak-clear) · [`uninstall`](docs/commands.md#ak-uninstall) | Clear data · uninstall |
| | [`activities`](docs/commands.md#ak-activities) · [`logcat`](docs/commands.md#ak-logcat) | Activity stack · logs of one or more apps |
| Screen | [`screenshot`](docs/commands.md#ak-screenshot) · [`record`](docs/commands.md#ak-record) | PNG to a file or the clipboard · MP4 recording |
| Files | [`pull`](docs/commands.md#ak-pull) | Pick recent files on the device and copy them here |
| Device | [`devices`](docs/commands.md#ak-devices) · [`mac`](docs/commands.md#ak-mac) · [`wifi`](docs/commands.md#ak-wifi) | Device list · MAC addresses · Wi-Fi debugging |
| Setup | [`doctor`](docs/commands.md#ak-doctor) · [`completion`](docs/commands.md#ak-completion) | Check the setup · shell completion script |

Every option and example is in **[docs/commands.md](docs/commands.md)**; `ak <command> --help` shows the same.

## Scripts and CI

- Choose devices with `-s <serial>`, `ANDROID_SERIAL`, or `-a` (all). Without a terminal ak never opens a picker; it asks for `-s` instead
- Results go to stdout, messages to stderr; `--json` on `devices`, `info`, `permissions`, `signature`, `mac`, `activities`, `restart` and `doctor`
- Exit codes: `0` success, `1` failure (also partial), `2` usage error, `130` interrupted
- Short options combine (`-ea` is `-e -a`) and may come before or after arguments; colors only on terminals (`NO_COLOR`, `--no-color`)

## Troubleshooting

Start with `ak doctor`. Common cases — macOS blocking a downloaded binary, devices dropping while Android Studio is open, installs waiting on a Play Protect prompt, old tab completion — are in **[docs/troubleshooting.md](docs/troubleshooting.md)**.

## Project

A personal project: pull requests are not accepted, but [issues](https://github.com/luminousvault/homebrew-adb-extensions/issues) are welcome — please paste the Environment block from `ak doctor -v`. Changes are listed in the [CHANGELOG](CHANGELOG.md). MIT licensed.
