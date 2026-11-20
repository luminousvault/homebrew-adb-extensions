---
name: ak
description: Use ak (ADB Extensions Kit) instead of raw adb for work on connected Android devices and emulators - installing APKs (it recovers from test-only, version-downgrade and signature conflicts), app version and SDK levels, permissions, launching, force-stopping, restarting with launch time, clearing data, uninstalling, extracting an installed APK, signing certificate hashes, the activity stack, an app's logcat, screenshots, screen recording, pulling recent files, pushing files to the device, listing devices, MAC addresses, Wi-Fi debugging and checking the adb setup. It works on several devices at once.
metadata:
  managed-by: ak
---

# ak

Before the first ak command in a task, run `ak --help`, then `ak <command> --help` for the command you need. The help comes from the installed ak, so trust it over memory.

- Name the package. Without one, app commands use the app on screen, and `clear` and `uninstall` do not ask first.
- You have no terminal, so ak opens no pickers and asks no questions. Choose the device with `-s <serial>` (`ak devices` lists them) or `-a` for all, and pass `-y` or the command's own options where it would ask.
- Use `--json` where a command offers it. Results go to stdout, messages to stderr.
- Exit codes: 0 success, 1 failure, 2 usage error, 130 interrupted. Follow the `Hint:` line under an error.
- Copy files with `ak pull` and `ak push`; `ak files` is a file manager for a person at a terminal.
- Use plain `adb` only for what ak has no command for. `ak doctor` checks the adb setup.

ak wrote this file and updates it when ak is upgraded. To keep your own edits, delete the `managed-by: ak` line. To remove it, delete this folder; ak will not add it again.
