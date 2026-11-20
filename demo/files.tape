# ak files: pull photos from the device, then paste a file from the computer into a device folder.
Output demo/files.gif
Source demo/settings.tape
Env AK_DEMO_DEVICES "1"

Hide
Type "source demo/setup.sh && cd ~/Downloads && clear" Enter
Sleep 1s
Show

Type "ak files" Sleep 400ms Enter
Sleep 1.5s
# Device side: /sdcard → DCIM → Camera
Down Sleep 250ms Down Sleep 250ms Down Sleep 500ms Enter
Sleep 800ms
Down Sleep 500ms Enter
Sleep 1s
# Select five photos and pull them to the computer side (~3.5 s)
Down Sleep 250ms Down Sleep 500ms
Space Sleep 350ms Space Sleep 350ms Space Sleep 350ms Space Sleep 350ms Space Sleep 800ms
Type "5"
Sleep 4.5s
# Computer side: copy design-review.pdf into the clipboard
Tab Sleep 800ms
Down Sleep 250ms Down Sleep 600ms
Type "3" Sleep 1s
# Back to the device: /sdcard/Documents, paste
Tab Sleep 600ms
Backspace Sleep 500ms Backspace Sleep 700ms
Down Sleep 500ms Enter Sleep 800ms
Type "6"
Sleep 2s
# Quit: the summary of what was copied
Type "q"
Sleep 4.5s
