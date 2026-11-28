# Phase 1.6.1 — Clipboard and paste handling

Scope: terminal-native paste in registration fields on `feat/tui-mvp`. No clipboard access or dependency was added. The Phase 1.6 evidence remains unchanged.

## Defect and reproduction

The pinned `charm.land/bubbletea/v2 v2.2.0` translates bracketed terminal input to `tea.PasteMsg` (`input.go`, `paste.go`). Pinned `charm.land/bubbles/v2 v2.2.2/textinput` accepts `tea.PasteMsg` in `Update`. The TUI model previously passed only `tea.KeyPressMsg` to the form and ignored `tea.PasteMsg` in its outer `Update` switch. `TestRegistrationPaste` failed before the fix: the ID field remained empty after a paste of `alpha`. A PTY probe using the pinned Bubble Tea runtime observed bracketed-paste mode enabled and received `/tmp/Sim é/exec` as one `PasteMsg`. Physical Ghostty clipboard behavior remains unverified.

## Fix

The model routes `tea.PasteMsg` to the focused registration textinput only while the form is open. The handler rejects invalid UTF-8, control and format characters, and Unicode line separators before Bubbles can sanitize or insert any part of the payload. Paste outside the form has no action. Existing keyboard shortcuts and field validation remain in place; positional argument syntax is unchanged.

## Acceptance checklist

| ID | Result | Evidence |
|---|---|---|
| PASTE-01 | PASS | Branch and working tree inspected; existing files and unrelated scripts preserved. |
| PASTE-02 | PASS | Pinned source shows `PasteMsg` emission and textinput support; model switch dropped it. |
| PASTE-03 | PASS | New regression failed pre-fix on empty ID after `alpha` paste. |
| PASTE-04 | PASS | Regression exercises ID, executable, work directory, port and added argument field. |
| PASTE-05 | PASS | Regression preserves path spaces, Unicode and positional argument text; PTY probe preserved Unicode payload. |
| PASTE-06 | PASS | Regression rejects newline, carriage return, tab, NUL, escape and Unicode line separator without insertion. |
| PASTE-07 | PASS | Regression checks paste outside form and shortcut-like payload in form cannot trigger mutation or submission. |
| PASTE-08 | PASS | Existing keyboard and mouse behavior retained; full suite result recorded below. |
| PASTE-09 | PASS | Targeted regression and three repeated race runs pass; PTY bracketed-paste event probe passes. |
| PASTE-10 | PASS | Unrestricted `bash scripts/check.sh` exited 0, including Go tests, race, coverage, build and JavaScript checks. |
| PASTE-11 | PASS | User confirmed native Ghostty Ctrl+Shift+V paste works. Ctrl+V was not separately confirmed. |
| PASTE-12 | PASS | TUI guide and this report updated; Git status recorded below. |

## Validation

Pre-change TUI test baseline was blocked in the sandbox by loopback socket denial (`httptest`); the isolated pre-fix regression was run with a temporary Go cache and failed as expected. The sandbox `bash scripts/check.sh` exited 1 due to loopback socket denial in HTTP integration tests; gofmt, vet, JavaScript, build and Git whitespace checks passed. The unrestricted `bash scripts/check.sh` rerun exited 0, including uncached Go tests, race tests, JavaScript tests (10/10), coverage (74.9% default Go statements), build and Git whitespace validation. The final Go source contains 146 top-level tests, retaining the 145-test baseline. `go test -race -count=3 ./internal/tui -run '^TestRegistrationPaste$'` exited 0. The PTY bracketed-paste probe exited 0 with the exact UTF-8 payload. `git diff --check` exited 0. Native CS2 validation is not part of this phase; the integration suite uses simulation.

## Git status

The repository remains on `feat/tui-mvp`. The pre-existing modified documentation and Go module files, untracked TUI files, and unrelated `scripts/download-skins.py` and `scripts/extract-skins.py` remain present. This phase changed `internal/tui/model.go`, `internal/tui/interaction.go`, `internal/tui/tui_test.go`, and `docs/TUI.md`, and added this report. No commit, push, merge or deployment was performed.

## Keyboard regression investigation

Objective: reproduce the reported `n` failure through Bubble Tea and a PTY, identify its cause, and restore registration without weakening connection or mutation guards. Scope is the event dispatcher, registration handlers, and regression validation. This checklist is fixed for the investigation; all results begin PENDING.

| ID | Result | Evidence |
|---|---|---|
| REG-01 | PENDING | User reports failure in connected normal mode. Connected PTY runs with plain `n` and Kitty CSI-u `n` both opened the form; the physical failure was not reproduced. |
| REG-02 | PENDING | No root cause established for the physical Ghostty report. The paste branch is independent of the key branch; see findings below. |
| REG-03 | PASS | User confirmed `n` opens registration while connected; no dispatcher change was made. |
| REG-04 | PASS | Both connected PTY runs opened the form and displayed bracketed-pasted `alpha`; the existing paste regression passes. |
| REG-05 | PASS | Existing shortcut tests pass and the user reported TUI-W03 keyboard acceptance PASS. No individual trace for every shortcut was supplied. |
| REG-06 | PASS | New update-loop regression verifies connecting and pending guards and filter, help and confirmation isolation; existing form tests pass. |
| REG-07 | PASS | All original 146 Go top-level tests remain; 147 now exist. The full Go and 10 JavaScript suites pass. |
| REG-08 | PASS | Connected plain-key and Kitty-key PTY commands, targeted race count=3, and unrestricted `bash scripts/check.sh` exit 0. |
| REG-09 | PASS | User confirmed connected `n` behavior and native Ctrl+Shift+V paste in Ghostty. |

