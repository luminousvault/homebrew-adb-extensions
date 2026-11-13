# The Galaxy needs a confirmation; the Pixel keeps installing meanwhile.
Output demo/confirm.gif
Source demo/settings.tape
Set Height 540
Env AK_DEMO_SCENARIO "conflict"

Hide
Type "source demo/setup.sh" Enter
Sleep 1s
Show

Type "ak install app-debug.apk app-release.apk --all" Sleep 400ms Enter
Sleep 4.5s
Type "y"
Sleep 10s
