# zsh Tab completion: commands, APK files, device serials.
Output demo/completion.gif
Source demo/settings.tape
Set Shell "zsh"
Set Height 260

Hide
Type "source demo/setup.sh" Enter
Sleep 3s
Show

Type "ak scr" Sleep 500ms Tab
Sleep 1.5s
Ctrl+U Sleep 300ms
Type "ak install " Sleep 500ms Tab
Sleep 2.5s
Ctrl+U Sleep 300ms
Type "ak screenshot -s " Sleep 500ms Tab
Sleep 800ms Tab
Sleep 2.5s
