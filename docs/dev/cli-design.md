# ak CLI 설계 규칙

ak의 출력·옵션·대화형 UI가 따르는 규칙입니다. 새 커맨드를 만들거나 출력을 바꿀 때 이 문서를 기준으로 삼습니다. 문서와 코드가 다르면 코드가 맞고, 이 문서를 고칩니다.

관련 코드: `internal/ui`(출력), `internal/cli/cli.go`(파서·도움말·종료 코드), `internal/cli/parallel.go`(동시 설치), `internal/tui`(선택 UI).

## 1. 원칙

1. **stdout은 결과, stderr은 그 밖의 모든 것.** 파이프로 넘겼을 때 결과만 남아야 합니다.
2. **색은 의미로만 씁니다.** 초록=성공, 노랑=경고, 빨강=오류, 흐림=부가 정보, 파랑 굵게=단계. 강조용으로 쓰지 않습니다.
3. **터미널이 아니면 꾸밈을 뺍니다.** 색, 스피너, 상태판, 선택 UI를 쓰지 않습니다.
4. **오류는 무엇이 / 왜 / 어떻게 고치는지.** 고칠 방법을 알면 `Hint:`를 붙입니다.
5. **종료 코드로 결과를 알 수 있어야 합니다.** 일부라도 실패하면 0이 아닙니다.
6. **사람이 없어도 동작해야 합니다.** 선택 UI와 확인 질문은 인자·`-s`·`-y`로 대신할 수 있어야 합니다.
7. **모든 커맨드가 같은 틀을 씁니다.** 도움말, 메시지, 표, 빈 줄 모두.

## 2. 스트림과 색상

| 내용 | 스트림 |
|---|---|
| 결과 데이터: 표, `info` 항목, 권한 목록, 서명, MAC, 저장·복사한 경로, `--json`, 도움말, 버전, `doctor` 보고서 | stdout (`ui.Out`) |
| 단계·성공·경고·오류·힌트, 확인 질문, 스피너, 상태판, 선택 UI | stderr (`ui.Err`) |
| adb 원본 출력 (`Performing Streamed Install` 등) | 성공하면 숨김, 실패하면 stderr에 흐리게 (`ui.Detail`) |

- 출력은 반드시 `internal/ui`를 거칩니다. 커맨드 코드에서 `fmt.Print*`나 ANSI 코드를 직접 쓰지 않습니다.
- 색은 스트림별로 판단합니다(`ui.Setup`). stdout을 파이프로 보내도 stderr이 터미널이면 메시지는 색이 남습니다.
- 색 끄기: `NO_COLOR`(값 무관), `--no-color`(어느 위치든), `TERM=dumb`. 강제로 켜기: `CLICOLOR_FORCE`(0이 아닌 값).
- 색은 `ui.Out.Green(...)`처럼 스트림 메서드로만 칠합니다. 색이 꺼진 스트림에서는 자동으로 빠집니다.

## 3. 메시지

| 함수 | 형식 | 색 |
|---|---|---|
| `ui.Step` | `==> 내용` | `==>` 파랑 굵게, 내용 굵게 |
| `ui.Success` | `✓ 내용` | `✓` 초록 |
| `ui.Warn` | `Warning: 내용` | `Warning:` 노랑 |
| `ui.Errorf` | `Error: 내용` | `Error:` 빨강 |
| `ui.Hint` | `Hint: 내용` | `Hint:` 흐림 |
| `ui.Info` | `  내용` (들여쓰기) | 기본 |
| `ui.Detail` | `  내용` (들여쓰기) | 전체 흐림 (adb 원본 출력, 경로, 시간) |

- **기호는 다섯 가지만**: `==>`, `✓`, `Warning:`, `Error:`, `Hint:`. 색을 꺼도 기호로 뜻이 전달됩니다.
- **이름은 작은따옴표로**: 패키지·파일·폴더·경로, 그리고 사용자가 입력한 값(옵션 값, 모르는 옵션·인자, `-s`로 준 시리얼)은 `q()`로 감쌉니다(`Installed 'app.apk' on Google Pixel 9 Pro`, `--repeat must be 1-100: '0'`). 문장과 이름의 경계를 보이게 하려는 것입니다. `q()`는 `ui.Quote`의 별칭이라 `internal/files`도 같은 함수를 씁니다.
  - 디바이스 이름은 사람이 읽는 이름이라 감싸지 않습니다.
  - stdout 결과(저장한 경로 등)는 스크립트가 읽으므로 감싸지 않습니다.
- **한 대상은 어느 메시지에서나 같은 이름으로**: 진행(스피너·`==>`), 성공, 경고, 실패가 같은 대상을 같은 말로 부릅니다. 앱이면 `target.describe`(아래), 파일이면 `'a.txt' to|from <디바이스>`. `✓ Stopped 'pkg' on Google Pixel 9 Pro`이면 실패도 `Error: Couldn't stop 'pkg' on Google Pixel 9 Pro`입니다. 실패만 디바이스가 빠지면 여러 대·여러 앱일 때 무엇이 실패했는지 모릅니다.
- **문장**: 대문자로 시작하고 마침표를 찍지 않으며, 메시지 하나는 한 문장입니다(`Warning:`·`Info`도). 두 생각은 `,`·`;`·`, so`로 잇습니다. 옵션으로 시작하는 문장(`--level must be one of …`, `-s requires a value (ID)`)만 옵션 그대로 소문자로 시작합니다.
- **실패는 `Couldn't <동사> …`** 한 가지입니다: `Couldn't pull 'a.pdf' from Google Pixel 9 Pro`, `Couldn't read the signature of 'pkg' on …: <원인>`. `Could not`·`Cannot`·`can't`·`Failed to`는 쓰지 않습니다. 여러 개 중 일부 실패 요약도 같습니다: `Couldn't pull 2 of 5 files`, `Couldn't install on 1 of 2 devices`.
- **상태 오류는 대상이 주어**입니다: `'x' is not installed on <디바이스>`, `'x' is not a valid package name`, `'x' is not a directory`, `'x' was not found`, `'x' does not exist`, `Device 'X' is unauthorized`. `Not a directory: 'x'`·`Package not installed on …: 'x'`처럼 라벨을 앞에 두지 않습니다.
  - 예외는 "모르는 것" 한 가족으로, 라벨 + 따옴표 이름, 콜론 없이 씁니다: `Unknown command 'x'`, `Unknown option '-x'`(묶음이면 `(in '-ex')`), `Unknown argument 'on' (use off)`, `Unknown shell 'x' (use zsh, …)`. 남는 인자는 `Unexpected argument 'x'`. 받을 수 있는 값을 알면 괄호에 `(use …)`.
