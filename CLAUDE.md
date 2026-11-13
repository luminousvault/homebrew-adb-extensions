# ADB Extensions Kit (ak) — AI 에이전트 컨텍스트

이 문서는 ADB Extensions Kit (`ak`) 프로젝트의 구조, 개발 규칙, 핵심 패턴을 AI 코딩 에이전트가 빠르게 파악하도록 정리한 컨텍스트 파일입니다.

## 커뮤니케이션 규칙

**이 코드베이스에서 작업할 때는 항상 한국어로 응답합니다.** 설명, 커뮤니케이션, 사용자 대면 메시지는 한국어를 사용합니다. 코드 내 주석·문자열은 기존 파일의 언어와 스타일을 따릅니다.

## 프로젝트 개요

**프로젝트명:** ADB Extensions Kit (ak)

**목적:** Android Debug Bridge (ADB) 작업을 단순화하는 통합 CLI 도구. APK 관리, 디바이스 제어, 앱 정보 조회 등을 하나의 진입점(`ak`)으로 제공합니다.

**주요 기술:**
* **Go:** 단일 바이너리 (v2.0.0부터. v1.x는 Bash). 외부 의존 라이브러리는 `golang.org/x/term`, `avast/apkparser`, `avast/apkverifier`뿐입니다.
* **ADB:** 디바이스 통신·제어의 유일한 외부 실행 파일 의존성입니다.

**주요 기능:**
* **통합 CLI:** 모든 ADB 관련 작업을 `ak <command>` 하나로 실행
* **멀티 디바이스:** 여러 디바이스에 동시 설치·명령 실행 (`-a/--all` 옵션 등)
* **인터랙티브 UI:** APK·디바이스 선택용 터미널 UI (키보드 탐색, 필터링, 정렬)
* **포그라운드 앱 자동 감지:** 패키지명 미지정 시 현재 화면 앱을 자동으로 감지 (`info`, `kill` 등)
* **자동 복구:** `install` 시 testOnly, 버전 다운그레이드, 서명 충돌을 자동 처리
* **adb 자동 탐색 + `doctor`:** 여러 설치 경로에서 adb를 찾고 버전 충돌을 경고
* **Zsh 탭 완성:** 커맨드·옵션 자동 완성
* **정형화된 출력:** stdout은 결과, stderr은 메시지. `==>`/`✓`/`Warning:`/`Error:`/`Hint:` 다섯 가지 메시지, `--json`, 종료 코드 0/1/2/130 ([docs/dev/cli-design.md](docs/dev/cli-design.md))
* **디바이스 지정:** `-s/--serial`, `ANDROID_SERIAL`, `-a/--all`. 터미널이 아니면 선택 UI 대신 `-s`를 요구
* **여러 디바이스 동시 설치:** 상태판, 확인이 필요하면 바로 묻고 겹치면 대기열 ([docs/dev/cli-design.md](docs/dev/cli-design.md))

**지원 플랫폼:** macOS, Linux, Windows

## 아키텍처

```
cmd/ak/main.go
  → enableVT() (Windows 콘솔 ANSI 활성화), debuglog.Init()
  → cli.Main(args)
      → --no-color 제거, ui.Setup() (터미널·색상 판단)
      → 글로벌 옵션(-v, -h) 처리
      → Commands 테이블에서 커맨드 찾기 → parse() (옵션 위치 무관, --opt=value, --)
      → ensureADB() (doctor 제외) → adb.Default = adb.Exec{Path}
      → Command.Run(*Ctx) error → report(err)가 메시지·종료 코드 결정
```

* **커맨드 정의:** `Command{Name, Summary, Args, Long, ArgHelp, Flags, Examples, Device, JSON, Run}`. 도움말 생성, 옵션 파싱, completion 검증이 모두 이 정의를 씁니다. `Device`(oneDevice/manyDevices)와 `JSON`을 켜면 `-s`, `-a/--all`, `--json`이 자동으로 붙습니다. `Hidden: true`면 전체 도움말과 zsh completion에서 빠집니다(개인용 `verifier`).
* **오류와 종료 코드:** 커맨드는 `error`를 반환합니다. `usageErr`(2), `failErr(hint, …)`(1), `errReported`(이미 출력한 실패, 1), `ui.ErrInterrupted`/`tui.ErrInterrupted`(130). `os.Exit`를 직접 쓰지 않습니다.
* **출력:** 반드시 `internal/ui`를 통합니다. 결과 데이터는 `ui.Out`(stdout, `ui.Table`, `ui.KeyValues`, `ui.JSON`), 메시지는 `ui.Step/Success/Warn/Errorf/Hint/Detail`(stderr). 색은 `ui.Out.Green(...)`처럼 스트림 메서드로만 칠합니다(색 끔이면 자동으로 빠짐).
* **adb 호출:** `internal/adb`의 `Out`, `Combined`, `Shell`, `Default.Passthrough`를 통해서만 호출합니다. 테스트는 `adbtest.Fake`를 `adb.Default`에 넣습니다(동시 호출 안전, `Delay`로 느린 명령 흉내).

