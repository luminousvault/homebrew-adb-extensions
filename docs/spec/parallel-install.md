# 여러 단말 동시 설치 설계 (초안)

- 작성: 2026-11-12
- 상태: **구현 완료 (2026-11-12)** — `internal/cli/parallel.go`. 출력 규칙은 `docs/design/cli-output.md`. 선택을 거절한 디바이스는 상태판에 `Skipped`로 표시
- 결정: 진행 표시는 **상태판 + 결과 블록**, 복구 확인은 **필요해지면 바로 묻기 + 대기열** (처음엔 모아서 마지막에 물었으나, 다른 디바이스를 다 기다리는 것이 비효율적이라 변경)

## 1. 적용 범위

| 상황 | 동작 |
|---|---|
| 대상 단말 1대 | **지금과 똑같이 순차 출력** (v1.x와 바이트 단위 동일 유지) |
| 대상 단말 2대 이상, stdout이 터미널 | 상태판 + 결과 블록 + 요약 |
| 대상 단말 2대 이상, stdout이 터미널 아님 (파이프, CI) | 줄 앞에 `[단말]` 접두사를 붙여 실시간 출력 + 요약 |

단말 선택 UI(`-a, --all` 또는 다중 선택)는 바뀌지 않는다.

## 2. 실행 모델

- **단말 사이는 병렬**, 한 단말 안의 APK는 **지정 순서대로 순차** 설치. 앱과 의존 APK처럼 순서가 의미 있을 수 있고, 단말의 패키지 매니저가 어차피 설치 마무리를 직렬화한다.
- 동시 실행 수 제한 없음 (연결된 단말 수만큼). adb 서버가 단말별 연결을 따로 처리한다.
- 단말별 출력은 버퍼에 모은다. 내용은 지금의 단말 블록(`==> Selected device: ...`, `Install command`, adb 출력, 재시도 메시지)과 같다.

## 3. 상태판

```
==> Installing on 2 devices (2 APKs)

  ⠋ Google Pixel 9 Pro    [1/2] app-debug.apk      Installing...
  ✓ Samsung SM-S928N      [2/2] feature.apk        Done (12.4s)
```

- 단말당 한 줄, 그 자리에서 갱신 (100ms 간격, 커서 위로 이동 후 다시 그림).
- 상태: `⠋ Installing...` → `↻ Retrying with -t` → `⏸ Needs confirmation` → `✓ Done (시간)` / `✗ Failed`.
- 줄은 터미널 폭에 맞춰 자른다. 줄바꿈이 생기면 제자리 갱신이 깨지기 때문.
- 커서는 숨기고, 끝나거나 Ctrl+C로 중단하면 복원한다.

## 4. 복구 확인 (바로 묻기, 대기열)

- 설치 중 `[Y/n]` 확인이 필요한 실패(다운그레이드 `-d` 재시도 후 실패, 서명 충돌)가 나면 그 단말은 `⏸ Needs confirmation` 상태가 되고 **그 단말의 남은 APK도 멈춘다**. 뒤 APK가 앞 APK에 의존할 수 있기 때문. 다른 단말은 계속 진행한다.
- 확인이 필요 없는 자동 재시도(testOnly → `-t`, 다운그레이드 → `-d`)는 지금처럼 묻지 않고 진행한다.
- 확인이 필요한 단말이 생기면 **바로** 상태판을 그 자리에 고정하고 묻는다. 다른 단말은 그동안 계속 설치한다(화면에는 질문이 끝난 뒤 상태판을 다시 그릴 때 반영).
- 질문이 떠 있는 동안 다른 단말도 확인이 필요해지면 대기열에 쌓이고, 지금 질문에 답하면 바로 다음 질문이 이어진다. 대기열이 비면 질문 아래에 상태판을 다시 그린다.
- 질문 중 Ctrl+C는 키로 들어오므로(터미널이 SIGINT를 보내지 않음), ak가 자기 프로세스 그룹에 SIGINT를 보내 설치 중인 adb도 멈추게 한다(Windows는 실행 중인 설치가 끝나기를 기다림).

```
Google Pixel 9 Pro — app-debug.apk
  INSTALL_FAILED_UPDATE_INCOMPATIBLE (different signature)
  WARNING: Uninstalling will remove the application data!
  Uninstall and reinstall? [Y/n/a]:
```