- **`Hint:`는 동사로 시작하는 명령문 한 문장**입니다: `Run 'ak info' …`, `Pass -y to create it`, `Turn the screen on: '…'`, `Check the path`. 명령은 작은따옴표로 감쌉니다.
  - 여러 대가 연결돼 있을 수 있으면 명령에 실제 시리얼을 넣습니다(`serialArg`: 준비된 디바이스가 2대 이상이면 ` -s <시리얼>`). 그대로 붙여 넣으면 같은 디바이스에서 돌게 하려는 것입니다.
  - 고칠 방법을 아는 오류에는 모두 `Hint:`를 붙입니다(`failErr(hint, …)`).
  - 고칠 방법이 아닌 설명은 `Hint:`로 쓰지 않습니다. `ui.Info`·`ui.Detail`로 쓰거나 명령문으로 바꿉니다(`Pass -e to show framework and driver logs (…)`).
- **확인 질문도 한 문장**입니다: `Create the missing directory 'x'? [y/N]`, `Uninstall 'pkg' from <디바이스> (removes its data) and reinstall? [Y/n]`.
- **개수는 `ui.Plural(n, "APK")`**: `1 APK` / `3 APKs`, `1 file` / `2 files`. `APK(s)`처럼 쓰지 않습니다.
- **시간은 `ui.Duration`**: 1초 미만은 `120ms`, 그 이상은 `1.2s`(소수 한 자리)이고 `0.0s`는 없습니다. 걸린 시간(`install`, 상태판 `Done`, `extract`, `pull`, `push`, `screenshot`)과 `restart`의 실행 시간(`totalTime`·`waitTime`·min/avg/max)에 씁니다. 예외: `record`의 시간은 녹화 길이라 정수 초(`12s`)입니다. 크기는 `files.HumanSize`(`1.2 MB`)이고 커맨드가 직접 만들지 않습니다.
- **단계에 대한 경고는 그 단계의 `==>` 뒤**에 둡니다. `push`·`pull`의 이름 바꾸기(`Warning: 'a.txt' already exists on …, pushing as 'a (1).txt'`)는 `==> Pushing 'a.txt' to …` 다음, `✓ Pushed …` 앞에 나옵니다. 터미널이 아니면 `==>`가 남으므로 무엇에 대한 경고인지 순서로 보입니다.
- **제목은 대상마다 `==>` 한 줄**: 결과를 보여 주는 커맨드는 대상(앱·디바이스)마다 stderr에 `ui.Step` 제목을 하나 둡니다. stdout만 보면 누구의 결과인지 모르기 때문입니다.
  - **대상은 한 번만 부릅니다**: stdout 결과에 대상 행(`package`·`file`·`serial`, §8)이 있으면 제목은 대상을 다시 부르지 않고 무엇과 어디만 말합니다. 대상 행이 없는 결과(한 줄에 하나인 목록, `✓ Stopped`·`✓ Saved` 같은 동작 메시지, 결과 데이터가 없는 커맨드의 스피너)는 제목이 대상을 부릅니다.
    ```
    ==> App info on Google Pixel 9 Pro (foreground app)        ← stdout 첫 행이 package
      package     com.example.app

    ==> Granted permissions of 'com.example.app' on Google Pixel 9 Pro   ← stdout은 권한 이름만
    android.permission.INTERNET
    ```
  - 대상 행이 있는 곳: `info`(`==> App info on <디바이스>`), `signature`(`==> Signature on <디바이스>`, 스피너 `Pulling the APK from <디바이스>`, 로컬 APK는 `==> Signature of the APK file`), `restart`(`✓ Restarted on <디바이스>`, 스피너 `Restarting on <디바이스> (1/3)`), `sysinfo`(`==> <디바이스>`).
  - 어디는 `on <디바이스>`, 포그라운드 앱을 쓴 경우는 `on <디바이스> (foreground app)`입니다(`target.onDevice`).
  - 앱을 부를 때는 `'pkg' on <디바이스>`, 포그라운드 앱이면 `'pkg' (foreground app) on <디바이스>` 하나만 씁니다(`target.describe`): 스피너(`Extracting …`), `✓ Stopped …`·`✓ Launched …`·`✓ Uninstalled …`, `permissions`·`logcat` 제목, 오류 메시지(`Couldn't stop …`, `… has no launcher activity`, `Couldn't pull the APK of …`).
  - 디바이스: `sysinfo`는 `Device` 섹션 첫 행이 `serial`이라 `==> <디바이스>`만. 숨긴 `mac`은 `serial` 행이 없어 `==> <디바이스> (serial)`, 시리얼은 흐리게. 디바이스 전체를 보는 결과는 `==> <무엇> on <디바이스>`(`==> Activity stack on Google Pixel 9 Pro`).
