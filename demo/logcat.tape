# ak logcat: two apps merged, colored like Android Studio, framework logs hidden.
Output demo/logcat.gif
Source demo/settings.tape
Set Height 620
Env AK_DEMO_DEVICES "1"

Hide
Type "source demo/setup.sh" Enter
Sleep 1s
Show

Type "ak logcat com.example.app com.example.shop" Sleep 400ms Enter
# fake-adb streams the log for about 5.5 s.
Sleep 6.5s
# Hold the result before the GIF loops.
Sleep 4.5s

Hide
Ctrl+C
Sleep 500ms
