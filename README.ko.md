[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Version](https://img.shields.io/github/v/release/luminousvault/homebrew-adb-extensions?color=blue)](https://github.com/luminousvault/homebrew-adb-extensions/releases/latest)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey.svg)
![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)
![Homebrew](https://img.shields.io/badge/Homebrew-supported-orange.svg?logo=homebrew)

# ak — ADB Extensions Kit

**adb로 여러 단계를 거치던 안드로이드 작업을 명령 하나씩으로.**

연결된 모든 디바이스에 APK를 한 번에 설치하고, 패키지 이름을 몰라도 지금 화면의 앱 정보를 보고, 스크린샷을 클립보드로 바로 복사하고, 앱이 재시작해도 그 앱의 로그만 따라갑니다. macOS · Linux · Windows용 단일 실행 파일이고, 필요한 것은 adb 하나뿐입니다.

![ak로 APK 두 개를 디바이스 두 대에 설치하는 화면](demo/hero.gif)

**언어:** [English](README.md) | 한국어

## 왜 ak인가

| 하려는 일 | adb로 | ak로 |
|---|---|---|
| 최신 빌드를 모든 디바이스에 설치 | APK를 찾고, 디바이스마다 `adb -s … install -r`, 실패하면 `-t` · `-d`로 재시도, 서명이 다르면 제거 후 재설치 | `ak install -la` |
| 지금 화면에 떠 있는 앱의 버전 보기 | `dumpsys activity`로 패키지를 찾고 `dumpsys package`에서 grep | `ak info` |
| 스크린샷 공유 | `screencap`, `pull`, 디바이스에서 `rm`, 열기 | `ak screenshot -c` (바로 클립보드로) |
| 앱 하나의 로그 보기 | `pidof`, `logcat --pid`, 앱이 재시작하면 처음부터 | `ak logcat` |
| Wi-Fi로 디버깅 | `ip addr`, `tcpip 5555`, `connect` | `ak wifi` |

패키지 이름을 생략하면 지금 화면에 떠 있는 앱을 씁니다.

## 설치

### Homebrew (macOS, Linux)

```bash
brew tap luminousvault/adb-extensions
brew trust luminousvault/adb-extensions
brew install ak
```

Homebrew 6.0부터는 신뢰하지 않은 tap의 formula를 거부해서 `brew trust`가 필요합니다. zsh · bash · fish 탭 자동 완성도 함께 설치됩니다. 터미널을 새로 열면 바로 쓸 수 있습니다.

### 빌드된 바이너리

[Releases](https://github.com/luminousvault/homebrew-adb-extensions/releases/latest)에서 기기에 맞는 압축 파일을 받아 `ak`를 `PATH`에 두세요.

| 기기 | 파일 |
|---|---|
| Mac (Apple Silicon · Intel 공통) | `ak_<버전>_macos.tar.gz` |
| Windows | `ak_<버전>_windows_x64.zip`, ARM: `…_windows_arm64.zip` |
| Linux | `ak_<버전>_linux_x86_64.tar.gz`, ARM: `…_linux_arm64.tar.gz` (`uname -m` 기준) |

> [!NOTE]
> **macOS는 브라우저로 받은 실행 파일을 막습니다**("개발자를 확인할 수 없기 때문에 열 수 없습니다"). 처음 한 번 격리 표시를 지우세요.
>
> ```bash
> xattr -d com.apple.quarantine ./ak
> ```
>
> Homebrew로 설치하면 필요 없습니다.

### 소스에서 (Go 1.27+)

```bash
git clone https://github.com/luminousvault/homebrew-adb-extensions.git
cd homebrew-adb-extensions
make install          # ak와 zsh 자동 완성을 /usr/local에 설치, make uninstall로 제거
```

### adb

ak가 adb를 직접 찾습니다(`AK_ADB` → `ANDROID_HOME` → `ANDROID_SDK_ROOT` → `PATH` → Android Studio 기본 SDK). 그래서 Android Studio 사용자는 따로 할 것이 없습니다. adb가 아예 없으면 `brew install --cask android-platform-tools`. 설정 상태는 `ak doctor`로 확인합니다.

## 빠른 시작

```bash
ak devices                 # 연결된 디바이스
ak install                 # 현재 폴더에서 APK를 고르고, 디바이스를 고름
ak install app.apk -a      # 연결된 모든 디바이스에 동시에 설치
ak info                    # 화면의 앱: 버전, SDK 레벨, 설치 출처
ak screenshot -co          # 저장하고 클립보드에 복사하고 열기
ak logcat                  # 화면의 앱 로그, 재시작해도 따라감
ak doctor                  # 설정이 제대로 됐는지 점검
```

## 할 수 있는 것

### 여러 디바이스에 동시에 설치

디바이스마다 동시에 설치하고, 상태판에 디바이스별 진행 상황이 보입니다. 흔한 실패는 알아서 처리합니다. 테스트 전용 APK는 `-t`, 다운그레이드는 `-d`로 다시 시도합니다. 먼저 제거해야 하는 경우(다운그레이드 불가, 서명 다름)에는 바로 묻고, 그동안 다른 디바이스는 계속 설치합니다.

![서명 충돌 질문이 뜨는 동안 다른 디바이스는 계속 설치하는 화면](demo/confirm.gif)

- `-l` 최신 APK, `-e` 모든 APK, `-f <단어>` 이름 필터 — 또는 목록에서 고르기
- `-a` 모든 디바이스, `-s <시리얼>`(여러 번 가능) — 또는 목록에서 고르기
- `-y`는 미리 "예", 질문에서 `a`는 이번 실행의 모든 디바이스에 "예"

### 무엇이 문제인지 찾기

`ak doctor`는 사용 중인 adb와 버전이 다른 adb, 연결된 디바이스(권한 없음 · 오프라인), 셸 자동 완성, 선택 도구를 점검합니다. `ak doctor -v`는 모든 상세와 함께, 이슈에 붙여 넣을 환경 정보 블록을 보여 줍니다.

![ak doctor](demo/doctor.gif)

### 화면, 로그, 그 밖에

![ak screenshot](demo/screenshot.gif)

- `ak screenshot`은 PNG를 이 컴퓨터에 저장합니다. `-c`는 클립보드에 복사, `-o`는 저장 후 열기
- `ak record`는 Ctrl+C까지(최대 180초) 녹화하고 MP4를 가져옵니다
- `ak logcat`은 앱 하나의 로그만 보여 주고, 앱이 재시작해도 이어서 따라갑니다
- `ak restart -r 5`는 앱을 다섯 번 재시작하며 실행 시간을 잽니다
- `ak wifi`는 USB 디바이스를 Wi-Fi 디버깅으로 바꾸고, `ak wifi off`로 되돌립니다

### 탭 자동 완성

명령, 옵션, 디바이스 시리얼을 완성하고, 인자는 받는 것만 보여 줍니다. `install`은 APK 파일과 폴더, `screenshot`은 폴더만 나옵니다.

![탭 자동 완성](demo/completion.gif)

| 셸 | 설정 (Homebrew는 zsh · bash · fish를 이미 설치함) |
|---|---|
| zsh | `~/.zshrc`에 `source <(ak completion zsh)` |
| bash | `~/.bashrc`에 `eval "$(ak completion bash)"` |
| fish | `ak completion fish > ~/.config/fish/completions/ak.fish` |
| PowerShell | `$PROFILE`에 `ak completion powershell \| Out-String \| Invoke-Expression` |

## 명령

| | 명령 | 하는 일 |
|---|---|---|
| APK | [`install`](docs/commands.md#ak-install) | 한 대 또는 여러 대에 APK 설치, 실패 자동 복구 |
| | [`extract`](docs/commands.md#ak-extract) | 설치된 앱의 APK 꺼내기 |
| | [`signature`](docs/commands.md#ak-signature) | 앱 또는 APK 파일의 서명 인증서 해시 |
| 앱 | [`info`](docs/commands.md#ak-info) · [`permissions`](docs/commands.md#ak-permissions) | 버전 · SDK 레벨 · 설치 출처 / 허용된 권한 |
| | [`launch`](docs/commands.md#ak-launch) · [`kill`](docs/commands.md#ak-kill) · [`restart`](docs/commands.md#ak-restart) | 실행 · 강제 종료 · 재시작하며 실행 시간 측정 |
| | [`clear`](docs/commands.md#ak-clear) · [`uninstall`](docs/commands.md#ak-uninstall) | 데이터 삭제 · 제거 |
| | [`activities`](docs/commands.md#ak-activities) · [`logcat`](docs/commands.md#ak-logcat) | 액티비티 스택 · 앱 하나의 로그 |
| 화면 | [`screenshot`](docs/commands.md#ak-screenshot) · [`record`](docs/commands.md#ak-record) | PNG를 파일이나 클립보드로 · MP4 녹화 |
| 파일 | [`pull`](docs/commands.md#ak-pull) | 디바이스의 최근 파일을 골라 가져오기 |
| 디바이스 | [`devices`](docs/commands.md#ak-devices) · [`mac`](docs/commands.md#ak-mac) · [`wifi`](docs/commands.md#ak-wifi) | 디바이스 목록 · MAC 주소 · Wi-Fi 디버깅 |
| 설정 | [`doctor`](docs/commands.md#ak-doctor) · [`completion`](docs/commands.md#ak-completion) | 설정 점검 · 셸 자동 완성 스크립트 |

모든 옵션과 예시는 **[docs/commands.md](docs/commands.md)**(영문)에 있고, `ak <명령> --help`로도 같은 내용을 볼 수 있습니다.

## 스크립트와 CI에서

- 디바이스는 `-s <시리얼>`, `ANDROID_SERIAL`, `-a`(전부)로 고릅니다. 터미널이 아니면 선택 화면을 띄우지 않고 `-s`를 요구합니다
- 결과는 stdout, 메시지는 stderr. `devices`, `info`, `permissions`, `signature`, `mac`, `activities`, `restart`, `doctor`는 `--json`을 지원합니다
- 종료 코드: `0` 성공, `1` 실패(일부 실패 포함), `2` 사용법 오류, `130` 중단
- 짧은 옵션은 묶어 쓸 수 있고(`-ea` = `-e -a`) 인자 앞뒤 어디에 와도 됩니다. 색은 터미널에서만(`NO_COLOR`, `--no-color`)

## 문제 해결

먼저 `ak doctor`를 실행하세요. 자주 겪는 경우는 **[docs/troubleshooting.md](docs/troubleshooting.md)**(영문)에 있습니다: macOS가 받은 실행 파일을 막을 때, Android Studio를 켜면 디바이스 연결이 끊길 때, 설치가 Play 프로텍트 확인을 기다릴 때, 자동 완성이 예전 것일 때.

## 프로젝트

개인 프로젝트라 Pull Request는 받지 않습니다. 버그 제보는 [Issue](https://github.com/luminousvault/homebrew-adb-extensions/issues)로 환영합니다 — `ak doctor -v`의 Environment 블록을 붙여 주세요. 변경 내역은 [CHANGELOG](CHANGELOG.md)에 있습니다. MIT 라이선스.