- **`✓` 뒤 괄호는 `(<디바이스>, 크기, 시간)` 순서**이고, 커맨드가 가진 것만 넣습니다: `✓ Saved 'shot.png' (Google Pixel 9 Pro, 1.2 MB, 0.8s)`, `✓ Pulled 'a.pdf' (Google Pixel 9 Pro, 2 KB, 0.4s)`, 문장이 이미 디바이스를 부르면(`✓ Installed 'app.apk' on Google Pixel 9 Pro (3.2s)`) 괄호에 디바이스를 다시 넣지 않습니다. 여러 대 설치에서 이 문장이 `[디바이스]` 접두사 뒤에도 쓰이기 때문입니다. `record`의 시간은 녹화 길이입니다(`(…, 4.1 MB, 12s)`).
- **빈 결과는 `ui.Info`로 `No …` 문장**(stderr)을 쓰고 exit 0입니다: `No granted permissions`, `No devices connected`, `No devices connected over Wi-Fi`, `No recent files found on <디바이스>`(`pull`), `No activities found on <디바이스>`(`activities`). 찾아봤는데 없는 것은 실패가 아닙니다. stdout은 비워 두고(`--json`이면 `[]`), `(none)` 같은 표시는 쓰지 않습니다.
  - 사용자가 좁힌 조건에 맞는 것이 없으면(`-f`) 실패(exit 1)이고 조건을 고치는 `Hint:`를 붙입니다: `No recent files on … match all of 'x'`, `No APK files match all of 'x'`.
- **빈 줄**: 커맨드 시작과 끝에는 넣지 않습니다. 블록(디바이스 하나, 표 하나, `==>` 제목과 그 아래 목록) 사이에 한 줄을 둡니다. 한 줄짜리 `==>` 단계끼리는 붙입니다.
- **블록 안에서는 짧은 문장**: 머리글에 이미 디바이스·APK 이름이 있으면 각 줄에서 이름을 반복하지 않습니다(§10의 `say`).
- 한 줄로 된 메시지를 미리 만들어야 하면(상태판·접두사 출력) `ui.Line(kind, text)`를 씁니다.

## 4. 오류와 종료 코드

커맨드는 `error`를 반환하고, 출력과 종료 코드는 `report()`(`cli.go`) 한 곳이 정합니다. `os.Exit`를 직접 쓰지 않습니다.

| 반환 | 종료 코드 | 출력 |
|---|---|---|
| `nil` | 0 | 없음 (도움말·버전도 0) |
| `failErr(hint, …)` | 1 | `Error:` + 있으면 `Hint:` |
| `errReported` | 1 | 없음 (이미 출력한 실패. 여러 대상 중 일부 실패 포함) |
| `usageErr(cmd, …)` | 2 | `Error:` + `Hint: Run 'ak <cmd> --help' to see the options` |
| `ui.ErrInterrupted` / `tui.ErrInterrupted` | 130 | 없음 (Ctrl+C) |
| 그 밖의 `error` | 1 | `Error: <err>` |

- 사용법 오류: 알 수 없는 옵션, 값 누락, 값을 받지 않는 옵션에 값, 인자 개수(`tooMany`), 함께 쓸 수 없는 옵션.
- 알 수 없는 커맨드는 exit 2와 `Hint: Run 'ak --help' to see all commands`.
- Ctrl+C(130)는 `report`가 아무것도 출력하지 않습니다. 누른 사람이 이미 알기 때문입니다. 중단 때문에 남은 상태를 알려야 할 때만 커맨드가 그 전에 `Warning:`을 출력합니다(여러 대 설치: `Warning: Interrupted; installs that were running may be incomplete`). `Error: Interrupted`는 쓰지 않습니다.
- **확인 질문을 거절하면** 사용자가 시킨 일을 하지 않은 것이므로 exit 1이고, `Error: Skipped …`로 무엇을 하지 않았는지 말합니다: `install`의 `Skipped 'app.apk' on <디바이스>`, `pull`의 `Skipped: 'dir' was not created`. `Aborted`는 쓰지 않습니다. 터미널이 아니라 묻지 못했으면 `Hint: Pass -y …`를 붙입니다(§7).

```
Error: Multiple devices connected: emulator-5554, emulator-5556
Hint: Pass -s <serial>, or set ANDROID_SERIAL

Error: Unknown option '-x'
Hint: Run 'ak info --help' to see the options

Error: 'com.example.typo' is not installed on Google Pixel 9 Pro
Hint: Open the app and run 'ak info' to see its package name
```

## 5. 도움말

도움말은 `Command` 정의에서 자동 생성합니다. 실제로 받는 옵션과 도움말이 어긋나지 않게 하기 위해서입니다. stdout에 출력하고 exit 0.

```
Show detailed information of a package.        ← Long (없으면 Summary + ".")

Usage:
  ak info [options] [package]                  ← Args

Arguments:                                     ← ArgHelp (없으면 생략)
  package  Package name (default: the foreground app)

Options:                                       ← Flags + 공통 옵션
  -s, --serial ID  Target device (default: $ANDROID_SERIAL, or ask)
      --json       Print the result as JSON
  -h, --help       Show this help

Examples:                                      ← Examples (없으면 생략)
  ak info
```

- 섹션 제목은 굵게. 짧은 이름이 없는 옵션은 4칸 들여 긴 이름을 맞춥니다.
- 공통 옵션은 `Device`·`JSON` 설정으로 붙습니다: `oneDevice` → `-s`, `manyDevices` → `-s`(반복 가능)·`-a`, `JSON` → `--json`, 항상 `-h`.
- `-y`는 자동으로 붙지 않습니다. 확인 질문이 있는 커맨드만 `flagYes`를 `Flags`에 넣습니다(`install`, `pull`).
- `Hidden` 커맨드는 전체 도움말과 자동완성에서 빠집니다. `TabHidden`은 `ak <Tab>` 후보에서만 빠지고 도움말에는 남습니다(§13).

## 6. 옵션 이름 규칙

