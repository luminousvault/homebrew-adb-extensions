# Sourced (hidden) at the start of every tape, from the repository root.
# Builds a throwaway home so nothing from this computer shows up on screen:
#
#   demo/home/                                    $HOME during the demo
#     .zshrc                                      loads 'ak completion zsh' (for doctor)
#     Library/Android/sdk/platform-tools/adb  ->  demo/fake-adb
#     my-app/*.apk                                sample APKs (copies of a tiny test APK)
#     _ak                                         zsh completion (completion.tape only)
#     Downloads/                                  computer side of files.tape
#     devices/<serial>/storage/emulated/0/...     each device's storage (fake-adb)
#     devices/<serial>/sdcard  ->  storage/emulated/0

demo=$(pwd)/demo
export HOME="$demo/home"
rm -rf "$HOME" "${TMPDIR:-/tmp}/ak-demo"
mkdir -p "$HOME/Library/Android/sdk/platform-tools" "$HOME/my-app/build"
ln -s "$demo/fake-adb" "$HOME/Library/Android/sdk/platform-tools/adb"
echo 'source <(ak completion zsh)' >"$HOME/.zshrc"

# Oldest first: the APK picker lists the newest on top.
apk=$demo/../internal/apk/testdata/aktest.apk
minute=0
for name in feature-login-debug app-release app-debug; do
  cp "$apk" "$HOME/my-app/$name.apk"
  touch -t "$(date +%Y%m%d)08$((minute += 10))" "$HOME/my-app/$name.apk"
done
touch "$HOME/my-app/build.gradle.kts" "$HOME/my-app/README.md" # not offered by 'ak install <Tab>'

# days[n]: the date n days ago (YYYYMMDD). Worked out once: setup must stay
# well under the tapes' hidden 1 s.
days=()
for n in 0 1 2 3 4 5 6 9 12; do days[n]=$(date -v-"$n"d +%Y%m%d); done

# demo_file <path> <KB> <days ago> <hhmm>: a file of that size and age, all
# zeros and sparse, so it takes no time or disk (its folder must exist).
demo_file() {
  dd if=/dev/null of="$1" bs=1024 seek="$2" count=0 2>/dev/null
  touch -t "${days[$3]}$4" "$1"
}

# Each device's storage (fake-adb runs shell commands against it).
for serial in emulator-5554 emulator-5556; do
  root=$HOME/devices/$serial
  s=$root/storage/emulated/0
  mkdir -p "$root/data/local/tmp" "$s/Alarms" "$s/Android/data" "$s/DCIM/Camera" "$s/DCIM/Screenshots" \
    "$s/Documents" "$s/Download" "$s/Movies" "$s/Music" "$s/Pictures"
  ln -s storage/emulated/0 "$root/sdcard"
  case $serial in
  emulator-5554) cam=PXL_ ;;
  *) cam= ;;
  esac
  demo_file "$s/DCIM/Camera/${cam}${days[9]}_101533.jpg" 2900 9 1015
  demo_file "$s/DCIM/Camera/${cam}${days[3]}_181455.jpg" 3100 3 1814
  demo_file "$s/DCIM/Camera/${cam}${days[3]}_181502.jpg" 4400 3 1815
  demo_file "$s/DCIM/Camera/${cam}${days[2]}_090044.jpg" 3600 2 0900
  demo_file "$s/DCIM/Camera/${cam}${days[2]}_090102.jpg" 4800 2 0901
  demo_file "$s/DCIM/Camera/${cam}${days[0]}_083012.jpg" 4100 0 0830
  demo_file "$s/DCIM/Screenshots/Screenshot_${days[1]}-142210.png" 330 1 1422
  demo_file "$s/Download/app-release.apk" 8350 5 1130
  demo_file "$s/Download/app-debug.apk" 8420 1 1702
  demo_file "$s/Download/boarding-pass.pdf" 230 6 0712
  demo_file "$s/Download/manual.pdf" 1430 12 1630
  demo_file "$s/Documents/notes.txt" 3 4 2110
  demo_file "$s/Movies/${cam}${days[3]}_181530.mp4" 9800 3 1816
  # Folders: the time of their newest file, the others some months back.
  for d in Alarms Android Android/data Music Pictures; do touch -t 202607010900 "$s/$d"; done
  touch -t "${days[0]}0830" "$s/DCIM/Camera" "$s/DCIM"
  touch -t "${days[1]}1422" "$s/DCIM/Screenshots"
  touch -t "${days[1]}1702" "$s/Download"
  touch -t "${days[4]}2110" "$s/Documents"
  touch -t "${days[3]}1816" "$s/Movies"
done

demo_file "$HOME/my-app/release-notes.pdf" 250 0 0805 # push.tape

# The computer side of files.tape.
mkdir -p "$HOME/Downloads"
demo_file "$HOME/Downloads/design-review.pdf" 1230 1 1540
demo_file "$HOME/Downloads/test-data.json" 12 2 1105
demo_file "$HOME/Downloads/app-release.apk" 8350 0 0815

unset ANDROID_HOME ANDROID_SDK_ROOT ANDROID_SERIAL ZDOTDIR
export AK_ADB="$HOME/Library/Android/sdk/platform-tools/adb"
export SHELL=/bin/zsh
export PATH="$demo/bin:/usr/bin:/bin:/usr/sbin:/sbin"
cd "$HOME/my-app" || return

if [ -n "${ZSH_VERSION:-}" ]; then
  # The completion demo: zsh -f with only ak's completion loaded.
  "$demo/bin/ak" completion zsh >"$demo/home/_ak"
  fpath=("$demo/home" $fpath)
  autoload -Uz compinit && compinit -u -D
  zstyle ':completion:*' menu select
  zstyle ':completion:*:descriptions' format '%F{8}%d%f'
  PROMPT='%F{141}$%f '
else
  PS1='\[\e[38;5;141m\]$\[\e[0m\] '
fi
clear
