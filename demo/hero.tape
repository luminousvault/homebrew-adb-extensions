# ak install: pick APKs, pick devices, install on both at once.
Output demo/hero.gif
Source demo/settings.tape
Set Height 420

Hide
Type "source demo/setup.sh" Enter
Sleep 1s
Show

Type "ak install" Sleep 400ms Enter
Sleep 1.2s
# APK picker: app-debug.apk (top), then app-release.apk
Space Sleep 400ms Down Sleep 300ms Space Sleep 600ms Enter
Sleep 1s
# Device picker: both devices
Space Sleep 400ms Down Sleep 300ms Space Sleep 600ms Enter
Sleep 9s