## 디렉터리 구조

```
adb-extensions/
├── cmd/ak/                     # 진입점 (ldflags로 version, date 주입)
├── internal/
│   ├── cli/                    # 커맨드 구현
│   │   ├── cli.go              # 커맨드 정의·파서·도움말·버전·오류/종료 코드
│   │   ├── device.go           # Ctx.Device()/Devices(): -s, ANDROID_SERIAL, --all, 선택 UI; deviceName
│   │   ├── pkg.go              # 포그라운드 앱, 패키지 검증(resolvePackage), APK 목록 등 공통 헬퍼
│   │   ├── apps.go             # info, permissions, launch, kill, clear, uninstall, extract
│   │   ├── install.go          # install + 자동 복구 (reporter 인터페이스로 출력)
│   │   ├── parallel.go         # 여러 디바이스 동시 설치 (상태판, 확인 질문 큐)
│   │   ├── activities.go, devices.go, mac.go, pull.go, signature.go, doctor.go
│   │   ├── screenshot.go, record.go, logcat.go, restart.go, wifi.go  # adb 여러 단계를 묶은 커맨드
│   │   ├── cli_test.go, parallel_test.go  # 가짜 adb로 커맨드 흐름 테스트
│   │   ├── completion.go       # ak completion zsh|bash|fish|powershell — 커맨드 정의 + compArgs(인자 종류)에서 생성
│   │   └── completion_test.go · completion_shell_test.go  # _ak = 생성 결과 검사, 설치된 셸(bash · fish · pwsh)에서 실제 Tab 결과 검사
│   ├── ui/                     # 출력 규칙 구현: 스트림·색상 판단, 메시지, 표, JSON, 확인 질문, 스피너, 상태판, Fit
│   ├── adb/                    # adb 실행·탐색(locate.go)·SDK build-tools 탐색(sdk.go)·파싱
│   │   └── adbtest/            # 가짜 adb Runner
│   ├── tui/                    # 인터랙티브 선택 UI (state.go 로직, render.go 화면, keys.go 키 파싱, term.go 터미널)
│   ├── apk/                    # APK 패키지명·서명 인증서 (aapt/apksigner 대체), testdata/aktest.apk
│   ├── style/                  # 선택 UI(tui) 내부 색상 상수
│   └── debuglog/               # AK_DEBUG=1 로그
├── completions/_ak             # `ak completion zsh` 출력(직접 고치지 않음, `make completions`로 재생성 — 테스트가 일치 검사)
├── docs/
│   ├── commands.md             # 명령 레퍼런스 (명령 정의에서 생성: make docs, 테스트가 일치 검사)
│   ├── commands.<lang>.md      # 번역본: docs/i18n/<lang>.json을 넣으면 make docs가 생성. 옵션을 추가하면 번역 JSON에도 넣어야 테스트 통과
│   ├── i18n/ko.json            # 한국어 번역 (ak 바이너리에는 들어가지 않음)
│   ├── troubleshooting.md      # 사용자용 문제 해결
│   └── dev/cli-design.md       # 개발 규칙: 출력 · 오류 · 종료 코드 · 옵션 이름 · 선택 UI · 동시 설치
├── demo/                       # README GIF: vhs tape + 데모용 가짜 adb (실기기 없이 같은 화면을 다시 녹화)
├── Formula/ak.rb               # Homebrew formula (릴리즈 워크플로가 생성, 직접 수정 금지)
├── .github/formula/            # Formula 템플릿 + render.sh (릴리스 바이너리 · checksums로 생성)
├── .github/workflows/          # ci.yml (vet/test/lint/snapshot), release.yml (수동 릴리즈)
├── .goreleaser.yaml            # 멀티 플랫폼 바이너리
├── Makefile                    # 로컬 빌드·설치 (make install / uninstall)
├── .golangci.yml
├── VERSION                     # 버전 단일 소스 (릴리즈 워크플로가 이 값을 씀)
├── CHANGELOG.md, README.md, README.ko.md
```