### Findings

The TUI source is untracked on this branch, so `git diff` cannot show a before/after hunk for it. The Phase 1.6.1 change recorded above added a distinct `tea.PasteMsg` case after the unchanged `tea.KeyPressMsg` case. `Update(tea.KeyPressMsg)` still calls `key`, and `key` opens the form for `n` only when `status == CONNECTED`, `pending == ""`, and no form, confirmation, filter or help context has already consumed the key. The paste handler does not alter those guards or route key presses. The user's report specifies CONNECTED normal mode but does not establish the internal pending state or running binary identity. Neither can be inferred from the available evidence.

The first PTY attempt failed to open the form because the isolated daemon policy was invalid (`policy requires 1-16 executables and roots`); the daemon exited and the TUI showed DISCONNECTED. That harness error was corrected by using a valid private policy and waiting for an authenticated HTTP response. With the daemon connected, both a plain `n` byte and Kitty CSI-u `n` opened registration, and bracketed paste was visible. The final plain-key PTY command exited 0; the Kitty-key command exited 0. A previous plain-key run verified opening and paste but its harness quit sequence timed out, so that run is not counted as a passing PTY command. All artifacts are under private `/tmp/cs2m-reg-*` directories.

`TestRegistrationShortcutThroughUpdate` was added to cover the outer Bubble Tea update route, including `n` after a paste outside the form. It passed once and under `-race -count=3`. No production code or dependencies changed in this investigation. Because the reported physical failure could not be reproduced, applying a production fix would be speculative.

The unrestricted `bash scripts/check.sh` exited 0: gofmt, vet, uncached Go tests, all 10 JavaScript tests, race tests, coverage, build and Git whitespace validation passed. Default Go coverage was 75.1%. `git diff --check` exited 0. The repository remained on `feat/tui-mvp` with the same pre-existing modified and unrelated untracked files listed above; this investigation only added a TUI regression test and extended this report. No commit, push, merge or deployment was performed.

## Final user-reported acceptance follow-up

The user reported native Ghostty Ctrl+Shift+V paste PASS, registration through `n` when connected PASS, and successful registration and simulator startup. Ctrl+V was not independently confirmed. The user also reported TUI-W01–W12 PASS; see [Phase 1.6](phase-01-6.md). PASTE-01–PASTE-12 are 12/12 PASS. REG-03, REG-05 and REG-09 move to PASS on this new user evidence, making REG-01–REG-09 7/9 PASS. REG-01 (reproducing the earlier reported `n` failure) and REG-02 (identifying its cause) remain PENDING: neither the PTY tests nor the new physical success reproduces or explains the old failure. This is a historical diagnostic limitation, not a claim of an ongoing observed failure. No production code changed for the keyboard report. Earlier unexpected SSE disconnections also remain unexplained, despite successful explicit reconnection and interactive TUI-W09.

Across Phase 1.6 and 1.6.1 fixed criteria, 55/57 are PASS (24 TUI, 12 manual TUI, 12 paste, 7 keyboard) and two historical keyboard diagnostics remain PENDING. Including the previously reported 162/162 through Phase 1.5 and six APIR criteria from Phase 1.5.1, the accumulated recorded total is 223/225 PASS. Native CS2 validation is separate and remains PENDING. Final closeout checks and Git status follow below.

## Phase 1.7 transition (planning only)

Phase 1.7 should validate native CS2 only after confirming a real installation, host prerequisites, private runtime, operator policy, and a safe disposable server plan. Retain the simulated TUI and API regression baseline. Record native startup, shutdown, restart, logs, isolation, error handling and cleanup separately from simulator results. Investigate long-line log navigation and the earlier unexpected SSE disconnections as separately scoped follow-ups if they recur; do not assume their cause from the current evidence. No Phase 1.7 implementation or native CS2 execution occurred during this closeout.

## Final closeout validation and staging inventory

The final unrestricted `bash scripts/check.sh` exited 0: gofmt, vet, uncached Go tests, 10/10 JavaScript tests, full race tests, coverage, build and Git whitespace validation all passed. The source has 147 top-level Go tests, retaining the prior 146. Default Go statement coverage is 75.1%; coverage is diagnostic, not an acceptance percentage. A separate `git diff --check` exited 0. No native CS2 run occurred. The final branch is `feat/tui-mvp` at `c1b7519`.

Files belonging to the Phase 1.6/1.6.1 staging set: `README.md`, `docs/ARCHITECTURE.md`, `docs/PROJECT_STATUS.md`, `docs/ROADMAP.md`, `docs/SECURITY.md`, `docs/TESTING.md`, `docs/TUI.md`, `docs/phases/phase-01-6.md`, `docs/phases/phase-01-6-1.md`, `go.mod`, `go.sum`, `cmd/cs2m-tui/main.go`, `cmd/cs2m-tui/main_test.go`, `internal/tui/model.go`, `internal/tui/interaction.go`, `internal/tui/view.go`, `internal/tui/sanitize.go`, `internal/tui/token.go`, `internal/tui/tui_test.go`, and `internal/tui/integration_test.go`. The unrelated untracked `scripts/download-skins.py` and `scripts/extract-skins.py` are excluded and untouched. No files were staged, committed, pushed, merged or deployed.
