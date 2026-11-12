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
* **멀티 디바이스:** 여러 디바이스에 동시 설치·명령 실행 (`-m` 옵션 등)
* **인터랙티브 UI:** APK·디바이스 선택용 터미널 UI (키보드 탐색, 필터링, 정렬)
* **포그라운드 앱 자동 감지:** 패키지명 미지정 시 현재 화면 앱을 자동으로 감지 (`info`, `kill` 등)
* **자동 복구:** `install` 시 testOnly, 버전 다운그레이드, 서명 충돌을 자동 처리
* **adb 자동 탐색 + `doctor`:** 여러 설치 경로에서 adb를 찾고 버전 충돌을 경고
* **Zsh 탭 완성:** 커맨드·옵션 자동 완성
* **구조화된 출력:** 색상·표 형식으로 정보 표시 (v1.x Bash 버전과 바이트 단위로 같은 출력이 목표)

**지원 플랫폼:** macOS, Linux, Windows

## 아키텍처

```
cmd/ak/main.go
  → enableVT() (Windows 콘솔 ANSI 활성화), debuglog.Init()
  → cli.Main(args)
      → 글로벌 옵션(-v, -h) 처리
      → Commands 테이블에서 커맨드 찾기
      → ensureADB() (doctor 제외) → adb.Default = adb.Exec{Path}
      → Command.Run(args)
```

* **커맨드 테이블:** `internal/cli/cli.go`의 `Commands` 슬라이스. 순서가 곧 전체 도움말 순서입니다.
* **종료 처리:** 커맨드는 `exit(code)`로 종료합니다. panic으로 전달되어 `Main`이 복구하므로 defer(터미널 복원, 임시 파일 삭제)가 실행되고 테스트에서 종료 코드를 확인할 수 있습니다. `os.Exit`를 직접 쓰지 않습니다.
* **출력:** `say`/`sayf`(stdout), `sayErr`(stderr)를 씁니다. 테스트가 버퍼로 바꿔 끼웁니다.
* **adb 호출:** `internal/adb`의 `Out`, `Combined`, `Shell`, `Default.Passthrough`를 통해서만 호출합니다. 테스트는 `adbtest.Fake`를 `adb.Default`에 넣습니다.

## 디렉터리 구조

```
adb-extensions/
├── cmd/ak/                     # 진입점 (ldflags로 version, date 주입)
├── internal/
│   ├── cli/                    # 커맨드 구현
│   │   ├── cli.go              # 라우터, Commands 테이블, 도움말·버전, parseLeading
│   │   ├── device.go           # 디바이스 선택(selectDevice, selectDevicesMulti), prettyDevice, pick
│   │   ├── pkg.go              # 포그라운드 앱, 패키지 검증, APK 목록 등 공통 헬퍼
│   │   ├── apps.go             # info, permissions, launch, kill, clear, uninstall, extract
│   │   ├── install.go          # install + 자동 복구
│   │   ├── activities.go, devices.go, mac.go, pull.go, signature.go, doctor.go
│   │   ├── readkey.go          # [Y/n] 한 글자 입력
│   │   ├── cli_test.go         # 가짜 adb로 커맨드 흐름 테스트
│   │   └── completion_test.go  # completions/_ak ↔ Commands 일치 검사
│   ├── adb/                    # adb 실행·탐색(locate.go)·SDK build-tools 탐색(sdk.go)·파싱
│   │   └── adbtest/            # 가짜 adb Runner
│   ├── tui/                    # 인터랙티브 선택 UI (state.go 로직, render.go 화면, keys.go 키 파싱, term.go 터미널)
│   ├── apk/                    # APK 패키지명·서명 인증서 (aapt/apksigner 대체), testdata/aktest.apk
│   ├── style/                  # 색상 상수 (dot import)
│   └── debuglog/               # AK_DEBUG=1 로그
├── completions/_ak             # Zsh completion (수동 관리, 테스트로 동기화 검증)
├── docs/
│   ├── go-migration-plan.md    # Go 전환 계획
│   └── spec/commands.md        # 커맨드 동작 사양 + Bash 대비 의도적 변경점
├── Formula/ak.rb               # Homebrew formula (릴리즈 워크플로가 생성, 직접 수정 금지)
├── .github/formula/ak.rb.tmpl  # Formula 템플릿 (소스 빌드)
├── .github/workflows/          # ci.yml (vet/test/lint/snapshot), release.yml (수동 릴리즈)
├── .goreleaser.yaml            # 멀티 플랫폼 바이너리
├── .golangci.yml
├── VERSION                     # 버전 단일 소스 (릴리즈 입력값과 일치해야 함)
├── CHANGELOG.md, README.md, README.ko.md, CONTRIBUTING.md
```

## 빌드·실행·테스트

```bash
go run ./cmd/ak <command>             # 바로 실행
go build -o ak ./cmd/ak               # 빌드
go test ./...                         # 단위 테스트
go vet ./... && golangci-lint run     # 정적 검사 (CI와 동일)
goreleaser build --snapshot --clean   # 멀티 플랫폼 빌드 확인
```

**실기기 확인:** 테스트는 가짜 adb로 돌기 때문에 디바이스 출력 형식 변화는 잡지 못합니다. 동작을 바꿨다면 0/1/다중 디바이스 환경에서 직접 실행해 확인합니다. 인터랙티브 UI는 실제 터미널(또는 `script`로 만든 pty)에서 확인합니다.

## 신규 커맨드 추가 체크리스트