1. **짧은 이름은 긴 이름 첫 단어의 첫 글자**입니다(`-l, --latest`, `-r, --repeat`, `-p, --port`).
2. **대문자 짧은 이름은 쓰지 않습니다.** Shift가 필요하고 소문자와 헷갈립니다.
3. **예약 이름**은 모든 커맨드에서 뜻이 같고, 다른 뜻으로 쓰지 않습니다.

   | 옵션 | 뜻 |
   |---|---|
   | `-s, --serial` | 디바이스 지정 |
   | `-a, --all` | 연결된 모든 디바이스 |
   | `-y, --yes` | 확인 질문에 예 |
   | `-h, --help` | 도움말 |

4. **커맨드가 다루는 항목 전부는 `-e, --every-<항목>`** 입니다(`install -e, --every-apk`, `activities -e, --every-task`, `logcat -e, --every-tag`). `-a`는 디바이스 전용이라 겹치지 않습니다: `ak install -ea` = 모든 APK를 모든 디바이스에.
5. **짧은 이름이 있으면 긴 이름도 둡니다.** 예외: 항상 켜져 있는 호환 옵션 `install -r`.
6. 예약 이름이 아니면 커맨드마다 같은 글자가 다른 뜻일 수 있습니다(`install -t, --test-only` / `record -t, --time-limit`).

### 파싱 (getopt 규칙)

- 옵션은 인자 앞뒤 어디에 와도 됩니다. `--` 뒤는 모두 인자입니다.
- 값은 `--opt value`, `--opt=value` 둘 다 됩니다.
- 짧은 옵션은 묶을 수 있습니다: `-ea` = `-e -a`, `-tdy` = `-t -d -y`.
- 값을 받는 옵션은 묶음을 끝냅니다. 남은 글자가 값이고, 없으면 다음 인자가 값입니다: `-clW` = `-cl W` = `-c -l W`, `-cl=W`도 같습니다.
- 반복 가능한 옵션은 쉼표로도 나눕니다(`c.Strings`): `-s A,B` = `-s A -s B`.
- `-y`를 파싱하면 `ui.AssumeYes`가 켜집니다.

## 7. 디바이스 지정과 비대화형 실행

`c.Device()`(한 대) / `c.Devices()`(여러 대)가 다음 순서로 정합니다.

| 방법 | 동작 |
|---|---|
| `-s, --serial ID` | 그 디바이스만. `manyDevices`는 반복 가능. 연결·인증 상태를 먼저 확인 |
| `ANDROID_SERIAL` | `-s`가 없을 때. `-a`를 주면 무시 |
| `-a, --all` | 준비된(`device` 상태) 모든 디바이스 |
| 지정 없음 + 1대 | 그 디바이스 |
| 지정 없음 + 여러 대 + 터미널 | 선택 UI (`manyDevices`면 다중 선택) |
| 지정 없음 + 여러 대 + 터미널 아님 | `Error: Multiple devices connected: …` + `Hint: Pass -s <serial> …` (exit 1) |
| 0대 | `Error: No devices connected` (exit 1) |

- `oneDevice` 커맨드에 `-s`를 두 번 주면 사용법 오류(exit 2)입니다.
- "터미널"은 stdin과 stderr이 둘 다 터미널이라는 뜻입니다(`canPick`). stdout은 파이프여도 됩니다.
- 터미널이 아닌데 선택이 필요하면 `pick()`이 힌트와 함께 실패합니다. 고를 것이 하나뿐이면(APK 1개 등) 묻지 않고 씁니다.

### 확인 질문

- `ui.Confirm(q, defaultYes)`: 한 글자로 답합니다(Enter 불필요). 기본값을 대문자로 보입니다: `[Y/n]`(설치 중 제거·재설치), `[y/N]`(`pull`의 디렉터리 만들기). 위험이 큰 쪽을 기본값으로 두지 않습니다.
- `ui.Choose(q, note, …)`: 세 개 이상의 선택지. 첫 번째가 기본값이고, `note`는 키의 범위를 흐리게 덧붙입니다.
- `-y`면 묻지 않고 예(첫 선택지). 터미널이 아니고 `-y`도 없으면 `ui.ErrNoInput`을 받아 추측하지 않고 실패·건너뜀 처리하며 `pass -y`를 안내합니다.
- 질문 중 Ctrl+C는 `ErrInterrupted`(exit 130)입니다.

## 8. 표와 JSON

- `ui.Table`: 머리글은 굵게(흐리게 하지 않음), 세로 테두리 없이 2칸 간격으로 정렬합니다.
  - stdout이 터미널이면 머리글 아래 흐린 `─` 구분선을 긋습니다. 파이프면 구분선이 없어 awk 등으로 읽기 쉽습니다.
