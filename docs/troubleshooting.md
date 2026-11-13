# Troubleshooting

Start with `ak doctor`. Most problems below show up there with a hint.

## Start here: `ak doctor`

```bash
ak doctor        # summary: only what needs attention
ak doctor -v     # every check, plus an Environment block
```

Each category gets one mark:

| Mark | Meaning | Exit code |
|---|---|---|
| `[✓]` | Fine | 0 |
| `[!]` | Worth fixing, but ak works (no device connected, tab completion missing or old) | 0 |
| `[✗]` | A problem (adb not found, adb versions differ, unauthorized or offline device, `ANDROID_SERIAL` not connected) | 1 |

Under a `[!]` or `[✗]` line, `Hint:` tells you what to run.

When you open an issue, paste the **Environment** block from `ak doctor -v`. It lists the ak, OS, shell and adb versions and the device models, without serial numbers.

## macOS: "ak cannot be opened because the developer cannot be verified"

Newer macOS versions say "Apple could not verify 'ak' is free of malware" instead.

**Cause:** you downloaded the release archive in a browser, so macOS marked the file as quarantined. ak is not signed with an Apple Developer ID or notarized; the Apple Silicon part of the binary only carries the ad-hoc signature the Go linker adds.

**Fix:** remove the quarantine flag from the extracted binary:

```bash
xattr -d com.apple.quarantine ./ak
```

Homebrew installs (`brew install ak`) download the same binary but are not quarantined, so they do not need this. Files downloaded with `curl` are not quarantined either.

## `adb not found`

ak looks for adb in this order and uses the first one it finds:

1. `AK_ADB` (path to the adb binary)
2. `$ANDROID_HOME/platform-tools/adb`
3. `$ANDROID_SDK_ROOT/platform-tools/adb`
4. `PATH`
5. Android Studio's default SDK: `~/Library/Android/sdk` (macOS), `~/Android/Sdk` (Linux), `%LOCALAPPDATA%\Android\Sdk` (Windows)
6. `~/.ak/platform-tools/adb`

**Fix:** install platform-tools, or point ak at an adb you already have:

```bash
brew install --cask android-platform-tools   # macOS
sudo apt install adb                         # Ubuntu
export AK_ADB=/path/to/adb                   # any existing adb
```

Platform-tools can also be downloaded from <https://developer.android.com/tools/releases/platform-tools>. `ak doctor -v` shows which adb ak uses and where it was found.

## Devices disconnect while Android Studio is open

**Cause:** Android Studio (or another tool) runs a different adb version than ak. When two adb versions talk to the adb server, each one restarts the server, and every device connection drops. `ak doctor` reports this as `[✗] adb … Other adb with a different version`.

**Fix:** make ak use the same adb as Android Studio:

```bash
export AK_ADB="$HOME/Library/Android/sdk/platform-tools/adb"   # macOS default SDK
```

Put the line in your shell profile so it stays. On Windows, use `set AK_ADB=…` (or the System environment settings).

## Device shows as `unauthorized`

**Cause:** the device has not accepted this computer's USB debugging key.

**Fix:** unlock the device and accept the "Allow USB debugging?" prompt. If no prompt appears, unplug and reconnect the cable, or revoke USB debugging authorizations in Developer options and reconnect.

## Device shows as `offline`

**Cause:** the adb connection broke (cable, USB hub, sleep, or an adb server restart).

**Fix:** reconnect the cable. If that does not help, restart the adb server:

```bash
adb kill-server
ak devices
```

## "Multiple devices connected" in scripts

```
Error: Multiple devices connected: emulator-5554, emulator-5556
Hint: Use -s <serial> or set ANDROID_SERIAL
```

**Cause:** several devices are connected and ak is not running in a terminal (a pipe, a script, CI), so it cannot show the device picker.

**Fix:** choose the device:

```bash
ak info -s emulator-5554
export ANDROID_SERIAL=emulator-5554      # default device, as with adb
ak install -a app.apk                    # install, mac, screenshot: every device
```

`ak devices` lists the serials. `install`, `mac` and `screenshot` accept `-s` more than once.

## Install takes long or shows "(check the device screen)"

**Cause:** when an install step takes more than 10 seconds, ak adds `(check the device screen)` because the device is usually waiting for you:

- Google Play Protect asks whether to scan an app it has not seen (often after an uninstall and reinstall).
- Some devices ask whether to allow installing over USB.

**Fix:** answer on the device and the install continues.

On test devices you can stop Play Protect from checking apps installed with adb. **This is a device security setting**, so ak never changes it for you. It stays after a reboot:

```bash
adb shell settings put global verifier_verify_adb_installs 0            # stop scanning adb installs
adb shell settings delete global verifier_verify_adb_installs           # back to the default (scans on)
adb -s <serial> shell settings put global verifier_verify_adb_installs 0   # with several devices
```

This setting is not confirmed to suppress every Play Protect prompt on every device or Play services version.

## Install asks to uninstall the app first

```
Uninstall 'com.example.app' from Google Pixel 9 Pro (removes its data) and reinstall? [Y/n]
```

**Cause:** ak already retried automatically (`-t` for test-only APKs, `-d` for downgrades), but the install still cannot proceed:

- The APK is older than the installed version and the device does not allow downgrading it.
- The APK is signed with a different key than the installed app (`INSTALL_FAILED_UPDATE_INCOMPATIBLE`).

The only way forward is to uninstall the app, which **removes its data**.

**Answers:**

- `Y` or Enter: uninstall and reinstall.
- `n`: skip this APK.
- `-y` answers yes without asking (`ak install -y app.apk`).

With several devices the question is `[Y/n/a]`: `a` answers yes for every device for the rest of this run, including questions already waiting. Answering `n` skips that APK and stops the remaining APKs on that device, since they may depend on it.

Without a terminal and without `-y`, ak does not guess: it skips the APK with `Skipped: uninstalling needs confirmation (pass -y)` and exits 1.

## Tab completion is missing or out of date

Run `ak doctor` and look at the **Shell completion** line. It finds the completion your shell loads and compares it with the installed ak.

Homebrew installs completion for bash, zsh and fish. Otherwise add one line for your shell:

| Shell | Setup |
|---|---|
| zsh | `source <(ak completion zsh)` in `~/.zshrc`, after `compinit` |
| bash | `eval "$(ak completion bash)"` in `~/.bashrc` (macOS: `~/.bash_profile`) |
| fish | `ak completion fish > ~/.config/fish/completions/ak.fish` |
| PowerShell | `ak completion powershell \| Out-String \| Invoke-Expression` in `$PROFILE` |

- **bash on macOS:** the system bash 3.2 cannot `source <(...)`, so use the `eval` line. Terminal opens login shells, which read `~/.bash_profile`.
- **Still old after updating ak:** an open zsh keeps the completion it loaded. Open a new terminal or run `exec zsh` (`exec bash`, `exec fish`).
- **Old file found:** `ak doctor` names the file and the command to regenerate it. For a Homebrew file, run `brew reinstall ak`.

## Screenshot copy (`-c`) fails or shows up somewhere unexpected

- **Linux:** ak needs `wl-clipboard` (Wayland, used when `WAYLAND_DISPLAY` is set) or `xclip`. Without one: `no clipboard tool found (install wl-clipboard or xclip)`.

  ```bash
  sudo apt install wl-clipboard   # Wayland
  sudo apt install xclip          # X11
  ```

- **macOS:** the image goes to the system clipboard as PNG and TIFF, so clipboard history apps (Raycast, Alfred, Paste and the like) record it like any other copied image.
- **Several devices:** the clipboard holds one image, so `-c` needs one device (`-s <serial>`).
- `-c` alone does not save a file. Add a path or `-o` to save it too.

## Debug log

```bash
AK_DEBUG=1 ak install app.apk
tail -f "${TMPDIR:-/tmp}/ak-debug.log"
```

The log records every adb call and picker result as `[time] [file:line] message`. Each run with `AK_DEBUG=1` starts a new log.

- Default location: the system temp directory (`$TMPDIR` on macOS and Linux, falling back to `/tmp`; `%TEMP%` on Windows), file `ak-debug.log`.
- `AK_DEBUG_LOG=/path/to/file.log` writes it elsewhere.

Check the log before attaching it to an issue: it can contain package names, file paths and device serials.
