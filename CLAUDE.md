# ADB Extensions Kit (ak) — AI 에이전트 컨텍스트

이 문서는 ADB Extensions Kit (`ak`) 프로젝트의 구조, 개발 규칙, 핵심 패턴을 AI 코딩 에이전트가 빠르게 파악하도록 정리한 컨텍스트 파일입니다.

## 커뮤니케이션 규칙

**이 코드베이스에서 작업할 때는 항상 한국어로 응답합니다.** 설명, 커뮤니케이션, 사용자 대면 메시지는 한국어를 사용합니다. 코드 내 주석·문자열은 기존 파일의 언어와 스타일을 따릅니다.

## 프로젝트 개요

**프로젝트명:** ADB Extensions Kit (ak)

**목적:** Android Debug Bridge (ADB) 작업을 단순화하는 통합 CLI 도구. APK 관리, 디바이스 제어, 앱 정보 조회 등을 하나의 진입점(`ak`)으로 제공합니다.

**주요 기술:**
* **Bash Scripting:** 전체가 Bash로 구현되어 macOS, Linux, WSL 등 Unix 계열 환경에서 동작합니다.
* **ADB:** 디바이스 통신·제어의 핵심 의존성입니다.

**주요 기능:**
* **통합 CLI:** 모든 ADB 관련 작업을 `ak <command>` 하나로 실행
* **멀티 디바이스:** 여러 디바이스에 동시 설치·명령 실행 (`-m` 옵션 등)
* **인터랙티브 UI:** APK·디바이스 선택용 터미널 UI (키보드 탐색, 필터링)
* **포그라운드 앱 자동 감지:** 패키지명 미지정 시 현재 화면 앱을 자동으로 감지 (`info`, `pull`, `kill` 등)
* **자동 복구:** `install` 시 서명 충돌, 버전 다운그레이드 등 일반적인 설치 실패를 자동 처리
* **Zsh 탭 완성:** 커맨드·옵션 자동 완성
* **구조화된 출력:** 색상·표 형식으로 정보 표시

**지원 플랫폼:** macOS, Linux, WSL

## 아키텍처

모듈화된 Bash 스크립트(`.sh`)를 빌드 시 단일 실행 파일(`build/ak`)로 병합하는 구조입니다.

### 개발 모드 vs 빌드 모드

| 구분 | 실행 방법 | 라이브러리 로드 | 커맨드 로드 |
|------|-----------|----------------|-------------|
| 개발 모드 | `./src/ak` | `bootstrap.sh`가 `src/lib/`에서 `source` | `load_command`로 동적 로드 |
| 빌드 모드 | `./build/ak` 또는 `ak` | 모든 모듈이 단일 파일에 인라인 병합 | `cmd_*` 함수가 이미 병합됨 |

**실행 흐름:**

```
src/ak (또는 build/ak)
  → bootstrap.sh (개발 모드만, 빌드 시 제거)
  → common.sh, filter.sh, ui.sh, device.sh
  → main() case 분기
  → load_command (개발 모드만)
  → cmd_<command> 실행
```

`bootstrap.sh`는 개발 모드에서 `src/lib/`를 찾아 필수 라이브러리를 로드하고, `load_command`로 `commands/<name>.sh`를 동적으로 `source`합니다. `#@@BUILD_EXCLUDE` 블록 안의 코드(`bootstrap.sh` 호출, `load_command` 등)는 빌드 결과물에서 제거됩니다.

**커맨드 라우팅** (`src/ak`):

- `case` 문에 커맨드명을 **수동 등록**해야 합니다 (`install|pull|info|...`)
- 실행 시 `cmd_${command}` 함수를 호출합니다 (예: `cmd_install`)
- 신규 커맨드 추가 시 `src/ak`의 `case`와 `show_help`도 함께 수정해야 합니다 (파일 병합만으로는 라우팅이 추가되지 않음)

## 디렉터리 구조