1. `internal/cli/<name>.go`에 `var cmd<Name> = &Command{...}`와 도움말 함수 작성
   - `Name`, `Desc`(completion 설명), `Usage`/`Help`(전체 도움말 열), `Flags`(허용 옵션), `Run`
   - 일반적인 옵션 처리는 `parseLeading` 사용 (첫 위치 인자에서 멈춤, `-h`는 도움말 후 exit 1)
2. `internal/cli/cli.go`의 `Commands`에 추가
3. `completions/_ak`에 커맨드와 옵션 추가 (`TestCompletionMatchesCommands`가 불일치를 잡음)
4. `cli_test.go`에 가짜 adb 기반 테스트 추가
5. `README.md`, `README.ko.md`, `CHANGELOG.md` 갱신

## 코드 스타일 (Go)

* `gofmt`, `go vet`, `golangci-lint` 통과 필수
* 색상은 `internal/style`을 dot import해서 `Red`, `Green`, `NC`, `Error`, `BArrow` 등으로 씁니다 (ST1001 예외 설정됨)
* 출력 문자열은 v1.x Bash 버전과 같게 유지합니다. 바꿀 때는 `docs/spec/commands.md`의 "의도적으로 바꾼 점"에 기록합니다
* adb 출력 파싱은 순수 함수(`parse...`)로 분리해 테스트합니다
* 테스트 fixture에 실제 시리얼·MAC·SSID 등 기기 식별 정보를 넣지 않습니다 (공개 저장소)

### 색상 상수 (`internal/style`)

| 상수 | 용도 |
|------|------|
| `Cyan` | 정보·Usage 표시 |
| `Green` | 성공 메시지 |
| `Yellow` | 경고 |
| `Red` | 오류 텍스트 |
| `Error` | `Red + "==>" + NC + " " + Bold + "Error:" + NC` 형식의 오류 접두사 |
| `BArrow`, `GArrow` | 파랑/초록 `==>` |
| `Bold`, `Dim`, `NC` | 스타일·리셋 |

## 핵심 함수·타입 레퍼런스

### `internal/adb`
* `Candidates()`, `Locate()` — adb 탐색 순서: `AK_ADB` → `ANDROID_HOME` → `ANDROID_SDK_ROOT` → `PATH` → Android Studio 기본 SDK → `~/.ak/platform-tools`
* `VersionInfo(path)` — 프로토콜·빌드 버전
* `FindBuildTool(name)` — SDK build-tools 최신 버전에서 도구 탐색 (apksigner 폴백용)
* `ListDevices()`, `ReadySerials()`, `ParseGetprop()`

### `internal/cli`
* `selectDevice()` / `selectDevicesMulti(all)` — 단일/다중 디바이스 선택
* `prettyDevice(serial, level)` — `minimal` | `short` | `normal`
* `foregroundPackage(serial)`, `requirePackage(serial, pkg)`, `packageArgOrForeground(serial, args)`
* `apkPathForPackage(serial, pkg)` — base.apk 우선
* `listAPKs(dir, sortByName|sortNewest)`
* `pick(tui.Options)` — 선택 UI 실행, Ctrl+C면 exit 130

### `internal/tui`
* `Select(Options) (Result, error)` — `Multi`, `NoFilter`, `Prompt`, `Location`, `Items`(탭 뒤는 흐린 접미사)
* `Result.Index`(단일) / `Result.Indices`(다중, 선택 순서) — 원본 `Items` 인덱스

### `internal/apk`
* `PackageName(path)`, `Signers(path)` (DN·SHA-256·SHA-1·MD5, apksigner와 같은 값), `HasSourceStamp(path)`

### `install` 자동 복구
`installWithRecovery` → `INSTALL_FAILED_TEST_ONLY`면 `-t` 재시도, `INSTALL_FAILED_VERSION_DOWNGRADE`면 `-d` 재시도 후 `resolveDowngrade`(확인 후 제거·재설치), `INSTALL_FAILED_UPDATE_INCOMPATIBLE`면 `resolveConflict`. `install` 관련 수정 시 이 파일을 우선 참고합니다.

## 디버깅

```bash
AK_DEBUG=1 ak <command>
tail -f "${TMPDIR:-/tmp}/ak-debug.log"    # AK_DEBUG_LOG로 경로 변경 가능
```

로그 형식: `[타임스탬프] [파일:행번호] 메시지`. adb 호출과 선택 UI 결과가 기록됩니다. 코드에서는 `debuglog.Logf(...)`를 씁니다.

## 릴리즈

* 버전: 루트 `VERSION` (단일 소스)
* `CHANGELOG.md`에 `## [x.y.z]` 항목이 있어야 릴리즈됩니다
* GitHub Actions `release.yml` — `workflow_dispatch`로 수동 실행. 테스트 → 소스 아카이브 → `Formula/ak.rb`를 템플릿에서 생성·커밋 → 태그 → GoReleaser로 바이너리와 소스 아카이브를 GitHub Release에 업로드
* Homebrew formula는 소스에서 `go build`합니다 (`depends_on "go" => :build`). `Formula/ak.rb`는 직접 고치지 말고 `.github/formula/ak.rb.tmpl`을 고칩니다

## 사전 요구사항

* **Go** 1.27+ — 개발 시
* **adb** — 실행 시 (유일한 런타임 의존성)
* **apksigner** — 선택. `signature`에서 Go 파서가 실패할 때만 폴백으로 사용

## 상세 가이드

기여 절차, PR 프로세스는 [CONTRIBUTING.md](CONTRIBUTING.md)를 참고합니다.
