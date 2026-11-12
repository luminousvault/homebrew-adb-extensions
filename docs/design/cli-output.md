# ak CLI 출력 설계 가이드 (초안)

- 작성: 2026-11-12
- 상태: **구현 완료 (2026-11-12)** — `internal/ui`, `internal/cli`
- 관련: `docs/spec/parallel-install.md`(동시 설치 UI는 이 가이드 위에서 구현)

## 1. 배경

v2.0.0의 Go 전환은 v1.x(Bash) 출력을 바이트 단위로 그대로 옮기는 것을 목표로 했다. 그 결과 Bash 시절의 출력 불일치도 그대로 옮겨졌다. Go로 바꾸면서 새로 할 수 있게 된 것들이 있다. 터미널인지 감지, stdout/stderr 분리, 구조화 출력(JSON), 진행 표시, 동시 실행 등이다. 이 기능들을 쓰려면 출력 규칙부터 하나로 정해야 한다. 메이저 버전이 바뀌는 지금이 출력 형식을 바꿀 수 있는 시점이다.

## 2. 현재 상태 진단

| 문제 | 현재 | 영향 |
|---|---|---|
| 오류 형식이 둘 | `==> Error: …` 37곳, 빨간 `ERROR: …` 17곳 | 같은 종류의 오류가 커맨드마다 다르게 보임 |
| 진행 메시지 접두사가 제각각 | 파란 `==>`, 초록 `==>`, 노란 `==> Attempting to kill:`, 노란 `Auto-detected package:`, 파란 `Using specified package:` | 색이 의미(성공/경고/정보)를 나타내지 못함 |
| 모든 출력이 stdout | stderr 사용 2곳 | `ak devices > list.txt`에 오류·안내가 섞임 |
| 색상을 항상 출력 | 파이프·파일로 보내도 ANSI 코드가 섞임, `NO_COLOR` 무시 | 스크립트·CI 로그가 깨짐 |
| 종료 코드 불일치 | 도움말(`-h`)이 exit 1 (`install`만 0). `uninstall`·`clear`·설치 실패가 exit 0. 디바이스 없음이 커맨드에 따라 0 또는 1 | 스크립트에서 성공 여부를 판단할 수 없음 |
| 도움말 형식이 둘 | 대부분 `Usage/Description/Options`, `install`만 탭 정렬된 다른 형식 | |
| 디바이스를 지정할 수 없음 | 2대 이상이면 무조건 선택 UI. `-s`도, `ANDROID_SERIAL`도 지원하지 않음 | 스크립트·CI에서 쓸 수 없음 |
| 확인 질문 방식이 둘 | `install`은 한 글자 `[Y/n]`, `pull`은 Enter로 끝내는 `[y/N]`. 미리 답을 주는 `-y` 옵션 없음 | |
| 기계가 읽을 출력 없음 | `devices`, `info`, `signature`가 사람용 표·색상만 출력 | 결과를 다른 도구로 넘기기 어려움 |
| 오래 걸리는 작업에 진행 표시 없음 | 설치·pull 중 화면이 멈춘 듯 보임 | |
| 빈 줄 규칙 없음 | 커맨드마다 앞뒤 빈 줄 수가 다름 | |

## 3. 참고한 CLI 패턴