```
adb-extensions/
├── src/                          # 소스 코드 (모듈화)
│   ├── ak                        # 메인 진입점 (라우터)
│   ├── bootstrap.sh              # 라이브러리 로더 (개발 모드 전용)
│   ├── VERSION                   # 버전 파일 (현재 단일 소스)
│   ├── completions/
│   │   └── _ak                   # Zsh completion 원본
│   └── lib/
│       ├── common.sh             # 공통 유틸, 색상, 디버그, SDK 도구 탐색
│       ├── filter.sh             # 인터랙티브 UI 필터·하이라이트
│       ├── ui.sh                 # 터미널 인터랙티브 UI
│       ├── device.sh             # 디바이스 감지·선택
│       └── commands/             # 커맨드별 모듈
│           ├── activities.sh
│           ├── clear.sh
│           ├── devices.sh
│           ├── extract.sh
│           ├── info.sh
│           ├── install.sh
│           ├── kill.sh
│           ├── launch.sh
│           ├── mac.sh
│           ├── permissions.sh
│           ├── pull.sh
│           ├── signature.sh
│           └── uninstall.sh
├── build/                        # 빌드 산출물 (gitignore)
│   ├── ak                        # 병합된 단일 스크립트
│   ├── completions/_ak           # 생성된 Zsh completion
│   └── debug.log                 # AK_DEBUG=1 시 로그
├── Formula/
│   └── ak.rb                     # Homebrew formula
├── .github/workflows/
│   └── release.yml               # 수동 workflow_dispatch 릴리즈
├── build.sh                      # 빌드·설치·제거 스크립트
├── CONTRIBUTING.md               # 상세 기여 가이드
├── README.md / README.ko.md      # 사용자 문서
├── CHANGELOG.md                  # 변경 이력
└── LICENSE.md                    # MIT 라이선스
```

## 빌드·설치·실행

### 빌드

```bash
./build.sh
```

수행 내용:
1. `src/lib/` 및 `src/lib/commands/`의 Bash 스크립트를 `build/ak`로 병합
2. 커맨드 소스의 `AK_COMPLETION*` 블록에서 Zsh completion 생성 (`build/completions/_ak`)
3. 기본 구문·버전 검사

### 빌드 스크립트 옵션

| 명령 | 설명 |
|------|------|
| `./build.sh` | 빌드 |
| `sudo ./build.sh --install` | `/usr/local/bin/ak` 및 completion 설치 |
| `sudo ./build.sh --uninstall` | 로컬 설치 제거 |
| `./build.sh --clean` | `build/` 디렉터리 정리 |
| `./build.sh --help` | 도움말 표시 |

### 설치

**Homebrew (권장):**
```bash
brew tap luminousvault/adb-extensions
brew install ak
```

**소스 빌드 후 설치:**
```bash
./build.sh
sudo ./build.sh --install
```

### 실행 예시

```bash
ak install app.apk
ak info com.example.app          # 패키지 생략 시 포그라운드 앱 자동 감지
ak launch com.example.app
ak devices
```

## 개발 워크플로

```bash
# 1. 기능 브랜치 생성
git checkout -b feature/your-feature

# 2. 소스 수정 후 즉시 테스트 (빌드 불필요)
./src/ak <command>

# 3. 병합 결과 검증
./build.sh
./build/ak <command>

# 4. (선택) 시스템 전역 테스트
sudo ./build.sh --install
ak <command>

# 5. 커밋 및 PR
```

**자동화 테스트는 없습니다.** 변경 후 반드시 `./src/ak` → `./build.sh` → `./build/ak` 순으로 수동 검증하고, 가능하면 0/1/다중 디바이스 환경에서 확인합니다.

## 신규 커맨드 추가 체크리스트

1. `src/lib/commands/<name>.sh` 생성
   - `cmd_<name>()` — 커맨드 본체
   - `show_help_<name>()` — 도움말
   - `AK_COMPLETION_DESC` / `AK_COMPLETION` 블록 — Zsh completion
2. `src/ak` 수정
   - `case` 문에 커맨드명 추가
   - `show_help()`에 커맨드 설명 추가
3. `README.md`, `README.ko.md`에 사용법 문서화
4. `./src/ak <name>` → `./build.sh` → `./build/ak <name>` 검증

커맨드 템플릿·상세 절차는 [CONTRIBUTING.md](CONTRIBUTING.md)의 "Adding a New Command" 섹션을 참고합니다.

## 코드 스타일 (Bash)

* **들여쓰기:** 4 spaces (탭 사용 금지)
* **함수명:** `snake_case`
  - 커맨드: `cmd_<name>()`
  - 도움말: `show_help_<name>()`
  - 유틸리티: 설명적 이름 (예: `detect_foreground_package`)