- 값에는 색을 쓰지 않습니다. 예외는 상태 열(`connected` 초록, `unauthorized` 빨강, `(64only)` 노랑 등)입니다. 색은 정렬 뒤에 칠합니다(`ui.Cell.Color`).
- 여러 단어인 머리글은 밑줄로 잇습니다(`PLAY_PROTECT_SCAN`). 띄어 쓰면 열이 나뉜 것처럼 보입니다.
- 폭 계산은 `ui.TextWidth`, 맞춤은 `ui.Pad`를 씁니다. 한글·CJK·이모지는 2칸이라 `len`이나 `%-*s`로는 열이 어긋납니다.
- 키-값 목록은 항상 `ui.KeyValues`(`  key  value`: 2칸 들여쓰기, 폭은 가장 긴 키에 맞춤)로 출력합니다. `Printf`로 열을 직접 맞추지 않습니다. 폭은 키로만 계산하므로 값 뒤에 흐린 부가 설명(`(randomized, SSID "…")`)을 붙여도 됩니다.
- 여러 묶음이면 섹션 제목을 0열에 굵게 두고 그 아래 `ui.KeyValues`를 둡니다(`sysinfo`의 `Device`, `signature`의 `Signer #1`, `activities`의 `Task #12`). 한 결과 안의 섹션들은 `ui.KeyWidth`로 구한 가장 긴 키 폭을 `ui.KeyValuesWidth`에 넘겨 값 열을 함께 맞춥니다(`sysinfo`). 블록(섹션, 디바이스, 표) 사이에는 stdout 빈 줄 하나를 둡니다.
- **stdout이 스스로 대상을 밝힙니다.** 키-값·섹션 결과는 첫 행에 대상을 둡니다: `info`·`restart`는 `package`, `signature`는 `package`(로컬 APK는 `file`) 행 뒤 빈 줄, `sysinfo`는 `Device` 섹션 첫 행 `serial`(adb 시리얼. 기기 자체 일련번호는 `Identifiers`의 `hardwareSerial`). 파일로 저장하거나 파이프로 넘겨도 누구의 결과인지 남게 하려는 것입니다. 예외: 한 줄에 하나인 목록(`permissions`)은 grep하기 좋게 이름만 출력합니다.
- **값이 없으면 `-`**(JSON은 `null`)입니다. `n/a` 등 다른 표시는 쓰지 않습니다(`doctor`는 자체 형식). 있을 때만 뜻이 있는 행(`info`의 `pid`, `sysinfo`의 `imei2`)은 없으면 행을 뺍니다.
- **불리언은 텍스트에서 `yes`/`no`**, JSON에서는 `true`/`false`입니다.
- **SDK 표기**: 앱의 SDK는 `34 (Android 14)`, 디바이스의 Android 버전은 `14 (API 34)`. 모르는 API 레벨은 숫자만 씁니다.
- `--json`: `devices`, `info`, `permissions`, `signature`, `sysinfo`, `mac`, `activities`, `restart`, `doctor`. stdout에는 JSON만 나갑니다(`ui.JSON`, 2칸 들여쓰기, HTML 이스케이프 없음).
- JSON 키는 camelCase이고, 버전 사이에 이름과 구조를 유지합니다. 기존 키의 이름을 바꾸거나 지우지 않고 추가만 합니다. `--json`이어도 종료 코드는 텍스트 출력과 같습니다.
- `manyDevices` 커맨드(`sysinfo`, `mac`)의 JSON은 대상이 1대여도 항상 배열입니다. `oneDevice` 앱 커맨드(`info`, `permissions`, `restart`, `signature`)의 JSON 객체에는 `serial`(adb 시리얼)과 `device`(디바이스 이름)가 들어 있습니다. `activities`는 태스크 배열 그대로입니다.

## 9. 진행 표시

- 1초 넘게 걸릴 수 있는 작업(설치, pull, extract, 캡처, 재시작 등)은 `ui.Spin`을 씁니다.
  - stderr이 터미널이면 한 줄 스피너(`⠋ Installing 'app.apk' on …`)를 그리고, 끝나면 지운 뒤 `✓`/`Error:` 줄을 남깁니다.
  - 터미널이 아니면 `==> 내용` 한 줄만 출력합니다. 결과 줄은 그대로 나갑니다.
- **문구는 `<동사>ing <대상> on|to|from <디바이스>`** 이고 전치사를 빼지 않습니다: `Installing 'app.apk' on …`, `Extracting 'pkg' on …`, `Capturing the screen on …`, `Recording the screen on … (0:12, Ctrl+C to stop)`, `Pulling 'a.pdf' from …`, `Pushing 'a.txt' to …`, `Restarting on … (2/3)`(`restart`는 결과 첫 행이 `package`라 앱을 부르지 않음, §3).
  - 반복 횟수는 `(i/N)`이고 N이 1이면 뺍니다. 상태판의 APK 번호도 `(1/2)`입니다.
  - 멈추는 방법은 어디서나 `(Ctrl+C to stop)`, 줄임표는 `…` 한 글자입니다(`...` 아님).
- **단계가 바뀌면 `SetText`, 같은 단계 안의 시계·카운터·덧붙임은 `Update`**:
  - `SetText`는 터미널이 아니면 바뀐 문구를 `==>` 줄로도 출력합니다(직전에 출력한 문구와 다를 때만). `record`의 `Stopping the recording on …` → `Saving the recording from …`, `wifi`의 `Connecting to …`가 로그에 남습니다.
  - `Update`는 출력하지 않습니다. 매초 바뀌는 `record`의 시계, 설치의 `(check the device screen)`처럼 진행 중에 바뀌는 것이 로그를 채우지 않게 하려는 것입니다. 그래서 터미널이 아니면 `record`는 `==> Recording the screen on … (Ctrl+C to stop)` 한 줄만 남깁니다.
- 설치 단계가 **10초를 넘으면** 스피너와 상태판에 흐리게 `(check the device screen)`을 붙입니다(`slowStep`, `Update`). 대개 디바이스가 Play Protect 검사나 USB 설치 허용을 묻고 기다리는 중이기 때문입니다.
- 여러 디바이스 작업은 상태판을 씁니다(§11).

## 10. 선택 UI (`internal/tui`)

