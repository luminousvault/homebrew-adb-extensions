# ak 커맨드 동작 사양 (Bash v1.4.0 기준)

> **참고:** 이 문서는 v1.4.0(Bash)의 동작 기록이며, Go 이식 초기에 대조 기준으로 썼다. v2.0.0의 출력·종료 코드·옵션은
> [`docs/design/cli-output.md`](../design/cli-output.md)를 따르며 이 문서와 다르다. 명령별 v2 동작은 `ak <command> --help`와 테스트(`internal/cli/*_test.go`)가 기준이다.

Go 이식의 대조 기준. Bash 소스(`src/`)에서 추출했다. "→"는 출력/동작, `exit N`은 종료 코드.

## 공통

- 인자 없음 → 전체 도움말, `exit 0`. `-v|--version` → 버전 배너. `-h|--help` → 전체 도움말.
- 알 수 없는 커맨드 → `==> Error: Unknown command: X` + 도움말, `exit 1`.
- 커맨드 실행 전 adb 존재 확인. 없으면 설치 안내, `exit 1`. (`--version`/`--help`는 adb 없이 동작)
- 커맨드별 `-h|--help` → 커맨드 도움말, **`exit 1`** (`install`만 `exit 0`).
- 알 수 없는 `-x` 옵션 → `==> Error: Invalid option: -x` + `Try 'ak <cmd> --help' for more information.`, `exit 1`.
- 옵션 파싱은 첫 위치 인자에서 멈춘다(`install`만 옵션·위치 인자 혼용 가능).
- 출력은 항상 ANSI 색상 포함 (TTY 여부 무관).
- `AK_DEBUG=1` → 디버그 로그 파일 기록.

### 디바이스 선택
- 단일(`find_and_select_device`): `adb devices` 중 상태 `device`만. 0개 → `==> Error: No connected devices found.` `exit 1`. 1개 → 자동. 2개+ → 인터랙티브 단일 선택(필터·정렬 없음), 항목은 `pretty_device`(normal).
- 다중(`install`): 2개+ 이고 `-m` → 전부, 아니면 인터랙티브 다중 선택("Select devices for installation").
- `pretty_device`: unauthorized/offline → `(serial)`. getprop 실패 → `Unknown Device` / `Unknown Device (serial): device not responding`. 그 외 `Brand Model (serial) Android X, API N` (minimal: `Brand Model`, short: `Brand Model (serial)`). brand는 첫 글자 대문자·나머지 소문자.

### 포그라운드 패키지
`dumpsys activity activities`에서 `hist`(대소문자 무시) 포함 첫 줄의 ` u0 <pkg>/` 부분.

### 패키지 검증
- 형식 `^[a-zA-Z0-9_]+(\.[a-zA-Z0-9_]+)+$`, 설치 여부 `pm list packages --user 0`에서 `package:<pkg>` 정확히 일치.
- 실패 → `ERROR: Invalid package name format: X` / `ERROR: Package not installed on the device: X`, `exit 1`.

### APK 경로
`pm path <pkg>` 결과 중 `base.apk` 우선, 없으면 첫 줄. 비어 있으면 `ERROR: Package not found: X`.

## 인터랙티브 선택 UI (`select_interactive`)

- 화면 clear + 스크롤백 clear 후 그림. 종료 후 UI 내용은 화면에 남는다. 커서 숨김, 브래킷 붙여넣기 모드.
- 레이아웃: `==> <prompt>` / (선택) `    Location: a • b` / 빈 줄 / 카운터 / 항목들 / 빈 줄 / 도움말 / (필터 모드) 필터 박스 3줄.
- 카운터: `N item(s)` 또는 `Showing a-b / N`, 필터 시 `(filtered from M)`, 다중 시 `| K selected`.
- 항목: `  [ ] 1. name  (suffix)`; 포커스 `➤ `(CYAN), 다중 선택됨은 체크박스 GREEN. 탭 뒤는 DIM suffix(필터 대상 아님).
- 키(일반 모드): ↑↓ 이동(순환), Enter 확정(다중에서 선택 0개면 포커스 항목 선택), Space 토글(다중), A/a 전체 토글(다중), S/s 정렬 토글 time↔name(필터 가능 시), `/` 필터 모드, 1-9 즉시 선택(항목 9개 이하), Ctrl+C → `exit 130`.
- 키(필터 모드): 출력 가능 ASCII(33-126) 입력, Backspace/Delete, ←→ Home End Ctrl+A Ctrl+E 커서, 붙여넣기, ↑↓, Enter, Space 토글(다중), `/` 필터 해제 후 일반 모드.
- 필터: 대소문자 무시 부분 문자열, 첫 매치 하이라이트(노랑 배경 검정 글자). 다중 모드에서 선택됨+미매치 항목은 맨 위에 유지.
- 높이: 터미널 높이 - 10 - 1 만큼 표시, 포커스 중심 스크롤. 리사이즈 시 다시 그림.
- 결과: 단일 → 원본 인덱스. 다중 → 선택 순서대로 원본 인덱스.

