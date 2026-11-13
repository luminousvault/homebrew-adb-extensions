# Sourced (hidden) at the start of every tape, from the repository root.
# Builds a throwaway home so nothing from this computer shows up on screen:
#
#   demo/home/                                    $HOME during the demo
#     .zshrc                                      loads 'ak completion zsh' (for doctor)
#     Library/Android/sdk/platform-tools/adb  ->  demo/fake-adb
#     my-app/*.apk                                sample APKs (copies of a tiny test APK)
#     _ak                                         zsh completion (completion.tape only)

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