## 빌드·실행·테스트

```bash
go run ./cmd/ak <command>             # 바로 실행
go build -o ak ./cmd/ak               # 빌드
make install                          # build/ak + completions/_ak를 /usr/local에 설치 (make uninstall로 제거)
go test ./...                         # 단위 테스트
go vet ./... && golangci-lint run     # 정적 검사 (CI와 동일)
goreleaser build --snapshot --clean   # 멀티 플랫폼 빌드 확인
```

**실기기 확인:** 테스트는 가짜 adb로 돌기 때문에 디바이스 출력 형식 변화는 잡지 못합니다. 동작을 바꿨다면 0/1/다중 디바이스 환경에서 직접 실행해 확인합니다. 인터랙티브 UI는 실제 터미널(또는 `script`로 만든 pty)에서 확인합니다.

## 신규 커맨드 추가 체크리스트

1. `internal/cli/<name>.go`에 `var cmd<Name> = &Command{...}` 작성
   - `Summary`(전체 도움말·completion 설명), `Args`, `Long`, `ArgHelp`, `Flags`, `Examples`, `Device`, `JSON`, `Run func(*Ctx) error`
   - 도움말은 정의에서 자동 생성됩니다. 디바이스는 `c.Device()`/`c.Devices()`, 옵션은 `c.Bool/String/Strings`, 인자 개수는 `tooMany(c, n)`
2. `internal/cli/cli.go`의 `Commands`에 추가
3. 인자가 있으면 `completion.go`의 `compArgs`에 종류(패키지 · APK · 폴더 · 단어)를 적고, 값을 고를 수 있는 옵션은 `compValues`에. 그다음 `make completions` (`TestCompletionFileIsGenerated` · `TestCompletionKnowsEveryArgument`가 빠진 것을 잡음)
4. `cli_test.go`에 가짜 adb 기반 테스트 추가
5. `make docs`(docs/commands.md 재생성), `README.md` · `README.ko.md`의 명령 표, `CHANGELOG.md` `[Unreleased]` 갱신
6. 확인: 디바이스 0 · 1 · 여러 대에서 실행, `ak <명령> --help`, Tab 자동완성, 오류 메시지에 고칠 방법(Hint)이 있는지

## 코드 스타일 (Go)

* `gofmt`, `go vet`, `golangci-lint` 통과 필수
* 옵션 이름은 [docs/dev/cli-design.md](docs/dev/cli-design.md)의 옵션 이름 규칙을 따릅니다: 짧은 이름 = 긴 이름 첫 글자, `-s`·`-a`·`-y`·`-h`는 전 커맨드 공통 의미로 예약, 커맨드 항목 전부는 `-e, --every-…`
* 출력은 [docs/dev/cli-design.md](docs/dev/cli-design.md)를 따릅니다: 결과는 stdout, 메시지는 stderr, 메시지 기호 다섯 가지, 실패하면 0이 아닌 종료 코드, 고칠 방법을 알면 `Hint`
* 터미널이 아닐 때(파이프·CI)도 동작해야 합니다: 선택 UI와 확인 질문은 `-s`, `-y`, 인자로 대신할 수 있어야 합니다
* adb 출력 파싱은 순수 함수(`parse...`)로 분리해 테스트합니다
* 테스트 fixture에 실제 시리얼·MAC·SSID 등 기기 식별 정보를 넣지 않습니다 (공개 저장소)

### 메시지 (`internal/ui`, stderr)

| 함수 | 출력 |
|------|------|
| `ui.Step` | `==> 내용` (단계) |
| `ui.Success` | `✓ 내용` |
| `ui.Warn` | `Warning: 내용` |
| `ui.Errorf` | `Error: 내용` (보통은 `failErr`를 반환해 `report`가 출력) |
| `ui.Hint` | `Hint: 내용` |
| `ui.Detail` | 흐린 들여쓰기 (adb 원본 출력 등) |
| `ui.Spin` | 터미널이면 스피너, 아니면 `==>` 한 줄 |
| `ui.Confirm` / `ui.Choose` | `[Y/n]` / `[Y/n/a]` 한 글자 질문 (`-y`면 묻지 않음, 터미널 아니면 `ErrNoInput`) |

## 핵심 함수·타입 레퍼런스