| 도구 | 가져올 점 |
|---|---|
| Homebrew | `==>` 단계 헤더(ak가 이미 쓰는 정체성), `Warning:` / `Error:` 접두사, 빈 줄 최소화 |
| GitHub CLI (`gh`) | `✓` 성공 / `!` 경고 / `X` 실패 기호, 터미널이 아니면 색·장식 제거, `--json` 출력, 비대화형이면 프롬프트 대신 오류와 해결 방법 안내 |
| Cargo | 진행 상황을 동사 하나로 짧게(`Compiling`, `Finished`), 진행 메시지는 stderr |
| Docker Compose | 여러 대상 동시 작업의 상태판(`✔ Container x  Started`) |
| kubectl | 데이터 출력은 장식 없는 표, `-o json` |
| [clig.dev](https://clig.dev) | stdout은 결과, stderr은 메시지. `NO_COLOR`. 종료 코드 규칙. 오류에는 해결 방법을 함께. `-y` / `--no-input` |

## 4. 원칙

1. **stdout은 결과, stderr은 그 밖의 모든 것.** 파이프로 넘겼을 때 결과만 남아야 한다.
2. **색은 의미로만 쓴다.** 초록=성공, 노랑=경고, 빨강=오류, 흐림=부가 정보, 파랑 굵게=단계. 강조하려고 쓰지 않는다.
3. **터미널이 아니면 꾸밈을 뺀다.** 색, 진행 표시, 선택 UI를 쓰지 않는다.
4. **오류는 무엇이 / 왜 / 어떻게 고치는지.** 고칠 방법을 알면 `Hint:`로 함께 보여 준다.
5. **종료 코드로 결과를 알 수 있어야 한다.** 일부라도 실패하면 0이 아니다.
6. **사람이 없어도 동작해야 한다.** 선택 UI와 확인 질문은 플래그와 환경 변수로 대신할 수 있어야 한다.
7. **모든 커맨드가 같은 틀을 쓴다.** 도움말, 메시지, 표, 빈 줄 모두.

## 5. 출력 규칙

### 5.1 스트림

| 내용 | 스트림 |
|---|---|
| 커맨드 결과 데이터 (디바이스 표, info 항목, 권한 목록, 서명, MAC, `--json`) | stdout |
| 단계·진행·성공·경고·오류 메시지, 확인 질문, 선택 UI, 상태판 | stderr |
| adb 원본 출력 (`Performing Streamed Install` 등) | 성공 시 숨김, 실패 시 stderr (흐림) |

### 5.2 색상

- 켜는 조건: stderr/stdout이 각각 터미널일 때.
- 끄는 조건: `NO_COLOR`가 설정됨, `--no-color`, `TERM=dumb`.
- 강제로 켜기: `CLICOLOR_FORCE=1` (CI에서 색을 보고 싶을 때).

### 5.3 메시지 레벨

| 레벨 | 형식 | 색 | 예 |
|---|---|---|---|
| 단계 | `==> 내용` | `==>` 파랑, 내용 굵게 | `==> Installing 'app-debug.apk' on Pixel 9 Pro` |
| 성공 | `✓ 내용` | 초록 | `✓ Installed 'app-debug.apk'` |
| 정보 | `  내용` (들여쓰기) | 기본 | `  Package: com.example.app (auto-detected)` |
| 부가 | `  내용` | 흐림 | adb 원본 출력, 경로, 소요 시간 |
| 경고 | `Warning: 내용` | `Warning:` 노랑 | `Warning: Skipping duplicate package 'com.example.app'` |
| 오류 | `Error: 내용` | `Error:` 빨강 | `Error: Package not installed: 'com.example.app'` |
| 힌트 | `Hint: 내용` | `Hint:` 흐림 | `Hint: Run 'ak devices' to see connected devices` |

- 기호는 `==>`, `✓`, `Warning:`, `Error:`, `Hint:` 다섯 가지만 쓴다. 색을 끄면 기호만 남는다(텍스트로 의미가 전달됨).
- 메시지 안의 패키지 이름, 파일 이름, 경로는 작은따옴표로 감싼다(`q()`): `Installed 'app.apk' on Pixel 9 Pro`. 문장과 이름의 경계가 보이게 하기 위해서다. 디바이스 이름은 사람이 읽는 이름이라 감싸지 않는다. stdout으로 내보내는 결과(저장한 경로 등)는 스크립트가 쓰므로 감싸지 않는다.
- 빈 줄 규칙: 커맨드 시작과 끝에 빈 줄을 넣지 않는다. 블록(디바이스 하나, 표 하나, `==>` 제목과 그 아래 목록) 사이에 빈 줄 하나를 둔다. 예: `==> Selected devices (2)` + 목록 → 빈 줄 → `==> Found 5 APK(s)` + 목록 → 빈 줄 → 설치 진행. 한 줄짜리 `==>` 단계끼리는 붙인다.

### 5.4 오류

```
Error: Package not installed on Google Pixel 9 Pro: com.example.app
Hint: Check the name with 'ak info' on the foreground app, or install it first
```

- 오류는 한 곳(`ui.Fail(err, hint)`)에서만 출력한다. 커맨드는 오류를 반환하고, 출력과 종료 코드는 `Main`이 정한다.
- 사용법 오류(알 수 없는 옵션, 인자 개수)는 끝에 `Run 'ak <cmd> --help' for usage.`를 붙인다.

### 5.5 종료 코드

| 코드 | 의미 |
|---|---|
| 0 | 성공 (도움말·버전 포함) |
| 1 | 실행 실패 (디바이스 없음, 패키지 없음, 설치 실패, 여러 대상 중 일부 실패) |
| 2 | 사용법 오류 (알 수 없는 옵션, 잘못된 인자 조합) |
| 130 | Ctrl+C로 중단 |

### 5.6 도움말 템플릿

모든 커맨드가 같은 순서와 정렬을 쓴다. 섹션이 없으면 생략한다.

```
Show detailed information of a package.

Usage:
  ak info [options] [package]

Arguments:
  package          Package name (default: the foreground app)

Options:
  -s, --serial ID  Target device (default: $ANDROID_SERIAL, or ask)
      --json       Print as JSON
  -h, --help       Show this help

Examples:
  ak info
  ak info com.example.app --json
```

- 도움말은 stdout에 출력하고 exit 0으로 끝낸다.
- 도움말 내용은 커맨드 정의(인자·옵션 목록)에서 만들어, 실제로 받는 옵션과 도움말 문구가 어긋나지 않게 한다. zsh completion도 같은 정의로 검증한다.

### 5.7 디바이스 지정 (전역 옵션)

| 방법 | 동작 |
|---|---|
| `-s, --serial <ID>` | 그 디바이스만 |
| `ANDROID_SERIAL` 환경 변수 | adb와 같은 규칙 |
| `-a, --all` (`install`·`mac`·`screenshot`) | 연결된 모든 디바이스 |
| 지정 없음 + 1대 | 그 디바이스 |
| 지정 없음 + 여러 대 + 터미널 | 선택 UI |
| 지정 없음 + 여러 대 + 터미널 아님 | `Error: Multiple devices connected` + 시리얼 목록 + `Hint: Use -s <serial> or ANDROID_SERIAL` (exit 1) |

#### 옵션 이름 규칙

1. 짧은 이름은 긴 이름 첫 단어의 첫 글자다 (`-l, --latest`, `-r, --repeat`). 대문자 짧은 이름은 쓰지 않는다 (Shift가 필요하고 소문자와 헷갈림).
2. 다음 짧은 이름은 모든 커맨드에서 뜻이 같고, 다른 뜻으로 쓰지 않는다.

   | 옵션 | 뜻 |
   |---|---|
   | `-s, --serial` | 디바이스 지정 |
   | `-a, --all` | 연결된 모든 디바이스 |
   | `-y, --yes` | 확인 질문에 예 |
   | `-h, --help` | 도움말 |

3. 커맨드가 다루는 항목 전부는 `all`이 아니라 `-e, --every-<항목>`이다 (`install -e, --every-apk`, `activities -e, --every-task`). `ak install -e -a` = 모든 APK를 모든 디바이스에.
4. 짧은 이름이 있는 옵션은 긴 이름도 갖는다 (`install -t, --test-only`, `-d, --downgrade`). 예외: 항상 켜져 있어 효과가 없는 v1 호환 옵션 `install -r`.
5. 서로 다른 커맨드에서는 같은 짧은 이름이 다른 뜻일 수 있다 (`install -l, --latest` / `logcat -l, --level`). 단, 2번의 예약 이름은 예외 없이 지킨다.
6. 짧은 옵션은 묶어 쓸 수 있다(getopt 규칙): `-ea` = `-e -a`, `-tdy` = `-t -d -y`. 값을 받는 옵션은 묶음의 마지막에 오고, 남은 글자가 값이 된다: `-clW` = `-c -l W`, `-cl W`도 같다. 옵션은 인자 앞뒤 어디에 와도 되고 `--opt=value`, `--`를 지원한다.

### 5.8 비대화형 실행

- 선택 UI: 터미널이 아니면 띄우지 않는다. 기본값이 있으면 쓰고(APK가 1개면 그것), 없으면 오류와 힌트를 출력한다.
- 선택 UI는 대체 화면(`\033[?1049h`)에 그린다. 닫으면 실행 전 화면과 스크롤백이 그대로 돌아오므로, 고른 결과는 `==> Selected …` 한 줄(여러 개면 아래에 목록)로 남긴다. 화면을 지우지 않고(v1은 스크롤백까지 지웠음), 터미널보다 높게 그리지 않으며(작은 터미널에서는 빈 줄 → 위치 → 도움말 → 개수 순으로 덜어냄), 크기가 바뀌면 바로 다시 그린다. 한 커맨드에서 선택 UI가 이어지면(`install`: APK → 디바이스, `pull`: 디바이스 → 파일) 대체 화면을 한 번만 열고 그 안에서 이어서 보여 준 뒤, 닫고 나서 고른 결과를 차례로 남긴다.
- 확인 질문: 형식을 `Question? [Y/n]`(Enter = 기본값)로 통일한다. `-y, --yes`면 묻지 않고 승인한다. 터미널이 아닌데 `-y`가 없으면 거절로 처리하고 힌트를 출력한다.

### 5.9 데이터 출력과 `--json`

- 사람용 출력: 표는 머리글을 굵게(흐리게 하지 않음), 터미널이면 머리글 아래에 흐린 구분선(`─`)을 긋고, 세로 테두리 없이 열을 정렬한다. 파이프로 나갈 때는 구분선 없이 머리글과 열만 출력한다(awk 등으로 읽기 쉽게). 값에는 색을 쓰지 않는다(상태 열 제외). 한글·이모지는 2칸으로 계산해 정렬한다. 여러 단어인 머리글은 밑줄로 잇는다(`PLAY_PROTECT_SCAN`). 띄어 쓰면 열이 나뉜 것처럼 보이기 때문이다.
- `--json` 지원 대상: `devices`, `info`, `permissions`, `signature`, `mac`, `activities`, `doctor`. stdout에 JSON만 출력하고, 메시지는 stderr로 보내거나 출력하지 않는다.
- JSON 키는 camelCase, 키 이름과 구조는 문서화하고 버전 사이에 유지한다.

### 5.10 진행 표시

- 1초 이상 걸릴 수 있는 작업(설치, pull, extract, signature용 APK 받기)은 터미널이면 stderr에 한 줄짜리 스피너(`⠋ Installing app-debug.apk…`)를 표시한다. 끝나면 `✓` 또는 `Error:` 줄로 바꾼다.
- 여러 디바이스 작업은 상태판(`docs/spec/parallel-install.md`)을 쓴다.
- 터미널이 아니면 시작과 끝 줄만 출력한다.

### 5.11 버전

- `ak --version`: 지금의 ASCII 아트 배너를 유지한다 (stdout). 내용만 Go 버전에 맞게(언어, 지원 OS, 의존성) 고친다.

## 6. 바뀌는 모습 예시

**`ak kill com.a.b bad com.a.b` (현재)**
```
Starting to kill packages...

==> Attempting to kill: com.a.b
Successfully killed

ERROR: Invalid package name format: bad

WARNING: Skipping duplicate package: com.a.b
```

**제안**
```
✓ Stopped com.a.b
Error: Invalid package name: bad
Warning: Skipping duplicate package com.a.b
```
(종료 코드 1: 일부 실패)

**`ak info` (현재)**
```

Auto-detected package: com.example.app

APK Detailed Info:
  versionName: 1.2.3
  ...
```

**제안**
```
==> com.example.app (foreground app on Google Pixel 9 Pro)
  versionName           1.2.3
  versionCode           1203
  minSdk                24
  targetSdk             36
  debuggable            true
  installerPackageName  com.android.vending
```

**`ak info` (디바이스 2대, 파이프)**
```
Error: Multiple devices connected: emulator-5554, emulator-5556
Hint: Use -s <serial> or set ANDROID_SERIAL
```

## 7. 구현 구조

- `internal/ui` 출력 패키지를 새로 만든다: `Step`, `Success`, `Info`, `Detail`, `Warn`, `Fail(err, hint)`, `Table`, `JSON`, `Spinner`, `Confirm`. 색상·터미널 판단은 이 패키지만 한다.
- 커맨드 코드는 `fmt`이나 색상 상수를 직접 쓰지 않는다. lint 규칙(depguard)으로 `internal/cli`에서 `internal/style` 직접 사용을 막는다.
- 커맨드는 `error`를 반환하고(`exit()` panic 제거), `Main`이 오류 종류(사용법 오류 / 실행 실패 / 중단)로 종료 코드를 정한다.
- 커맨드 정의에 인자·옵션 목록을 두고 파싱, 도움말, completion 검증이 모두 이 정의를 쓴다.
- 테스트: 커맨드별 골든 파일(색 끔/켬, JSON)로 출력 변경을 리뷰할 수 있게 한다.

## 8. 호환성

- v2.0.0에서 바꾸면 **출력 형식, 종료 코드, 도움말 위치(stdout)가 v1.x와 달라진다.** 사람이 읽는 출력이 바뀌는 것은 메이저 버전에서 허용되는 범위로 본다. 다만 v1.x 출력을 grep하던 스크립트는 깨질 수 있으므로 CHANGELOG에 "Breaking"으로 적는다.
- 커맨드 이름과 `-l`, `-f`, `-t`, `-d`, `-r`은 그대로 유지한다. §5.7 옵션 이름 규칙에 맞춰 `-m` → `-a, --all`, `install -a` → `-e, --every-apk`, `restart -n` → `-r`로 바꾸고 CHANGELOG에 "Breaking"으로 적는다.
- `docs/spec/commands.md`의 "Bash와 바이트 단위 동일" 목표는 이 가이드로 대체한다.

## 9. 진행 순서 (제안)

1. `internal/ui` + 색상·터미널 판단 + 종료 코드 체계 (커맨드는 아직 그대로)
2. 커맨드를 하나씩 새 출력으로 전환 + 골든 테스트 + 도움말 템플릿
3. `-s/--serial`, `ANDROID_SERIAL`, `--all`, 비대화형 동작, `-y`
4. `--json`
5. 진행 표시(스피너) → 여러 디바이스 동시 설치(상태판)
6. README·CHANGELOG·completion 갱신 후 v2.0.0 릴리즈

## 10. 결정 (2026-11-12, 모두 권장안 채택)

| # | 항목 | 결정 |
|---|---|---|
| 1 | v2.0.0에서 출력 형식을 바꿀지 | 바꾼다 (메이저 버전, CHANGELOG에 Breaking 명시) |
| 2 | 시각 정체성 | brew의 `==>` 단계 헤더는 유지하고, 성공·경고·오류는 `✓` / `Warning:` / `Error:` |
| 3 | `--json` 포함 범위 | §5.9의 데이터 커맨드 전부 |
| 4 | 버전 배너 | 기존 배너 유지 (목업 확인 후 변경, 2026-11-12) |
| 5 | 순서 | 이 가이드를 먼저 적용하고, 동시 설치는 그 위에 구현 |
| 6 | `devices`의 `(64only)` 강조 | 노란색 유지 (경고 의미) |
| 7 | 설치 중 adb 원본 출력 | 성공하면 숨기고, 실패하면 흐리게 표시 |