* **전역 상수:** `UPPER_CASE` (예: `RED`, `VERSION`)
* **지역 변수:** `local` 사용
* **따옴표:** 변수는 항상 `"$variable"`로 감싸기
* **조건문:** `[ ]` 대신 `[[ ]]` 선호
* **Bash 3.2 호환:** macOS 기본 Bash를 고려하여 연관 배열 등 Bash 4+ 전용 기능은 지양 (필요 시 배열 순회 등으로 대체)

### 색상 변수 (`common.sh`)

| 변수 | 용도 |
|------|------|
| `CYAN` | 정보·Usage 표시 |
| `GREEN` | 성공 메시지 |
| `YELLOW` | 경고 |
| `RED` | 오류 텍스트 |
| `ERROR` | `"${RED}==>${NC} ${BOLD}Error:${NC}"` 형식의 오류 접두사 |
| `BOLD`, `DIM`, `NC` | 스타일·리셋 |

### 빌드 시스템 마커

| 마커 | 용도 |
|------|------|
| `#@@HEADER_START` / `#@@HEADER_END` | `src/ak` 헤더 메타데이터 추출 |
| `#@@BUILD_EXCLUDE_START` / `#@@BUILD_EXCLUDE_END` | 빌드 결과물에서 제외할 개발 전용 코드 |
| `: <<'AK_COMPLETION_DESC'` / `AK_COMPLETION_DESC` | Zsh completion 커맨드 설명 |
| `: <<'AK_COMPLETION'` / `AK_COMPLETION` | Zsh completion 핸들러 코드 |

## 핵심 함수·변수 레퍼런스

### `common.sh`

* `show_version` — 버전·의존성 배너 출력
* `debug_log`, `debug_init` — 디버그 로깅 (`AK_DEBUG=1` 시)
* `check_adb_available` — ADB 설치 확인
* `detect_android_home`, `find_android_tool` — Android SDK 및 `aapt`, `apksigner` 탐색
* `get_apk_list` — 디렉터리에서 APK 목록 조회·정렬
* `detect_foreground_package` — 현재 포그라운드 앱 패키지명 감지 (다수 커맨드에서 사용)

### `device.sh`

* `pretty_device` — 디바이스 정보 포맷팅
* `find_and_select_device`, `G_SELECTED_DEVICE` — 단일 디바이스 선택
* `find_and_select_devices_multi`, `selected_device[]` — 다중 디바이스 선택
* `find_and_list_devices` — 연결된 디바이스 목록·상태 표시

### `filter.sh`

* `matches_sequential` — 인터랙티브 UI용 순차 문자 매칭 필터 (Bash 3.2 호환)
* `compute_highlight` — fzf 스타일 필터 하이라이트 계산

### `ui.sh`

* `select_interactive` — 터미널 선택 UI (단일/다중, 필터, 정렬, 키보드 탐색)
* `SELECTED_ITEM`, `SELECTED_INDEX` — 단일 선택 결과
* `SELECTED_ITEMS[]`, `SELECTED_INDICES[]` — 다중 선택 결과

### `install.sh` (자동 복구)

설치 실패 시 `resolve_downgrade`, 서명 충돌 시 기존 앱 제거 후 재설치 등 자동 복구 로직이 구현되어 있습니다. `install` 관련 수정 시 이 파일을 우선 참고합니다.

## 디버깅

```bash
AK_DEBUG=1 ./src/ak <command>
# 또는
AK_DEBUG=1 ak <command>
```

* 로그 파일: `build/debug.log` (`src/ak` 실행 시에도 프로젝트 루트의 `build/`에 기록)
* 로그 형식: `[타임스탬프] [파일:행번호] 메시지`
* 코드에서 `debug_log "메시지"` 사용 (`AK_DEBUG=1`일 때만 기록, 일반 실행 성능 영향 없음)

## 릴리즈

* 버전: `src/VERSION` (단일 소스)
* GitHub Actions: `.github/workflows/release.yml` — `workflow_dispatch`로 수동 트리거, semver 형식 (`x.y.z`)
* 변경 이력: `CHANGELOG.md`에 기록

## 사전 요구사항

* **adb** — 필수
* **aapt** — `install` 커맨드
* **apksigner** (`ANDROID_HOME` 필요) — `signature` 커맨드

## 상세 가이드

기여 절차, PR 프로세스, 커맨드 템플릿 전문은 [CONTRIBUTING.md](CONTRIBUTING.md)를 참고합니다.