- **대체 화면(`\033[?1049h`)에 그립니다.** 닫으면 실행 전 화면과 스크롤백이 그대로 돌아옵니다. 셸 화면을 지우지 않기 위해서입니다.
- 그래서 고른 결과는 닫은 뒤 `==> Selected …` 한 줄(여러 개면 아래 목록 + 빈 줄)로 남깁니다(`printPicked`).
- **stderr에 그립니다.** stdout을 결과용으로 비워 두기 위해서입니다.
- **터미널보다 높게 그리지 않습니다.** 넘치면 빈 줄 → 위치 → 키 도움말 → 필터 테두리 → 개수 → 제목 순으로 덜어내고, 그래도 넘치면 자릅니다. 화면이 스크롤되면 제자리 다시 그리기가 깨지기 때문입니다.
- 각 줄은 `ui.Fit`으로 터미널 폭에 맞춰 자릅니다. 줄바꿈이 생기면 아래 줄이 모두 밀립니다.
- **크기가 바뀌면 바로 다시 그립니다**(`watchResize`).
- 색이 꺼져 있으면 `ui.Plain`으로 색을 뺍니다. `➤`, `[✓]` 기호만으로 포커스와 선택이 보입니다.
- **선택 UI가 이어지면 대체 화면을 한 번만 엽니다**: `install`(APK → 디바이스), `pull`(디바이스 → 파일). `holdPickers()` → `tui.Hold()`로 화면을 유지하고, `release()` → `tui.Release()`로 닫은 뒤 고른 결과를 차례로 출력합니다. 사이에 셸 화면이 깜빡이지 않게 하려는 것입니다.

## 11. 여러 디바이스 동시 설치 (`parallel.go`)

대상이 2대 이상이면 `installParallel`, 1대면 `directReporter`로 바로 출력합니다. `installOne`은 `reporter` 인터페이스로만 출력하므로 두 경우 모두 같은 코드입니다.

### 실행 모델

- **디바이스 사이는 병렬, 한 디바이스 안의 APK는 지정 순서대로 순차.** 앱과 의존 APK처럼 순서가 의미 있을 수 있습니다.
- 동시 실행 수 제한은 없습니다.

### 상태판 (stderr이 터미널일 때)

```
==> Installing 2 APKs on 2 devices
  ⠋ Google Pixel 9 Pro  (1/2) app-debug.apk  Installing…
  ✓ Samsung SM-S928N    (2/2) feature.apk    Done (12.4s)
```

- 디바이스당 한 줄, 100ms마다 제자리에서 다시 그립니다(`ui.Board`). 줄은 `ui.Fit`으로 자르고 커서는 숨깁니다.
- APK가 하나면 번호 `(1/1)`을 뺍니다.
- 상태: 스피너 + 현재 동작(`Installing…`, `Uninstalling…`, `Reinstalling…`) → `⏸ Needs confirmation` → `✓ Done (시간)` / `✗ Failed` / `– Skipped` / `✗ Interrupted`.

### 터미널이 아닐 때

- 상태판 대신 모든 메시지를 일어나는 즉시 `[디바이스 이름] ` 접두사(흐림)를 붙여 출력하고, 끝에 요약을 출력합니다.
- 확인이 필요한 APK는 `-y`가 없으면 `Skipped: uninstalling needs confirmation (pass -y)`로 건너뛰고, 그 디바이스의 다음 APK는 계속 설치합니다(아무도 거절하지 않았으므로).

### 확인 질문: 바로 묻기 + 대기열

- 제거·재설치 확인이 필요한 디바이스는 `⏸ Needs confirmation`이 되고 그 디바이스의 남은 APK도 멈춥니다. 다른 디바이스는 계속 설치합니다.
- **필요해지는 즉시 묻습니다.** 상태판을 그 자리에 두고(`Board.Release`) 아래에 질문을 띄웁니다. 다른 디바이스가 다 끝나기를 기다리면 낭비이기 때문입니다.
- 질문 중에 다른 디바이스도 확인이 필요해지면 대기열에 쌓이고, 답하면 바로 다음 질문이 이어집니다.
- 대기열이 비면 빈 줄 뒤에 상태판을 새로 그립니다. **아직 끝나지 않은 디바이스만** 넣습니다. 끝난 디바이스는 위 상태판에 이미 남아 있습니다.

```
Google Pixel 9 Pro (SERIAL) — 'app-debug.apk'
  Warning: Signature differs from the installed app
  Uninstall 'com.example.app' (removes its data) and reinstall? [Y/n/a] (a: yes for every device)
```

- `Y`/Enter: 이 디바이스만 제거 후 재설치.
- `n`: 이 APK를 건너뛰고 **그 디바이스를 멈춥니다**. 뒤 APK가 이 APK에 의존할 수 있기 때문입니다.
- `a`: 이번 실행의 모든 디바이스에 예(대기열 포함, 이후 질문도). 범위가 보이도록 흐린 `(a: yes for every device)`를 붙입니다.
- 1대 설치는 `[Y/n]`(`ui.Confirm`)로 묻고, 거절해도 다음 APK는 계속 설치합니다.

### Ctrl+C

- 터미널이 SIGINT를 adb에도 보내므로 실행 중인 설치는 스스로 멈춥니다. ak는 `installStop`을 켜서 다음 APK·재시도·제거를 시작하지 않게 합니다.
- 진행 중이던 디바이스는 `✗ Interrupted`로 표시하고(최대 5초 기다림) 상세·요약을 출력한 뒤 `Warning: Interrupted; installs that were running may be incomplete`, exit 130(§4: 130에 `Error:`는 없음).
- 질문 중의 Ctrl+C는 SIGINT가 아니라 키로 들어옵니다. 그래서 ak가 자기 프로세스 그룹에 SIGINT를 보내 실행 중인 adb도 멈춥니다(`interruptGroup`). Windows는 실행 중인 설치가 끝나기를 기다립니다.

### 메시지 두 벌: `say`

모든 설치 메시지는 `say{full, short}` 두 형태로 만듭니다.

| 형태 | 쓰는 곳 | 예 |
|---|---|---|
| `full` | 1대 설치, 비터미널 `[디바이스]` 줄 | `Couldn't install 'app.apk' on Google Pixel 9 Pro: INSTALL_FAILED_…` |
| `short` | 상세 블록·질문 블록 (머리글에 이름이 있음) | `Couldn't install: INSTALL_FAILED_…` |

