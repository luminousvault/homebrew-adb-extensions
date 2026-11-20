# ak push: copy two files to both devices; one name is taken, so it gets ' (1)'.
Output demo/push.gif
Source demo/settings.tape
Set Width 1300
Set Height 300

Hide
Type "source demo/setup.sh" Enter
Sleep 1s
Show

Type "ak push app-release.apk release-notes.pdf -a" Sleep 400ms Enter
# The pushes take ~2.5 s; hold the result before the GIF loops.
Sleep 7s
