# ak screenshot: save a PNG, or copy it to the clipboard with -c.
Output demo/screenshot.gif
Source demo/settings.tape
Set Height 260
Env AK_DEMO_DEVICES "1"

Hide
Type "source demo/setup.sh" Enter
Sleep 1s
Show

Type "ak screenshot" Sleep 400ms Enter
Sleep 3s
Type "ak screenshot -c" Sleep 400ms Enter
# Hold the result before the GIF loops.
Sleep 4.5s
