# ak Go 전환 계획 (초안)

- 작성: 2026-11-12
- 근거: Obsidian `100. Raw/개발환경/대화/2026-11-12 ak Go 전환과 adb 의존성 처리 검토`
- 상태: **확정 (2026-11-12) — §6 권장안 채택, 브랜치 `feature/go-migration`에서 진행**

## 1. 목표와 성공 기준

| 목표 | 성공 기준 (검증 방법) |
|---|---|
| Bash(약 5,100줄, 13개 커맨드)를 Go 단일 바이너리로 교체 | 13개 커맨드가 같은 이름·옵션·주요 출력으로 동작 (커맨드별 대조 체크리스트 통과) |
| 외부 도구 의존 축소 | `awk`/`sed`/`tput`/`stty` 의존 0, `aapt`·`apksigner` 없이 `install`·`signature` 동작 |
| adb 미설치·경로 미설정 사용자도 바로 사용 | 정해진 순서로 adb 자동 탐색, `ak doctor`가 사용 경로·버전·충돌을 표시 |
| 배포 자동화 유지 | 태그 1회로 macOS(arm64/amd64)·Linux 바이너리 + Homebrew tap 갱신 (goreleaser) |
| 기존 zsh completion 동작 유지 | `_ak` 내용 동일, 같은 경로(`zsh_completion`)에 설치, Go 커맨드·플래그와 일치 테스트 통과 |
| 자동화 테스트 확보 (현재 0개) | adb 출력 샘플 기반 파싱 단위 테스트가 CI에서 실행 |

비목표(1차): adb 프로토콜 직접 구현, adb 바이너리 내장(`go:embed`) — 라이선스·adb 서버 버전 충돌 문제로 제외.

## 2. 현재 구조 → Go 패키지 대응

| Bash | 줄 수 | Go 대응 | 비고 |
|---|---|---|---|
| `src/ak` (라우터, `case` 수동 등록) | 108 | `cmd/ak/main.go` + cobra 루트 | 커맨드 등록이 코드로 강제됨 |
| `common.sh` (색상, 디버그, SDK 탐색, foreground 감지) | 398 | `internal/adb`(실행·탐색), `internal/log`, `internal/style` | `AK_DEBUG=1` 유지 |
| `device.sh` (단일/다중 선택, 목록) | 369 | `internal/device` | `-m` 다중 디바이스 |
| `ui.sh` + `filter.sh` (인터랙티브 선택 UI) | 1,206 | `internal/tui` (bubbletea 예정) | 가장 큰 덩어리, 키 조작·필터·정렬·하이라이트 재현 필요 |
| `commands/*.sh` | 2,418 | `internal/cmd/<name>.go` | 13개 |
| `AK_COMPLETION` 블록 → `_ak` 생성 | — | 기존 zsh `_ak`를 `completions/_ak`로 그대로 유지·설치 | **요구사항: 기존 completion 동작 유지.** cobra 기본 `completion` 커맨드는 끔. 커맨드·플래그와 `_ak` 일치 여부를 테스트로 검증 |
| `build.sh` + `release.yml` | 605+ | `Makefile`(선택) + `.goreleaser.yaml` + workflow | |

외부 도구 대체:

| 도구 | 현재 사용처 | Go 전환 후 |
|---|---|---|
| `awk`/`sed`/`grep`/`cut`/`tr` | 출력 파싱 전반 | 표준 라이브러리 |
| `tput`/`stty` | `ui.sh` | 터미널 라이브러리 (Windows 대응 포함) |
| `aapt` | `install` (패키지명 추출) | Go APK 매니페스트 파서 (예: `shogo82148/androidbinary`) — **채택 전 실제 APK로 검증** |
| `apksigner` | `signature` (SHA-256 인증서 다이제스트) | Go APK 서명 파서 (예: `avast/apkverifier`, v1~v3 지원) — **검증 후 채택, 실패 시 apksigner 탐색으로 폴백** |
| `adb` | 전부 | 유지 — 탐색 순서로 찾기 (§4) |

## 3. 전환 방식

**권장: 별도 브랜치에서 전체 이식 → 동등성 확인 후 v2.0.0으로 교체 릴리즈.**

- 규모가 13개 커맨드로 작아 Bash·Go 혼합 단계(Go가 내장 Bash를 호출하는 방식)의 이득보다 복잡도(빌드 2종, 배포물 2종)가 크다.
- 이식 기간에도 `main`의 Bash 버전은 그대로 릴리즈 가능.
- 대안(혼합 방식)은 "결정 필요 1"에서 선택.

## 4. adb 탐색 설계

처음 발견한 것을 사용하는 함수 하나로 처리한다 (설치 출처를 구분하지 않음).

1. `AK_ADB` 환경변수
2. `ANDROID_HOME` / `ANDROID_SDK_ROOT` 의 `platform-tools/adb`
3. `PATH` (Homebrew cask `android-platform-tools` 포함)
4. Android Studio 기본 위치 — macOS `~/Library/Android/sdk`, Linux `~/Android/Sdk`, Windows `%LOCALAPPDATA%\Android\Sdk`
5. ak 캐시 `~/.ak/platform-tools/adb`
6. 없으면 설치 안내 (macOS: `brew install --cask android-platform-tools`, 그 외: 공식 zip 링크) — 자동 다운로드는 "결정 필요 3"