## 커맨드

### devices
- 인자 불가(`Unexpected argument`). `adb devices` 전체 상태 표 출력.
- 열: Device ID / Model(`Brand Model`) / Android(`X (API N)`) / CPU(`abi (64/32|64only|32only)`) / Status(Connected·Unauthorized(RED)·Offline(DIM)·Unknown(DIM)). 최소 폭 15/20/12/10/11, 구분선 `─`. 64only는 YELLOW.
- 디바이스 없음 → `ERROR: No connected devices found.` (`exit 0`).

### info [pkg]
- 패키지 없으면 포그라운드 자동 감지(`Auto-detected package:`), 있으면 `Using specified package:`.
- `dumpsys package`에서 versionName, versionCode, minSdk, targetSdk, DEBUGGABLE 플래그, installerPackageName.
- versionName/versionCode/targetSdk 누락 → 오류 `exit 1`. installer 없으면 `(not available)`.

### permissions [pkg]
- `dumpsys package`의 `granted=true` 줄에서 `:` 앞을 앞 공백 제거해 출력. 없으면 오류 `exit 1`.

### launch <pkg>
- 패키지 없으면 도움말 `exit 1`. 미설치 → 오류 `exit 1`.
- `cmd package resolve-activity --brief` 마지막 줄 → `am start --user 0 -n`. 출력에 `Exception occurred while executing 'start'` → 오류(들여쓰기 2칸) `exit 1`.

### kill [pkg...] / clear [pkg...]
- 인자 없으면 포그라운드 자동 감지 후 검증·실행.
- 여러 개: `Starting to kill packages...` / `Starting to clear app data...` 후 각 패키지에 대해 형식 오류·미설치·중복은 메시지 후 계속.
- kill: `am force-stop`, 종료 코드로 성공 판정. clear: `pm clear` 출력에 `Success` 포함 여부.

### uninstall [pkg]
- 자동 감지·검증 후 `adb uninstall`. 출력에 `Success` → 성공, 아니면 오류 메시지(종료 코드 0).

### extract [pkg|file] [file|pkg]
- 인자 0~2개, 3개+ → `Too many arguments` `exit 1`. `.apk`로 끝나거나 `/` 포함이면 파일명.
- 파일명 기본값 `<pkg>.apk`, `.apk` 없으면 붙임. `adb pull` 실패 → 오류 `exit 1`.

### pull [dir]
- 인자 2개+ → `Too many arguments`. 디렉터리 없으면 `Create it? [y/N]` (y/Y/yes/YES만 생성), 아니면 `Aborted.` `exit 1`.
- MediaStore `content query`(최근 300개) → 다중 선택("Select files to pull", Location "Recent files (newest first)"), suffix `(상대폴더, 크기)`.
- 같은 파일명은 ` (N)` 접미사. 개별 실패해도 계속, 실패 있으면 `exit 1`.

### mac [-m]
- `-m` → 연결된 모든 디바이스, 아니면 단일 선택.
- Wi-Fi(current): API 30+ `cmd wifi status`(COMPLETED 줄) → `dumpsys wifi`(COMPLETED 줄) → `ip link show wlan0` → `/sys/class/net/wlan0/address` → `ifconfig wlan0`. randomized(locally administered bit), SSID 표시.
- Wi-Fi(factory): `wifi_sta_factory_mac_address`. Bluetooth: `settings get secure bluetooth_address` → `dumpsys bluetooth_manager`.
- 무효 MAC(형식 불일치, 02:00:00:00:00:00, 00:00:00:00:00:00)은 n/a.

### activities [-a|--all]
- 위치 인자 불가. 포그라운드 태스크(첫 ` t<N>}`)의 Hist 줄들 또는 `-a` 시 태스크별 전부.
- 최대 번호 CYAN(top), #0 GREEN(root), 중간 YELLOW. `Total: N activities in this task`.