- `Y`/Enter: 이 단말만 제거 후 재설치. `n`: 이 단말은 중단. `a`: 이번 실행의 모든 단말에서 남은 확인에 모두 `Y` 적용(대기열에 있는 것 포함). 범위가 보이도록 질문 끝에 흐리게 `(a: yes for every device)`를 붙인다.
- 확인에 답하면 그 단말의 재설치와 남은 APK를 이어서 진행하고(이 단계도 단말 사이 병렬), 질문 아래에 상태판을 새로 그린다. 새 상태판에는 **이어서 진행하는 단말만** 넣는다. 이미 끝난 단말은 위 상태판에 남아 있으므로 다시 그리지 않는다.
- 비터미널(파이프, CI)에서는 물을 수 없으므로 확인이 필요한 항목을 실패로 처리하고 요약에 표시한다.

## 5. 결과 출력

상태판이 끝나면:

1. 상세: 경고나 오류가 있었던 APK만, 단말별·APK별로 묶어 **경고 → 조치 → 결과** 순서로 출력한다. 단말 이름과 APK 이름은 머리글에 있으므로 각 줄은 짧은 문장으로 쓰고(이름을 반복하지 않음), 한 APK의 경고는 한 줄로 합치며, 제거·재설치 단계는 결과 한 줄(`✓ Installed (시간), after uninstalling …`)로 줄인다. 결과(설치)를 앞에, 과정(제거)을 뒤에 둔다. 질문 블록도 같은 짧은 문장을 쓴다. 경고만 덩그러니 남아 왜 나왔는지 헤매지 않도록, 그 뒤에 한 일(제거, 재설치)과 결과(`✓`/`Error:`)를 함께 보여 준다. 문제없이 설치된 APK는 요약에만 나온다.
2. 요약: 단말당 한 줄(`n/m installed`). 다 설치되지 않은 단말 아래에만 실패하거나 실행되지 않은 APK를 적는다. APK 이름을 한 줄에 늘어놓으면 터미널 폭을 넘어 줄이 깨지기 때문.

```
Google Pixel 9 Pro (SERIAL)
  com.example.app.apk
    Warning: Older than the installed version, retrying with -d; downgrade not allowed without uninstalling
    ✓ Installed (43.5s), after uninstalling 'com.example.app' (its data was removed)

Summary
  ✓ Google Pixel 9 Pro  5/5 installed
  ✗ Samsung SM-S928N    4/5 installed
      ✗ feature.apk  INSTALL_FAILED_INSUFFICIENT_STORAGE
```

- 실패가 하나라도 있으면 종료 코드 1 (지금은 설치 실패여도 0 — 단말 1대일 때는 호환을 위해 그대로 둔다).

## 6. 중단 (Ctrl+C)

- 실행 중인 `adb install` 프로세스를 종료하고, 상태판에 `✗ Interrupted`를 표시한 뒤 결과 블록과 요약을 출력하고 `exit 130`.
- 단말에 설치가 반쯤 반영됐을 수 있음을 요약에 표시한다.

## 7. 구조 변경

- `installWithRecovery`가 직접 `say`·`confirmYes`를 호출하지 않도록, 단말별 `installJob`(출력 버퍼, 상태, 확인 요청 채널)을 통해 쓰게 바꾼다. 단말 1대일 때는 버퍼 없이 즉시 출력하는 같은 인터페이스 구현을 쓴다.
- 상태판 렌더링은 `internal/tui`에 `StatusBoard`로 둔다 (상태 → 줄 문자열 변환은 순수 함수로 테스트).

## 8. 검증 계획

- 단위 테스트: 가짜 adb에 단말별 지연·실패를 넣어 병렬 실행 여부(전체 시간), 출력 블록 순서, 확인 큐, `a`(전체 적용), 비터미널 접두사 출력, 종료 코드
- 실기기: 2대에서 동시 설치·서명 충돌 확인 흐름 (테스트 패키지 `com.example.aktest` 사용). 1대에서는 기존 출력과 동일한지 재확인
- pty(`script`)로 상태판 갱신과 커서 복원 확인