### `internal/adb`
* `Candidates()`, `Locate()` — adb 탐색 순서: `AK_ADB` → `ANDROID_HOME` → `ANDROID_SDK_ROOT` → `PATH` → Android Studio 기본 SDK → `~/.ak/platform-tools`
* `VersionInfo(path)` — 프로토콜·빌드 버전
* `FindBuildTool(name)` — SDK build-tools 최신 버전에서 도구 탐색 (apksigner 폴백용)
* `ListDevices()`, `ReadySerials()`, `ParseGetprop()`
* `Start(args)` — 오래 도는 명령(logcat, screenrecord). 별도 프로세스 그룹이라 터미널 Ctrl+C를 받지 않으므로 호출자가 직접 멈춘다
* `Output(args)` — 바이너리 stdout (screencap)

### `internal/cli`
* `Ctx.Device()` / `Ctx.Devices()` — 대상 디바이스 결정 (-s, ANDROID_SERIAL, --all, 1대, 선택 UI 또는 오류)
* `deviceName(serial)` — "Google Pixel 9 Pro" (getprop 캐시)
* `foregroundPackage(serial)`, `requirePackage(serial, pkg)`, `resolvePackage(serial, args)`
* `apkPathForPackage(serial, pkg)` — base.apk 우선
* `listAPKs(dir, sortByName|sortNewest)`
* `pick(tui.Options, hint)` — 선택 UI, 터미널이 아니면 hint와 함께 실패

### `internal/tui`
* `Select(Options) (Result, error)` — `Multi`, `NoFilter`, `Prompt`, `Location`, `Items`(탭 뒤는 흐린 접미사)
* `Result.Index`(단일) / `Result.Indices`(다중, 선택 순서) — 원본 `Items` 인덱스

### `internal/apk`
* `PackageName(path)`, `Signers(path)` (DN·SHA-256·SHA-1·MD5, apksigner와 같은 값), `HasSourceStamp(path)`

### `install` 자동 복구
`installOne(reporter, …)` → `INSTALL_FAILED_TEST_ONLY`면 `-t` 재시도, `INSTALL_FAILED_VERSION_DOWNGRADE`면 `-d` 재시도 후 확인을 받아 제거·재설치, `INSTALL_FAILED_UPDATE_INCOMPATIBLE`면 확인 후 제거·재설치. 출력은 `reporter`로만 합니다: 1대면 `directReporter`(바로 출력), 여러 대면 `jobReporter`(버퍼 + 상태판, `parallel.go`).

## 디버깅

```bash
AK_DEBUG=1 ak <command>
tail -f "${TMPDIR:-/tmp}/ak-debug.log"    # AK_DEBUG_LOG로 경로 변경 가능
```

로그 형식: `[타임스탬프] [파일:행번호] 메시지`. adb 호출과 선택 UI 결과가 기록됩니다. 코드에서는 `debuglog.Logf(...)`를 씁니다.

## 릴리즈

* 버전: 루트 `VERSION` (단일 소스)
* `CHANGELOG.md`에 `## [x.y.z]` 항목이 있어야 릴리즈됩니다
* GitHub Actions `release.yml` — main에서 수동 실행: `gh workflow run release.yml --ref main` (버전은 `VERSION` 파일, `-f version=x.y.z`는 확인용). 테스트 → 소스 아카이브 → 태그 → GoReleaser가 바이너리·소스 아카이브를 GitHub Release에 업로드 → `dist/checksums.txt`로 `Formula/ak.rb`를 생성해 main에 커밋. main이 아니면 실패합니다(tap은 main의 formula를 읽음)
* Homebrew formula는 GitHub Release의 플랫폼별 바이너리(tar.gz)를 내려받습니다. 사용자 Mac에 Go가 필요 없습니다. cask가 아니라 formula라서 quarantine이 붙지 않습니다. `Formula/ak.rb`는 직접 고치지 말고 `.github/formula/ak.rb.tmpl`(렌더링은 `.github/formula/render.sh`)을 고칩니다

## 사전 요구사항

* **Go** 1.27+ — 개발 시
* **adb** — 실행 시 (유일한 런타임 의존성)
* **apksigner** — 선택. `signature`에서 Go 파서가 실패할 때만 폴백으로 사용

## 상세 가이드

혼자 개발하는 개인 프로젝트입니다. PR은 받지 않고, 버그 제보는 GitHub Issue로 받습니다. 개발 규칙의 자세한 내용과 이유는 [docs/dev/cli-design.md](docs/dev/cli-design.md)에 있습니다.
