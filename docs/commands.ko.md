# ak 명령 레퍼런스

<!-- 명령 정의에서 `make docs`로 생성합니다. 직접 고치지 마세요. -->

[English](commands.md) · **한국어**

사용법: `ak <command> [options] [arguments]`. 옵션은 인자 앞뒤 어디에 와도 되고, 짧은 옵션은 묶어 쓸 수 있습니다(`-ea`는 `-e -a`). 디바이스를 쓰는 명령은 `-s/--serial`, 없으면 `$ANDROID_SERIAL`로 디바이스를 정합니다. 둘 다 없을 때 연결된 디바이스가 한 대면 그 디바이스를 쓰고, 여러 대면 물어봅니다(지원하는 명령은 `-a/--all`로 전부 지정). 결과는 stdout, 메시지는 stderr로 나갑니다. 종료 코드: 0 성공, 1 실패, 2 사용법 오류, 130 중단.

## 목차

- **APK:** [install](#ak-install), [extract](#ak-extract), [signature](#ak-signature)
- **앱:** [info](#ak-info), [permissions](#ak-permissions), [launch](#ak-launch), [kill](#ak-kill), [clear](#ak-clear), [uninstall](#ak-uninstall), [restart](#ak-restart), [activities](#ak-activities), [logcat](#ak-logcat)
- **화면:** [screenshot](#ak-screenshot), [record](#ak-record)
- **파일:** [pull](#ak-pull)
- **디바이스:** [devices](#ak-devices), [mac](#ak-mac), [wifi](#ak-wifi)
- **설정:** [doctor](#ak-doctor), [completion](#ak-completion)
- [전역 옵션](#전역-옵션)

## ak install

한 대 또는 여러 대에 APK 설치.

```
ak install [options] [apk|directory...]
```

APK 파일을 디바이스 한 대 또는 여러 대에 설치합니다.
APK 파일을 지정하면 그 파일을 설치하고, 폴더를 지정하거나 인자가 없으면 APK를 고릅니다(최신 순).
흔한 실패는 알아서 처리합니다. 테스트 전용 APK는 -t, 다운그레이드는 -d로 다시 시도합니다.
다운그레이드가 허용되지 않거나 서명이 다르면 먼저 제거할지 묻습니다.
여러 디바이스에는 상태판을 보여 주며 동시에 설치합니다. 제거 여부를 답해야 하는 디바이스는
바로 묻고 그동안 다른 디바이스는 계속 설치합니다. 거절하면 그 디바이스만 멈춥니다.
'&lt;file&gt;.apk.idsig'가 있으면 --no-incremental을 붙입니다.

**인자**

| 인자 | 설명 |
|---|---|
| `apk` | 설치할 APK 파일 |
| `directory` | APK를 고를 폴더 |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-l, --latest` | 최신 APK 설치 (현재 폴더 또는 지정한 폴더) |
| `-e, --every-apk` | 모든 APK 설치 (현재 폴더 또는 지정한 폴더) |
| `-f, --filter WORDS` | 이름에 모든 단어가 들어간 APK 중에서 고르기 |
| `-r` | 데이터를 유지한 채 기존 앱 교체 (항상 켜짐) |
| `-t, --test-only` | 테스트 전용 APK 허용 (주지 않아도 필요하면 -t로 재시도) |
| `-d, --downgrade` | 버전 다운그레이드 허용 (주지 않아도 필요하면 -d로 재시도) |
| `-y, --yes` | 디바이스가 업데이트를 거부하면 묻지 않고 제거 후 재설치 (앱 데이터 삭제) |
| `-s, --serial ID` | 대상 디바이스, 여러 번 지정 가능 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-a, --all` | 연결된 모든 디바이스 |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak install
ak install app-debug.apk
ak install -l build/outputs/apk/debug
ak install -f "myapp release"
ak install app.apk --all
ak install -e build/outputs/apk --all
ak install app.apk -s emulator-5554 -y
```

## ak extract

설치된 앱의 APK 저장.

```
ak extract [options] [package] [file]
```

설치된 패키지의 APK를 로컬 파일로 저장합니다.
두 인자는 순서에 상관없습니다. .apk로 끝나거나 '/'가 들어간 값을 파일로 봅니다.

**인자**

| 인자 | 설명 |
|---|---|
| `package` | 패키지 이름 (기본: 화면의 앱) |
| `file` | 저장할 파일 (기본: &lt;package&gt;.apk) |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak extract
ak extract myapp.apk
ak extract com.example.app
ak extract com.example.app out.apk
```

## ak signature

서명 인증서 해시 보기.

```
ak signature [options] [package|file.apk]
```

설치된 패키지나 로컬 APK의 서명 인증서(DN, SHA-256, SHA-1, MD5)를 보여 줍니다.
Android SDK는 필요 없습니다. APK를 파싱하지 못할 때만 apksigner를 씁니다.
인자가 없으면 연결된 디바이스마다 화면의 앱과 현재 폴더의 APK 중에서 고릅니다.

**인자**

| 인자 | 설명 |
|---|---|
| `package` | 디바이스에 설치된 패키지 |
| `file.apk` | 로컬 APK 파일 |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `--json` | 결과를 JSON으로 출력 |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak signature
ak signature com.example.app
ak signature app-release.apk
ak signature app.apk --json | jq -r '.signers[0].sha256'
```

## ak info

앱 버전과 SDK 레벨 보기.

```
ak info [options] [package]
```

패키지의 버전, SDK 레벨, debuggable 여부, 설치 출처를 보여 줍니다.

**인자**

| 인자 | 설명 |
|---|---|
| `package` | 패키지 이름 (기본: 화면의 앱) |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `--json` | 결과를 JSON으로 출력 |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak info
ak info com.example.app
ak info --json | jq .versionName
```

## ak permissions

앱에 허용된 권한 목록.

```
ak permissions [options] [package]
```

패키지에 허용된 권한을 나열합니다.

**인자**

| 인자 | 설명 |
|---|---|
| `package` | 패키지 이름 (기본: 화면의 앱) |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `--json` | 결과를 JSON으로 출력 |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak permissions
ak permissions com.example.app --json
```

## ak launch

앱 실행.

```
ak launch [options] <package>
```

패키지의 런처 액티비티를 실행합니다.

**인자**

| 인자 | 설명 |
|---|---|
| `package` | 패키지 이름 |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak launch com.example.app
```

## ak kill

앱 강제 종료.

```
ak kill [options] [package...]
```

앱을 강제 종료합니다. 여러 앱을 한 번에 지정할 수 있습니다. 패키지 이름을 생략하면 화면의 앱을 씁니다.

**인자**

| 인자 | 설명 |
|---|---|
| `package` | 패키지 이름, 여러 개 가능 (기본: 화면의 앱) |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak kill
ak kill com.example.app com.example.other
```

## ak clear

앱 데이터와 캐시 삭제.

```
ak clear [options] [package...]
```

앱의 데이터와 캐시를 지워 새로 설치한 상태로 만듭니다. 여러 앱을 한 번에 지정할 수 있습니다.
패키지 이름을 생략하면 화면의 앱을 씁니다. 따로 묻지 않습니다.

**인자**

| 인자 | 설명 |
|---|---|
| `package` | 패키지 이름, 여러 개 가능 (기본: 화면의 앱) |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak clear
ak clear com.example.app
```

## ak uninstall

앱 제거.

```
ak uninstall [options] [package]
```

앱을 제거합니다(데이터도 함께 지워집니다).
패키지 이름을 생략하면 화면의 앱을 씁니다. 따로 묻지 않습니다.

**인자**

| 인자 | 설명 |
|---|---|
| `package` | 패키지 이름 (기본: 화면의 앱) |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak uninstall
ak uninstall com.example.app
```

## ak restart

앱을 재시작하며 실행 시간 측정.

```
ak restart [options] [package]
```

앱을 강제 종료하고 런처 액티비티를 다시 실행해, 실행에 걸린 시간을 보여 줍니다
(am start -W: TotalTime). --repeat을 주면 여러 번 재시작하고 min/avg/max를 보여 줍니다.

**인자**

| 인자 | 설명 |
|---|---|
| `package` | 패키지 이름 (기본: 화면의 앱) |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-r, --repeat N` | N번 재시작 (기본 1) |
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `--json` | 결과를 JSON으로 출력 |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak restart
ak restart com.example.app -r 5
ak restart --json
```

## ak activities

액티비티 스택 보기.

```
ak activities [options]
```

화면에 떠 있는 태스크의 액티비티 스택을 보여 줍니다. --every-task를 주면 모든 태스크를 보여 줍니다.

**옵션**

| 옵션 | 설명 |
|---|---|
| `-e, --every-task` | 모든 태스크 보기 |
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `--json` | 결과를 JSON으로 출력 |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak activities
ak activities -e
ak activities --json
```

## ak logcat

앱 하나의 로그 보기.

```
ak logcat [options] [package]
```

앱 하나의 logcat 로그를 보여 주고, 앱이 재시작해도 이어서 따라갑니다(새 프로세스 ID를
자동으로 잡고, 'pkg:remote' 같은 프로세스도 포함합니다). 디바이스 로그 버퍼에 남아 있는
그 앱의 로그를 먼저 보여 줍니다(-c를 주면 비우고 시작). Ctrl+C로 멈춥니다.

**인자**

| 인자 | 설명 |
|---|---|
| `package` | 패키지 이름 (기본: 화면의 앱) |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-l, --level LEVEL` | 최소 레벨: V, D, I, W, E, F (기본 V) |
| `-c, --clear` | 디바이스 로그 버퍼를 먼저 비우기 |
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak logcat
ak logcat com.example.app -l W
ak logcat -c > app.log
```

## ak screenshot

화면을 파일이나 클립보드로 캡처.

```
ak screenshot [options] [file|directory]
```

화면을 캡처해 이 컴퓨터에 PNG로 저장합니다(디바이스에는 아무것도 남지 않습니다).
기본 이름은 현재 폴더의 screenshot-&lt;model&gt;-&lt;time&gt;.png입니다.
디바이스가 여러 대면 디바이스마다 파일을 하나씩 저장합니다. 저장한 경로는 stdout으로 출력합니다.
-c는 파일로 저장하지 않고 클립보드에 복사합니다(디바이스 한 대일 때만). 파일이나 폴더를
지정하거나 -o를 함께 주면 저장도 합니다.

**인자**

| 인자 | 설명 |
|---|---|
| `file` | 저장할 .png 파일 (디바이스 한 대일 때) |
| `directory` | 이 폴더에 저장 |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-c, --clipboard` | 이미지를 클립보드에 복사 (단독으로 쓰면 파일은 저장하지 않음) |
| `-o, --open` | 저장한 파일을 기본 앱으로 열기 |
| `-s, --serial ID` | 대상 디바이스, 여러 번 지정 가능 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-a, --all` | 연결된 모든 디바이스 |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak screenshot
ak screenshot -o
ak screenshot -c
ak screenshot -c ~/Desktop
ak screenshot home.png
ak screenshot ~/Desktop --all
```

## ak record

화면 녹화.

```
ak record [options] [file|directory]
```

Ctrl+C를 누르거나 시간 제한에 이를 때까지 화면을 MP4로 녹화한 뒤, 영상을 이 컴퓨터에
저장하고 디바이스에서는 지웁니다. Android는 한 번에 180초까지만 녹화하고 소리는 녹음하지 않습니다.
저장한 경로는 stdout으로 출력합니다.
한 번에 디바이스 한 대만 녹화합니다(-s로 지정).

**인자**

| 인자 | 설명 |
|---|---|
| `file` | 저장할 .mp4 파일 (기본: record-&lt;model&gt;-&lt;time&gt;.mp4) |
| `directory` | 이 폴더에 저장 |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-t, --time-limit SECONDS` | 이 시간(초)이 지나면 멈춤 (1-180, 기본 180) |
| `--size WxH` | 영상 크기, 예: 1280x720 (기본: 화면 크기) |
| `-o, --open` | 저장한 파일을 기본 앱으로 열기 |
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak record
ak record -o
ak record bug.mp4
ak record -t 30 --size 720x1600
```

## ak pull

디바이스의 최근 파일을 골라 가져오기.

```
ak pull [options] [directory]
```

디바이스에서 최근 수정된 파일을 골라 가져옵니다(파일 앱의 '최근' 탭처럼 MediaStore 기준).
파일 이름은 그대로 둡니다. 같은 이름의 로컬 파일은 덮어쓰고, 고른 파일끼리 이름이 같으면
' (N)'을 붙입니다. 최신 파일 300개를 보여 줍니다.

**인자**

| 인자 | 설명 |
|---|---|
| `directory` | 저장할 폴더 (기본: 현재 폴더, 없으면 확인 후 생성) |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-y, --yes` | 저장할 폴더가 없으면 묻지 않고 생성 |
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak pull
ak pull ~/Downloads/device
ak pull new-dir -y
```

## ak devices

연결된 디바이스 보기.

```
ak devices [options]
```

연결된 디바이스를 모델, Android 버전, CPU ABI, 상태와 함께 보여 줍니다.
ABI 옆의 (64/32), (64only), (32only)는 32/64비트 앱 지원 여부입니다.

**옵션**

| 옵션 | 설명 |
|---|---|
| `--json` | 결과를 JSON으로 출력 |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak devices
ak devices --json
```

## ak mac

디바이스 MAC 주소 보기.

```
ak mac [options]
```

디바이스의 MAC 주소를 보여 줍니다.
  Wi-Fi (current)  현재 연결의 MAC (네트워크마다 다르고 무작위일 수 있음)
  Wi-Fi (factory)  디바이스 고유의 Wi-Fi MAC, 볼 수 있는 경우만 (Wi-Fi가 꺼져 있어도 표시)
  Bluetooth        Bluetooth MAC

**옵션**

| 옵션 | 설명 |
|---|---|
| `-s, --serial ID` | 대상 디바이스, 여러 번 지정 가능 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-a, --all` | 연결된 모든 디바이스 |
| `--json` | 결과를 JSON으로 출력 |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak mac
ak mac --all
ak mac --json
```

## ak wifi

디바이스를 Wi-Fi 디버깅으로 전환.

```
ak wifi [options] [off]
```

USB로 연결된 디바이스를 Wi-Fi 디버깅(adb tcpip)으로 바꾸고 연결해, 케이블을 뽑아도 쓸 수 있게
합니다. 컴퓨터와 디바이스가 같은 네트워크에 있어야 합니다.
'ak wifi off'는 Wi-Fi 디바이스를 USB 모드로 되돌리고 연결을 끊습니다.
연결된 주소는 stdout으로 출력합니다.

**인자**

| 인자 | 설명 |
|---|---|
| `off` | USB로 되돌리고 연결 끊기 |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-p, --port PORT` | TCP 포트 (기본 5555) |
| `-s, --serial ID` | 대상 디바이스 (기본: $ANDROID_SERIAL, 없으면 물어봄) |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak wifi
ak wifi -s emulator-5554
ak wifi off
```

## ak doctor

adb, 디바이스, 셸 설정 점검.

```
ak doctor [options]
```

ak가 쓰는 adb, 그 밖의 adb 실행 파일, 연결된 디바이스, 셸 자동 완성, 선택 도구를 점검합니다.
adb를 찾는 순서: AK_ADB, ANDROID_HOME, ANDROID_SDK_ROOT, PATH, Android Studio 기본 SDK, ~/.ak/platform-tools.
버전이 다른 adb를 함께 쓰면 adb 서버가 재시작되면서 디바이스 연결이 끊깁니다.
[✓] 정상, [!] 고치는 편이 좋음, [✗] 문제. 문제가 있으면 종료 코드 1로 끝납니다.

**옵션**

| 옵션 | 설명 |
|---|---|
| `-v, --verbose` | 모든 상세 정보 보기 |
| `--json` | 결과를 JSON으로 출력 |
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
ak doctor
ak doctor -v
ak doctor --json
```

## ak completion

셸 자동 완성 스크립트 출력.

```
ak completion [options] <shell>
```

zsh, bash, fish, PowerShell용 탭 자동 완성 스크립트를 출력합니다.
Homebrew는 bash, zsh, fish 자동 완성을 함께 설치합니다. 그 밖의 셸 설정 방법은 'ak doctor'에서 볼 수 있습니다.

**인자**

| 인자 | 설명 |
|---|---|
| `shell` | zsh, bash, fish 또는 powershell |

**옵션**

| 옵션 | 설명 |
|---|---|
| `-h, --help` | 이 도움말 보기 |

**예시**

```bash
source <(ak completion zsh)          # in ~/.zshrc, after compinit
eval "$(ak completion bash)"         # in ~/.bashrc
ak completion fish > ~/.config/fish/completions/ak.fish
ak completion powershell | Out-String | Invoke-Expression   # in $PROFILE
```

## 전역 옵션

| 옵션 | 설명 |
|---|---|
| `-h, --help` | 도움말 보기 |
| `-v, --version` | 버전 보기 |
| `--no-color` | 색 끄기 |

| 환경 변수 | 설명 |
|---|---|
| `ANDROID_SERIAL` | 기본 대상 디바이스 |
| `AK_ADB` | adb 실행 파일 경로 |
| `NO_COLOR` | 색 끄기 |
| `AK_DEBUG` | 1이면 $TMPDIR/ak-debug.log에 디버그 로그 기록 (경로는 AK_DEBUG_LOG로 변경) |
