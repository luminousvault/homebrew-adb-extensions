[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Version](https://img.shields.io/github/v/release/luminousvault/homebrew-adb-extensions?color=blue)](https://github.com/luminousvault/homebrew-adb-extensions/releases/latest)
![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey.svg)
![Go](https://img.shields.io/badge/Go-00ADD8?logo=go&logoColor=white)
![Android](https://img.shields.io/badge/Android-3DDC84?logo=android&logoColor=white)
![Homebrew](https://img.shields.io/badge/Homebrew-supported-orange.svg?logo=homebrew)

# ADB Extensions Kit (ak)

**안드로이드 개발을 위한 필수 ADB 유틸리티**

APK 관리, 디바이스 제어, 앱 검사 등 Android Debug Bridge(ADB) 작업을 단순화하는 통합 CLI 도구입니다.

![ak install 데모](demo/install.gif)

**Languages:** [🇺🇸 English](README.md) | [🇰🇷 한국어](README.ko.md)

## 주요 기능

- **통합 CLI** - 모든 ADB 작업을 하나의 명령어로 실행
- **멀티 디바이스 지원** - 여러 디바이스에 동시 APK 설치
- **인터랙티브 UI** - 키보드 탐색이 가능한 직관적인 선택 인터페이스
- **자동 복구** - 자동 에러 처리 및 복구 시도
- **탭 자동완성** - 명령어 및 옵션 Zsh 자동완성
- **풍부한 출력** - 색상 구분, 구조화된 정보 표시

## 설치

### Homebrew (권장)

```bash
brew tap luminousvault/adb-extensions
brew trust luminousvault/adb-extensions
brew install ak
```

> **참고:** Homebrew 6.0부터 서드파티 탭은 명시적으로 신뢰(trust)해야 합니다. `Error: Refusing to load formula ... from untrusted tap` 오류가 나오면 `brew trust luminousvault/adb-extensions`를 실행한 뒤 다시 설치하세요.

### 소스에서 설치

```bash
# 저장소 클론
git clone https://github.com/luminousvault/homebrew-adb-extensions.git
cd homebrew-adb-extensions

# 빌드 후 ak와 zsh 자동 완성을 /usr/local에 설치 (Go 1.27+)
make install

# 제거
make uninstall
```

`make install PREFIX=~/.local`처럼 다른 위치에 설치할 수 있습니다. 자동 완성 파일은 `$PREFIX/share/zsh/site-functions`에 들어가므로 그 경로가 `fpath`에 있어야 합니다.

또는 `go install`로 설치:

```bash
go install github.com/luminousvault/homebrew-adb-extensions/cmd/ak@latest
```

### 빌드된 바이너리

[GitHub Releases](https://github.com/luminousvault/homebrew-adb-extensions/releases/latest)에서 OS별(macOS, Linux, Windows / amd64·arm64) 압축 파일을 받으세요. `ak` 실행 파일과 Zsh 자동 완성 파일이 들어 있습니다.

### 요구 사항

필요한 것은 `adb` 하나입니다. ak는 다음 순서로 찾아 처음 발견한 것을 씁니다.

1. `AK_ADB` 환경 변수
2. `$ANDROID_HOME/platform-tools`, `$ANDROID_SDK_ROOT/platform-tools`
3. `PATH` (예: `brew install --cask android-platform-tools`)
4. Android Studio 기본 SDK (`~/Library/Android/sdk`, `~/Android/Sdk`, `%LOCALAPPDATA%\Android\Sdk`)
5. `~/.ak/platform-tools`

어떤 adb를 쓰는지는 `ak doctor`로 확인합니다. 버전이 다른 adb를 함께 쓰면(예: ak와 Android Studio) adb 서버가 서로 재시작되어 디바이스 연결이 끊기는데, `ak doctor`가 이를 경고합니다.

## 빠른 시작

```bash
# APK 설치 (인터랙티브 선택)
ak install

# 특정 APK 설치
ak install app.apk

# 앱 정보 조회
ak info com.example.app

# 앱 실행
ak launch com.example.app

# 연결된 디바이스 목록
ak devices
```

## 사용법

### 기본 문법

```bash
ak <command> [options] [arguments...]
```

**참고:** 많은 명령어가 패키지를 지정하지 않으면 포그라운드 앱을 자동으로 감지합니다. 자세한 사용 시나리오는 [사용 예시](#사용-예시)를 참조하세요.

### 디바이스 지정

| 옵션 | 동작 |
|---|---|
| `-s, --serial <ID>` | 이 디바이스만 (`install`, `mac`은 여러 번 지정 가능) |
| `ANDROID_SERIAL` | 기본 대상 디바이스 (adb와 같은 규칙) |
| `-a, --all` | 연결된 모든 디바이스 (`install`, `mac`, `screenshot`) |
| (없음) | 연결된 디바이스가 하나면 그것, 여러 대면 선택 UI |

터미널이 아니면(파이프, CI) 선택 UI를 띄우지 않고 `-s`를 지정하라고 안내합니다.

### 출력과 스크립트 사용

- 옵션은 인자 앞뒤 어디에 써도 되고, 짧은 옵션은 묶어 쓸 수 있습니다: `ak install -ea`는 `ak install -e -a`, `ak logcat -clW`는 `ak logcat -c -l W`와 같습니다.
- 결과는 stdout, 진행·경고·오류·질문은 stderr로 나갑니다. `ak info --json | jq`에는 결과만 전달됩니다.
- `--json`: `devices`, `info`, `permissions`, `signature`, `mac`, `activities`, `doctor`의 결과를 JSON으로 출력합니다.
- `-y, --yes`: 확인 질문(예: `install`의 제거 후 재설치)에 자동으로 예라고 답합니다. 터미널이 아니고 `-y`도 없으면 임의로 진행하지 않습니다.
- 색상은 터미널에서만 씁니다. `NO_COLOR=1` 또는 `--no-color`로 끄고, `CLICOLOR_FORCE=1`로 강제로 켭니다.
- 종료 코드: `0` 성공, `1` 실패(일부 실패 포함), `2` 사용법 오류, `130` 중단.

### 사용 가능한 명령어

#### APK 관리

- `install [apk_files|directories...]` - APK 파일 설치 (APK 파일을 지정하면 바로 설치, 디렉토리나 인자 없음이면 선택 UI)
  - APK 선택:
    - `-l, --latest` - 최신 APK 설치 (현재 디렉토리 또는 지정된 디렉토리)
    - `-e, --every-apk` - 모든 APK 설치 (현재 디렉토리 또는 지정된 디렉토리). 설치할 목록을 먼저 보여 줌
    - `-f, --filter <words>` - 이름에 모든 단어가 들어간 APK 중에서 선택
  - 디바이스 옵션:
    - `-a, --all` - 연결된 모든 디바이스에 동시에 설치
    - `-s, --serial <ID>` - 지정한 디바이스에 설치 (여러 번 지정 가능)
  - ADB 옵션:
    - `-r` - 기존 앱 교체 (항상 적용)
    - `-t, --test-only` - 테스트 전용 APK 허용
    - `-d, --downgrade` - 버전 다운그레이드 허용
  - `-y, --yes` - 복구에 제거가 필요할 때 묻지 않고 제거 후 재설치
- `extract [package|filename] [filename|package]` - 디바이스에서 APK 추출 (순서 무관)

#### 파일 관리

- `pull [directory]` - 디바이스 최근 파일 가져오기 (MediaStore '최근 파일', 인터랙티브 다중 선택)
  - 인자 없음: 현재 폴더에 저장
  - 디렉터리 인자: 해당 폴더에 저장 (없으면 확인 후 생성, `-y`면 바로 생성)
  - 파일은 원본 이름으로 저장 (이름이 겹치면 ' (N)' 접미사 부여)

#### 앱 정보

**참고:** `[package]`를 생략하면 포그라운드 앱을 자동으로 감지합니다.

- `info [package]` - 앱 정보 표시 (버전, SDK, 디버그 가능 여부, 설치자)
- `permissions [package]` - 허용된 앱 권한 목록
- `signature [package|apk_file]` - 앱 서명 표시 (인터랙티브 선택 지원)
- `activities [-e]` - 액티비티 스택 표시 (`-e, --every-task` 모든 태스크)
- `logcat [package] [-l level] [-c]` - 앱 하나의 로그만 표시. 앱이 재시작해도 새 PID를 따라감 (`pkg:remote` 프로세스 포함)

#### 화면

- `screenshot [file|directory]` - PC에 PNG로 저장 (디바이스에 파일을 남기지 않음). `--all`은 디바이스별로 저장, `-o, --open`은 저장 후 열기
- `record [file|directory] [-t seconds] [--size WxH]` - Ctrl+C 또는 시간 제한(최대 180초)까지 녹화한 뒤 MP4를 저장하고 디바이스에서 삭제. `-o, --open`은 저장 후 열기

#### 앱 제어

**참고:** `[package]` 또는 `[packages...]`를 생략하면 포그라운드 앱을 자동으로 감지합니다.

- `launch <package>` - 앱 실행 (메인 액티비티)
- `restart [package] [-r N]` - 강제 종료 후 다시 실행하고 실행 시간(`am start -W`) 표시. `-r, --repeat`은 반복 후 최소/평균/최대
- `kill [packages...]` - 앱 강제 종료
- `clear [packages...]` - 앱 데이터 삭제
- `uninstall [package]` - 앱 제거

#### 디바이스 관리

- `devices` - 연결된 디바이스 목록 (브랜드, 모델, ID, Android 버전, CPU ABI 및 32/64비트 지원 여부)
- `mac [options]` - 현재 Wi-Fi MAC(랜덤 여부·SSID 포함), 공장 Wi-Fi MAC, Bluetooth MAC 표시
  - `-a, --all` 연결된 모든 디바이스 표시
- `wifi [off]` - USB로 연결된 디바이스를 Wi-Fi 디버깅으로 전환하고 연결 (`adb tcpip` + IP 확인 + `adb connect`). `off`는 USB로 되돌림
- `doctor` - 사용 중인 adb, 버전이 다른 adb, 연결된 디바이스(권한 없음·오프라인), 선택 도구를 점검. 문제가 있으면 종료 코드 1

### 인터랙티브 UI 기능

#### APK 선택

- **방향키** (위/아래) - APK 탐색
- **Space** - 선택 토글
- **A** - 전체 선택/해제
- **S** - 시간순/이름순 정렬 전환
- **/** - 이름 일부를 입력해 필터링
- **숫자키** (1-9) - 빠른 선택 (단일 항목, 9개 이하 APK)
- **Enter** - 선택 확정
- **Ctrl+C** - 취소
- 여러 디렉터리에서 선택 시 각 항목 뒤에 출처 폴더명이 dim 색상으로 표시됨 (예: `(directory)`)

#### 디바이스 선택

- **방향키** (위/아래) - 디바이스 탐색
- **숫자키** (1-9) - 빠른 선택 (9개 이하 디바이스)
- **Enter** - 선택 확정
- **Ctrl+C** - 취소

### 글로벌 옵션

```bash
ak --version, -v                # 버전 정보 표시
ak --help, -h                   # 도움말 표시
ak --no-color <command>         # 색상 끄기
ak <command> --help             # 명령어별 도움말
```

## 사용 예시

### APK 설치

**현재 디렉토리에서 인터랙티브 선택:**
```bash
ak install
```

**특정 APK 설치:**
```bash
ak install app.apk
```

**최신 APK 설치:**
```bash
ak install -l
```

**특정 디렉토리에서 최신 APK 설치:**
```bash
ak install -l /path/to/dir
ak install /path/to/dir -l  # 위와 동일
```

**모든 APK 설치:**
```bash
ak install -a
```

**특정 디렉토리에서 모든 APK 설치:**
```bash
ak install -a /path/to/dir
ak install /path/to/dir -a  # 위와 동일
```

**필터링:**
```bash
ak install -f debug              # 현재 디렉토리
ak install -f debug /path/to/dir  # 특정 디렉토리
```

**모든 연결된 디바이스에 설치:**
```bash
ak install -a app.apk
ak install -s emulator-5554 -s emulator-5556 app.apk
ak install -e -a                 # 현재 디렉토리의 모든 APK를 모든 디바이스에
```

디바이스가 여러 대면 상태판에 디바이스별 진행 상황이 표시됩니다. 확인이 필요한 디바이스(예: 서명이 달라 먼저 제거해야 하는 경우)는 바로 질문하고, 그동안 다른 디바이스는 계속 설치합니다. 질문이 이어지면 차례로 묻습니다(`a`는 이번 실행에서 모든 디바이스의 남은 질문에 모두 예). 마지막에 요약이 나옵니다.

**설치가 멈춘 것 같다면:** 한 단계가 10초를 넘기면 `(check the device screen)`이 표시됩니다. 대개 디바이스가 사용자의 응답을 기다리는 중입니다. Google Play 프로텍트는 처음 보는 앱(제거 후 재설치한 앱 포함)을 검사할지 묻고, 일부 디바이스는 USB 설치를 허용할지 묻습니다. 디바이스에서 답하면 설치가 이어집니다.

테스트용 디바이스라면 adb로 설치하는 앱을 Play 프로텍트가 검사하지 않도록 끌 수 있습니다. 디바이스 보안 설정이므로 ak가 대신 바꾸지는 않습니다:

```bash
adb shell settings put global verifier_verify_adb_installs 0   # 1이면 다시 켜짐
adb -s <serial> shell settings put global verifier_verify_adb_installs 0   # 디바이스가 여러 대일 때 (`ak devices`로 시리얼 확인)
```

**스크립트·CI에서:**
```bash
ak install -s "$SERIAL" -y app.apk   # 질문 없음, 하나라도 실패하면 종료 코드 1
```

**디렉토리에서 인터랙티브 선택:**
```bash
ak install /path/to/dir
```

**여러 디렉터리에서 인터랙티브 선택:**
```bash
ak install /path/to/dir1 /path/to/dir2
```

### 최근 파일 가져오기

**최근 파일을 선택해서 현재 폴더로 가져오기:**
```bash
ak pull
```

**지정 폴더로 가져오기 (없으면 생성):**
```bash
ak pull /path/to/folder
```

### APK 추출

**포그라운드 앱 추출:**
```bash
ak extract
```

**포그라운드 앱을 사용자 지정 파일명으로 추출:**
```bash
ak extract myapp.apk
```

**특정 패키지 추출:**
```bash
ak extract com.example.app
```

**패키지와 파일명 지정 (순서 무관):**
```bash
ak extract com.example.app my.apk
ak extract my.apk com.example.app  # 위와 동일
```

### 앱 정보

**앱 정보 표시 (포그라운드 앱 자동 감지):**
```bash
ak info
ak info com.example.app
```

**앱 권한 목록:**
```bash
ak permissions
ak permissions com.example.app
```

**앱 서명 확인 (인터랙티브 선택):**
```bash
ak signature                   # 인터랙티브: 포그라운드 앱 + APK 파일
ak signature com.example.app   # 설치된 앱
ak signature app.apk           # 로컬 APK 파일
```

**액티비티 스택 조회:**
```bash
ak activities                   # 포그라운드 태스크
ak activities -a               # 모든 태스크
```

### 앱 제어

**앱 실행:**
```bash
ak launch com.example.app
```

**앱 종료:**
```bash
ak kill                         # 포그라운드 앱
ak kill com.app1 com.app2      # 여러 앱
```

**앱 데이터 삭제:**
```bash
ak clear                        # 포그라운드 앱
ak clear com.app1 com.app2     # 여러 앱
```

**앱 제거:**
```bash
ak uninstall                    # 포그라운드 앱 (자동 감지)
ak uninstall com.example.app
```

### 디바이스 관리

**연결된 디바이스 목록:**
```bash
ak devices
```

**MAC 주소 조회:**
```bash
ak mac                # 선택한 디바이스의 MAC 주소
ak mac -a             # 연결된 모든 디바이스 표시
ak mac --json         # JSON 출력
```

출력 예시:
```
Samsung SM-S928N (R5CX00000000) Android 15, API 35
  Wi-Fi (current)  02:00:5e:00:53:01   (randomized · SSID "MyWiFi")
  Wi-Fi (factory)  00:00:5e:00:53:10
  Bluetooth        00:00:5e:00:53:11
```

**참고:** Android 10부터 Wi-Fi MAC은 기본적으로 네트워크마다 랜덤으로 사용되므로, `Wi-Fi (current)`는 현재 연결에 쓰이는 MAC만 표시합니다(Wi-Fi가 꺼져 있거나 미연결이면 `n/a`). `Wi-Fi (factory)`는 기기 고유의 Wi-Fi MAC으로 Wi-Fi가 꺼져 있어도 조회되지만, 일부 기기는 노출하지 않을 수 있습니다. 이 경우 Wi-Fi 개인정보 보호 설정을 '기기 MAC 사용'으로 바꾸면 현재 MAC으로 확인할 수 있습니다. Bluetooth MAC 접근을 제한하는 기기에서는 `n/a`로 표시될 수 있습니다.

### 워크플로우 예시

**APK 추출 및 서명 확인:**
```bash
ak extract com.example.app
ak signature com.example.app.apk
```

**설치, 실행, 정보 조회:**
```bash
ak install app.apk
ak launch com.example.app
ak info com.example.app
```

## 버전 히스토리

자세한 버전 히스토리는 [CHANGELOG.md](CHANGELOG.md)를 참조하세요.

## 라이선스

MIT License - 자세한 내용은 [LICENSE.md](LICENSE.md)를 참조하세요.

## 작성자

Claude Hwang

## 기여하기

기여를 환영합니다! 버그 수정, 새로운 기능, 문서 개선 등 모든 기여를 감사히 받습니다.

자세한 가이드라인은 [CONTRIBUTING.md](CONTRIBUTING.md)를 참조하세요:

- 개발 환경 구축
- 프로젝트 구조
- 빌드 시스템
- 새 명령어 추가 방법
- 코드 스타일 가이드
- Pull Request 프로세스

기여자를 위한 빠른 시작:

```bash
# Fork 및 클론
git clone https://github.com/YOUR_USERNAME/homebrew-adb-extensions.git
cd homebrew-adb-extensions

# 변경사항 실행
go run ./cmd/ak <command>

# 테스트
go test ./...
```

## 감사의 말

- 런타임 의존성 없는 단일 바이너리를 위해 Go로 제작
- 효율적인 Android 개발 워크플로우의 필요성에서 영감을 받음
- Android 개발 커뮤니티에 특별한 감사

## 지원

- **이슈**: [GitHub Issues](https://github.com/luminousvault/homebrew-adb-extensions/issues)
- **토론**: [GitHub Discussions](https://github.com/luminousvault/homebrew-adb-extensions/discussions)