### 결과 출력

1. **상세 블록**(상태판일 때만): 경고나 오류가 있었던 APK만 디바이스별로 묶습니다.
   - 한 APK의 경고는 한 줄로 합칩니다(`; `로 연결, `joinWarnings`). 그 뒤에 한 일과 결과(`✓`/`Error:`)를 둡니다. 경고만 남아 왜 나왔는지 헤매지 않게 하려는 것입니다.
   - 제거·재설치는 결과 한 줄로 줄입니다: `✓ Installed (43.5s), after uninstalling 'pkg' (its data was removed)`. 결과를 앞에, 과정을 뒤에 둡니다.
   - 문제없던 APK는 마지막에 개수만 셉니다(`✓ 4 other APKs installed`). 디바이스가 거기서 멈춘 것처럼 읽히지 않게 하려는 것입니다.
2. **요약**: 디바이스당 한 줄(`✓ 이름  n/m installed`). 다 설치되지 않은 디바이스 아래에만 실패(`✗`)·건너뜀(`–`)·미실행 APK를 한 줄씩 적습니다. APK 이름을 한 줄에 늘어놓으면 폭을 넘어 깨지기 때문입니다.
3. 마지막 줄: 모두 성공이면 `✓ Installed on N devices`, 아니면 `Error: Couldn't install on X of N devices`(exit 1).

```
Google Pixel 9 Pro (SERIAL)
  com.example.app.apk
    Warning: Older than the installed version, retrying with -d; downgrade not allowed without uninstalling
    ✓ Installed (43.5s), after uninstalling 'com.example.app' (its data was removed)

Summary
  ✓ Google Pixel 9 Pro  5/5 installed
  ✗ Samsung SM-S928N    4/5 installed
      ✗ feature.apk  INSTALL_FAILED_INSUFFICIENT_STORAGE
Error: Couldn't install on 1 of 2 devices
```

## 12. doctor

- flutter doctor 형식입니다. 보고서는 stdout, 마지막 판정 줄은 stderr입니다.
- 범주: `adb`, `Connected devices`, `Shell completion`, `apksigner`. adb가 없으면 `adb`(✗), `Shell completion`, `apksigner`만.

| 표시 | 뜻 | 종료 코드 |
|---|---|---|
| `[✓]` / `•` | 정상 (항목 `•`은 `-v`에서만) | 영향 없음 |
| `[!]` / `!` | 고치면 좋음 (디바이스 없음, 자동완성 없음·오래됨) | 영향 없음 |
| `[✗]` / `✗` | 문제 (adb 없음, 버전이 다른 adb, unauthorized/offline, 연결 안 된 `ANDROID_SERIAL`) | 1 |

- 범주는 가장 나쁜 항목의 수준을 따릅니다. `!`, `✗` 항목은 `-v` 없이도 항상 보입니다. 항목 아래에 흐린 상세와 `Hint:`를 둡니다.
- `apksigner`는 선택입니다. 없어도 `[✓]`(`optional, not installed`)입니다. ak가 서명을 직접 읽기 때문입니다.
- `-v`: 모든 `•` 항목, 범주 사이 빈 줄, 그리고 끝에 **Environment 블록**(ak·OS·셸·adb 버전·디바이스 모델/Android 버전). 버그 리포트에 붙여 넣는 용도라 시리얼은 넣지 않습니다.
- 판정 줄: `✓ N/N checks passed. No issues found!` 또는 `! M/N checks passed. Doctor found issues in K categories.`
- `--json`: `adb`, `versionConflict`, `devices`, `apksigner`, `completion`, `problems`. 종료 코드는 텍스트와 같습니다.
- doctor는 adb를 찾지 못해도 실행됩니다(`NoADB`).

## 13. 자동완성

- `ak completion zsh|bash|fish|powershell`이 커맨드 정의에서 스크립트를 생성합니다. 셸마다 같은 커맨드·옵션·인자 종류를 완성합니다.
- 위치 인자의 종류는 `completion.go`의 `compArgs`에 둡니다(패키지명: 제안 없음, APK 파일·디렉터리, 디렉터리만, 고정 단어, 셸). 옵션 값은 `compValues`(`--serial`, `--level` 등).
- `TabHidden` 커맨드는 `ak <Tab>`의 커맨드 후보에서만 빠집니다. 도움말에는 남고 자기 옵션·인자는 그대로 완성됩니다. `completion`이 그렇습니다: Tab이 된다면 이미 설치한 것이라 후보에 둘 이유가 없습니다(`ak completion <Tab>`은 여전히 셸 이름을 제안).
- `-s` 값은 숨은 커맨드 `ak __serials`로 채웁니다. ak의 adb 탐색을 쓰므로 adb가 PATH에 없어도 됩니다.
- `completions/_ak`는 아카이브와 `make install`용으로 체크인한 zsh 출력입니다. 직접 고치지 않고 생성합니다.
- 테스트가 지킵니다: `TestCompletionFileIsGenerated`(`_ak` = `ak completion zsh`), `TestCompletionMatchesCommands`, `TestCompletionKnowsEveryArgument`(`Args`가 있으면 `compArgs` 항목 필요).
- `doctor`가 사용자의 셸이 읽는 자동완성 파일·rc를 찾아 현재 버전과 비교합니다. 없거나 오래되면 `[!]`와 설치 명령을 보여 줍니다.

## 14. 클립보드와 열기

- `-c, --clipboard`(`screenshot`): 이미지를 클립보드에 복사합니다.
  - **혼자 쓰면 파일을 저장하지 않습니다**(임시 파일을 쓰고 지움). 경로 인자나 `-o`를 함께 주면 저장도 합니다.
  - 클립보드는 하나라서 여러 디바이스와 함께 쓰면 사용법 오류입니다(`choose one device with -s`).
  - macOS: PNG + LZW TIFF를 한 항목으로 씁니다. 클립보드 기록 앱이 TIFF가 있어야 이미지를 기록하기 때문입니다.
  - Linux: Wayland(`WAYLAND_DISPLAY`)면 `wl-copy`, 아니면 `xclip`. 둘 다 없으면 오류. Windows: PowerShell.
