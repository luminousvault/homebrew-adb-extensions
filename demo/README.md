# Demo GIFs

The GIFs in the main README are recorded with [vhs](https://github.com/charmbracelet/vhs)
against a fake adb, so no real device (and no real serial number) is needed or shown.

## Re-record

From the repository root:

```bash
go build -ldflags "-X main.version=2.x" -o demo/bin/ak ./cmd/ak
vhs demo/hero.tape        # or confirm, doctor, screenshot, logcat, completion
```

The demo binary is built as version `2.x`, not the release number, so `doctor.gif` (the
only GIF that shows a version) stays right across releases. Re-record it when the doctor
screen changes or the major version does. Without `-ldflags` the version shows as `dev`.

| Tape | GIF | Shows |
|------|-----|-------|
| `hero.tape` | `hero.gif` | `ak install`: pick 2 APKs, pick 2 devices, parallel status board, summary |
| `confirm.tape` | `confirm.gif` | A signature conflict on one device asks to uninstall and reinstall |
| `doctor.tape` | `doctor.gif` | `ak doctor` |
| `screenshot.tape` | `screenshot.gif` | `ak screenshot` and `ak screenshot -c` (writes your real clipboard) |
| `logcat.tape` | `logcat.gif` | `ak logcat` with two apps: colors, app bars, a wrapped line, framework lines hidden |
| `completion.tape` | `completion.gif` | zsh Tab completion of commands, APK files and serials |

`settings.tape` holds the shared look (font, size, theme). Every tape first runs
`setup.sh` while hidden: it makes a throwaway `$HOME` at `demo/home` with sample APKs
in `~/my-app` (copies of `internal/apk/testdata/aktest.apk`) and points ak at the fake
adb, so nothing from your own machine appears on screen.

GIFs loop straight back to the first frame, so every tape ends with a `Sleep` that keeps
the final screen up for 3–5 s after the last output appears, and leaves about 1 s
between steps. If a command gets slower, lengthen that last `Sleep` too.

## fake-adb

`fake-adb` is a small bash script that answers the adb calls the demos make:
`version`, `devices`, `shell getprop`, `install` (with a delay per device, so the status
board animates), `uninstall`, `exec-out screencap -p` (returns `assets/screen.png`),
`shell ps -A -o PID,NAME` and `logcat -v threadtime` (a scripted log: `com.example.app`
is running, `com.example.shop` starts partway through, and a few framework lines that
ak hides; it then waits like the real logcat).
Other `shell` commands succeed with no output; anything else fails.

| Serial | Device |
|--------|--------|
| `emulator-5554` | Google Pixel 9, Android 16 (API 36), installs take 2 s |
| `emulator-5556` | Samsung Galaxy S25, Android 15 (API 35), installs take 3.5 s |

Environment variables:

- `AK_DEMO_DEVICES=1`: only `emulator-5554` is connected.
- `AK_DEMO_SCENARIO=conflict`: the Galaxy fails the first install with
  `INSTALL_FAILED_UPDATE_INCOMPATIBLE`; after `uninstall` it succeeds.
- `AK_DEMO_STATE`: where the scenario keeps state between calls
  (default `$TMPDIR/ak-demo`; `setup.sh` clears it).

You can also try it by hand:

```bash
source demo/setup.sh    # changes HOME, PATH and the current directory of this shell
ak devices
```