`ak doctor`: 사용 중인 adb 경로·버전, 발견된 다른 adb 목록, 버전 불일치 경고(adb 서버 충돌 위험) 표시.

## 5. 단계별 계획

### Phase 0 — 동작 사양 고정 (Go 코드 작성 전)
- 커맨드별 입력(인자·옵션)·출력·종료 코드·실패 시 동작을 `docs/spec/<command>.md`로 정리
- 실제 디바이스에서 커맨드가 호출하는 adb 출력 수집 → `testdata/` (예: `adb devices -l`, `dumpsys window`, `pm list packages`, `dumpsys package <pkg>`, 설치 실패 메시지들)
- → 검증: 13개 사양 문서와 샘플이 존재하고, 각 샘플에 대응 커맨드가 명시됨

### Phase 1 — 골격과 배포 파이프라인
- `go.mod`, cobra 루트, `--version`/`--help`, `internal/adb`(exec 래퍼를 interface로 두어 테스트에서 가짜 adb 주입)
- adb 탐색(§4) + `ak doctor`
- `.goreleaser.yaml`, CI(`go test`, `go vet`, `golangci-lint`), 브랜치에서 snapshot 빌드
- → 검증: `ak --version`, `ak doctor`가 3개 adb 설치 형태(Studio/Homebrew/ANDROID_HOME)에서 올바른 경로를 표시, CI 녹색

### Phase 2 — 공통 기반
- `internal/device`: 디바이스 목록·상태 파싱, 단일/다중 선택
- `internal/tui`: `select_interactive` 재현 (단일/다중, 순차 문자 필터, 하이라이트, 정렬, 키 탐색)
- `detect_foreground_package` 이식
- → 검증: 파싱 단위 테스트 통과, 0/1/다중 디바이스에서 선택 UI 수동 확인

### Phase 3 — 커맨드 이식 (단순 → 복잡)
1. `devices`, `info`, `launch`, `kill`, `clear`, `uninstall`
2. `permissions`, `activities`
3. `pull`, `extract`, `mac`
4. `signature` (Go 서명 파서 검증 포함)
5. `install` (APK 선택 UI, `-m`/`-t`/`-d`/`-r` 옵션, `resolve_downgrade`·`resolve_conflict` 자동 복구, Go 매니페스트 파서)
- → 검증: 커맨드마다 Phase 0 사양 대조 체크리스트 + 파싱 테스트, Bash 버전과 같은 디바이스에서 결과 비교

### Phase 4 — 배포 전환과 문서
- Homebrew tap(`luminousvault/homebrew-adb-extensions`) 배포 방식 전환 (formula 또는 cask, "결정 필요 2")
- `README.md`/`README.ko.md`/`CONTRIBUTING.md`/`CLAUDE.md`/`CHANGELOG.md` 갱신, Bash 소스·`build.sh` 제거
- v2.0.0 릴리즈
- → 검증: 깨끗한 macOS에서 `brew install` → completion·`ak doctor`·주요 커맨드 동작

### Phase 5 (선택) — adb 자동 설치, Windows 공식 지원

## 6. 결정 (권장안으로 확정)

| # | 항목 | 권장 | 대안 |
|---|---|---|---|
| 1 | 전환 방식 | 브랜치에서 전체 이식 후 v2.0.0 교체 | Go가 미이식 커맨드를 내장 Bash로 위임하며 점진 교체 |
| 2 | Homebrew 배포 형태 | formula + caveats로 adb 설치 안내 (Linuxbrew도 지원) | cask + `depends_on cask: "android-platform-tools"` (macOS 전용, adb 자동 설치) |
| 3 | adb 자동 다운로드(`~/.ak/`) | v2.0.0에서는 안내만, 이후 Phase 5 | v2.0.0에 포함 (사용자 확인 후 공식 zip 다운로드) |
| 4 | Windows 지원 범위 | 빌드는 만들되 공식 지원은 Phase 5 | v2.0.0부터 공식 지원 |
| 5 | aapt/apksigner 대체 | Go 라이브러리 검증 후 채택, 실패 시 기존 도구 탐색 폴백 | 기존 도구 탐색 유지 |

## 7. 리스크

- **인터랙티브 UI 재현 비용**: `ui.sh` 1,086줄의 세부 키 동작. Phase 0에서 키 바인딩·동작 목록을 먼저 고정.
- **adb 서버 버전 충돌**: 다른 버전 adb를 쓰면 Android Studio와 서버를 서로 재시작. 탐색 순서상 Studio/SDK를 캐시보다 우선하고 `doctor`로 경고.
- **Go APK 파서 정확도**: split APK, 최신 서명 방식(v3.1/v4)에서 `apksigner`와 결과가 다를 수 있음 → 실제 APK 샘플로 대조.
- **기존 사용자 호환**: 커맨드명·옵션·completion 유지, 출력 형식 변경은 CHANGELOG에 명시.