- `-o, --open`(`screenshot`, `record`): 저장한 파일을 기본 앱으로 엽니다(`open` / `xdg-open` / `rundll32`). 파일은 이미 저장됐으므로 열기 실패는 `Warning:`입니다.
- 파일을 만드는 커맨드는 저장한 경로를 stdout에 따옴표 없이 한 줄씩 출력합니다(`✓ Saved …` 메시지는 stderr): `screenshot`, `record`, `extract`(로컬 APK), `pull`(받은 파일마다). `push`는 기기 쪽 경로(`/sdcard/Download/x.png`)를 출력합니다. `wifi`는 연결한 주소를 출력합니다.

## 15. `ak files`

대화형 파일 관리자의 규칙 요약입니다. 화면 예시, 실기기 확인 결과, 고른 이유는 [files-design.md](files-design.md)에 있습니다.

- **터미널 전용:** 터미널이 아니면 `ak files needs a terminal` + `Hint: Run 'ak pull' or 'ak push' instead`. 같은 일을 비대화형으로 하는 `pull` · `push`가 짝입니다.
- **화면:** 선택 UI 규칙(§10)을 따릅니다: 대체 화면, stderr, 터미널 높이를 넘지 않기, `ui.Fit`, 크기 변경 시 다시 그리기, 색이 꺼져도 기호로 구분. 위에서부터 머리글 · 문자 키 줄 · 목록 · 클립보드 줄 · 상태 줄 · 번호 키 바입니다. 폭이 되면 기기(왼쪽, 고정)와 컴퓨터(오른쪽) 두 창, 한 창이면 목록을 두 단(mc brief)으로 놓습니다.
- **줄 수는 폭과 높이로만 정합니다.** 복사 중 진행 줄, 질문 · 입력은 상태 줄과 번호 바 첫 줄 자리를 대신 쓰고 줄 수를 바꾸지 않습니다. 목록이 들썩이지 않게 하기 위해서입니다. 클립보드 줄은 비어 있어도 한 줄을 잡아 둡니다. 높이가 모자라면 번호 바 위 빈 줄 → 클립보드 줄 → 문자 키 줄 → 번호 바 → 구분선 → 머리글 순으로 뺍니다.
- **키:** mc의 F키 배치를 숫자로 씁니다(macOS 터미널은 F키를 시스템이 먼저 받음): 1 Sort · 2 Recent · 3 Copy · 4 Cut · 5 To computer/To device · 6 Paste · 7 Mkdir · 8 Delete · 9 Show/Hide computer · 0 Rename. 문자 별칭은 바 · 도움말에 괄호로 보입니다. 지금 쓸 수 없는 키는 라벨을 남기고 흐리게(색이 꺼지면 라벨 앞 `×`) 그리며 눌러도 아무 일이 없습니다. 도움말은 `?`, 끝내기는 `q` · Ctrl+C. 필터 · 이름 입력 중에는 숫자도 글자입니다.
- **5는 바로 반대쪽으로 복사**(넣기 `adb push`, 꺼내기 `adb pull`), 원본은 그대로. 도착지는 반대쪽 창의 현재 폴더입니다.
- **클립보드(3 Copy · 4 Cut · 6 Paste):** ak 안의 클립보드로, 폴더 · 창을 옮겨도 유지됩니다(폴더를 떠나면 선택이 사라지기 때문). 담을 때마다 교체되므로 한 번에 한 폴더의 선택입니다. 붙여넣기는 커서가 있는 창의 현재 폴더로, 기기 ↔ 컴퓨터 또는 같은 쪽 안(`cp -r` · `mv` · 파일 복사)입니다. 잘라내기는 이동: 같은 쪽은 rename, 다른 쪽은 크기 · 파일 수 확인 후에만 원본 삭제(크기를 모르면 원본을 남기고 실패). 같은 폴더에 복사는 ` (N)`, 잘라내기는 무동작, 폴더를 자기 안으로는 거절(링크 · 별칭 포함). Esc는 진행 중 작업 취소 > 필터 끄기 > 클립보드 비우기 순입니다.
- **대상:** 3 · 4 · 5 · 8은 보이는 선택, 없으면 커서 항목. 이름 바꾸기(0)는 최근 보기에서도 됩니다.
- **아무것도 말없이 덮어쓰지 않습니다.** 같은 이름이면 o/s/r(대문자 = 전부)로 묻고, 덮어쓰기는 임시 이름으로 받아 크기를 확인한 뒤 바꿔 넣습니다. 실패 · 취소면 이번에 만든 것만 지웁니다. 이름 비교는 대상 파일 시스템 기준(대소문자 무시 FS에서는 Stat으로 확인)입니다. 이름 바꾸기도 대상이 있으면 실패합니다.
- **색:** 이름 색은 할 수 있는 일로 정합니다(확장자가 아님): 빨강 읽을 수 없음, 보라 링크, 노랑 읽기 전용, 파랑 폴더, 초록 실행 파일, 흐림 숨김.
- **삭제:** 항상 확인([y/N], `This cannot be undone.`). 기기는 `rm -rf`, 컴퓨터도 지금은 영구 삭제입니다. 기기에서 폴더를 지우면 MediaStore에 남는 폴더 행을 `content delete`로 정리합니다(실패는 로그만).
- **끝난 뒤:** 한 일을 stderr에 요약합니다(`✓ Pulled …`, `Warning: Canceled …`, `Error: Couldn't …`). 실패가 있으면 1, Ctrl+C면 130입니다.