### signature [pkg|apk]
- apksigner 필요(없으면 오류 `exit 1`) → Go 전환 후 Go 파서 우선, 실패 시 apksigner 폴백.
- 인자 없음 → 모든 디바이스 포그라운드 앱(`📱 pkg (Brand Model)`) + 현재 폴더 APK(`📦 name`, 이름순) 단일 선택, 1개면 자동.
- `.apk`로 끝나면 로컬 파일(없으면 오류 `exit 1`), 아니면 디바이스 패키지를 임시 파일로 pull 후 분석, 끝나면 삭제.
- 출력: `Signer #N certificate DN/SHA-256/SHA-1/MD5 digest` 줄, SHA-256 GREEN BOLD, SHA-1 YELLOW BOLD. `WARNING:` 줄 제외.

### install [options] [apk|dir...]
- 옵션 `-l` 최신 1개, `-a` 전부, `-f <filter>` 공백 구분 AND 필터(대소문자 무시) — 셋은 상호 배타. `-m` 모든 디바이스, `-r`(기본), `-t`, `-d`. `--xxx`·알 수 없는 옵션은 오류(stderr) `exit 1`.
- 위치 인자: APK 파일/디렉터리 혼합, 존재하지 않는 경로는 Location에 `(not found)`로 표시하며 계속. APK 아닌 파일 → 오류 `exit 1`. `-l/-a/-f`와 APK 파일 동시 사용 불가.
- APK 목록은 수정 시간 최신순. 1개면 자동 선택(`Only one APK file found:`), 여러 개면 다중 선택. 여러 출처면 suffix `(폴더명)`.
- 설치: `adb -s <serial> install -r [-t] [-d] [--no-incremental(.idsig 존재 시)] <apk>`.
- 자동 복구: `INSTALL_FAILED_TEST_ONLY` → `-t` 붙여 재시도. `INSTALL_FAILED_VERSION_DOWNGRADE` → `-d` 재시도, 그래도 실패하면 `[Y/n]` 확인 후 패키지 삭제·재설치(패키지명은 APK에서 읽음). `INSTALL_FAILED_UPDATE_INCOMPATIBLE` → `[Y/n]` 확인 후 오류 메시지의 패키지 삭제·재설치.

## Go 버전에서 의도적으로 바꾼 점

Bash v1.4.0 대비 동작이 달라지는 부분만 적는다. 나머지는 ANSI 색상 코드까지 같은 출력을 목표로 한다.

| 항목 | Bash | Go |
|---|---|---|
| 새 커맨드 | — | `ak doctor`: 사용하는 adb 경로·버전, 다른 adb 목록, 버전 충돌 경고 |
| adb 탐색 | `PATH`의 `adb`만 | `AK_ADB` → `ANDROID_HOME` → `ANDROID_SDK_ROOT` → `PATH` → Android Studio 기본 SDK → `~/.ak/platform-tools` |
| adb 설치 안내 | `brew install android-platform-tools` | `brew install --cask android-platform-tools` (+ `AK_ADB` 안내) |
| `signature` | apksigner 필요, 출력 형식이 apksigner 버전에 따라 다름 | SDK 없이 Go로 추출, `Signer #N certificate ...` 형식 고정. 파싱 실패 시 apksigner 폴백. Source Stamp 서명자는 표시하지 않고 안내 문구 출력 |
| `signature` 임시 파일 | 현재 폴더에 `tmp_signature_<pkg>.apk` | OS 임시 폴더 |
| `signature x.apk` (파일 없음) | 디바이스 선택 후 오류 | 디바이스 선택 없이 오류 |
| `install` 다운그레이드 복구 | aapt로 패키지명 추출 (aapt 필요) | Go로 매니페스트에서 추출 |
| `install -f <filter> <dir>` | 디렉터리에 APK가 1개면 필터와 무관하게 설치 | 필터를 먼저 적용 |
| `activities` 태스크 필터 | `t2}` 검색이라 태스크 12도 섞임 | ` t2}`로 정확히 일치 |
| 다중 선택 필터 고정 | 이름 정렬 후 선택 항목 고정이 엇갈릴 수 있음 | 원본 인덱스 기준으로 고정 |
| 크기 0 터미널 | `tput` 기본 24행 | 80×24로 간주 |
| 터미널보다 긴 줄 (선택 UI) | 다음 줄을 그릴 때 덮어써져 결과적으로 잘림 | 터미널 폭에 맞춰 명시적으로 자름 (이모지·한글은 2칸으로 계산) |
| 디버그 로그 | 개발 모드에서만 `build/debug.log` | `AK_DEBUG=1`이면 `$TMPDIR/ak-debug.log` (또는 `AK_DEBUG_LOG`) |
| adb 자식 프로세스 stdin | 상속 (프롬프트 입력을 adb가 가져갈 수 있음) | 연결하지 않음 |
