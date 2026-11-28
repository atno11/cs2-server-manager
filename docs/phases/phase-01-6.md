# Phase 1.6 — TUI MVP

Objective: deliver an independent local administrative TUI through the existing public REST/SSE API. Scope excludes daemon changes, native CS2, future product features and unrelated skin scripts. This fixed checklist was recorded before application changes. Every criterion starts PENDING.

| ID | Fixed acceptance criterion | Status | Evidence |
| --- | --- | --- | --- |
| TUI-01 | Establish the pre-change baseline and preserve all 127 Go and 10 JavaScript regression scenarios. | PASS | Baseline check exit 0; all 127 original Go test names and 10 Node scenarios retained. |
| TUI-02 | Use compatible stable Charm v2 dependencies, pin resolved versions and verify reproducibility. Avoid unnecessary direct dependencies. | PASS | Official stable v2 modules pinned; go mod tidy and go mod verify exit 0; Go 1.27.3. |
| TUI-03 | Provide an independent cmd/cs2m-tui executable. All server administration uses internal/apiclient. No direct access to manager, storage or process supervision internals. | PASS | Separate cmd/cs2m-tui builds; import graph uses apiclient, not manager/storage in production. |
| TUI-04 | Support validated local endpoint configuration and secure loading of the existing admin token. Credentials never appear in terminal output, arguments, URLs or diagnostic messages. | PASS | Private existing token loader and loopback client validation; configuration and unsafe-file tests pass. |
| TUI-05 | Authenticate and accurately represent connecting, connected, reconnecting, disconnected and authentication-failure states. | PASS | Connection state model and authentication/reconnect tests pass; connected follows snapshot only. |
| TUI-06 | Render the authoritative instance snapshot, apply subsequent SSE updates and preserve selection by instance ID rather than row index. | PASS | Snapshot, reorder, removal and stable-ID selection tests pass. |
| TUI-07 | Provide a responsive split layout with a usable compact fallback for smaller terminal dimensions. Rendered content must remain inside the viewport. | PASS | 120x24, 80x20, 60x18 and below-minimum rendering tests enforce viewport cell bounds. |
| TUI-08 | Provide a terminal-adaptive theme with readable light, dark and monochrome rendering. Do not communicate state through color alone. | PASS | Light, dark and monochrome rendering tests preserve text state labels; background message selects accent. |
| TUI-09 | Implement contextual keyboard navigation using Vim keys, arrows, panel focus and documented administrative shortcuts. | PASS | Vim/arrow, focus, filter and contextual form/modal tests pass; controls in TUI.md. |
| TUI-10 | Implement mouse selection, panel focus and scrolling without accidental administrative operations. | PASS | Mouse row selection, compact focus and wheel tests pass; no mouse mutation command exists. |
| TUI-11 | Provide a usable registration form with local validation, server-side error handling and successful registration via the public API. | PASS | Form validation, ordered argument and real HTTP registration tests pass. |
| TUI-12 | Start registered instances and update their displayed state from authoritative SSE events. | PASS | Real HTTP model workflow observes authoritative running SSE state. |
| TUI-13 | Stop and restart instances with explicit confirmation, pending-operation feedback and safe error handling. | PASS | Captured-target confirmation and real HTTP stop/restart workflow pass. |
| TUI-14 | Remove eligible instances with explicit confirmation and correct selection recovery. | PASS | Removal confirmation/cancel and real HTTP removal/selection tests pass. |
| TUI-15 | Display retained and incremental logs using the existing byte-offset recovery contract. Reject stale responses and avoid duplicated or incorrectly associated log bytes. | PASS | Model log overlap, retained fetch, stale response and real HTTP redaction tests pass. |
| TUI-16 | Support log scrolling, top/bottom navigation and explicit follow mode. | PASS | Viewport top/bottom/follow and wheel-scroll tests pass. |
| TUI-17 | Recover from recoverable stream failures using bounded backoff and a fresh authoritative snapshot. Authentication failures require explicit user action. | PASS | Stale generation, auth-failure and stable/short-lived backoff tests pass. |
| TUI-18 | Keep UI updates responsive under network activity. Bound asynchronous operations, reject stale results and prevent unintended duplicate mutations. | PASS | Single pending mutation, bounded contexts, cancellation and race tests pass. |
| TUI-19 | Satisfy TUI-SEC01. Safely render all untrusted terminal content, including ANSI/OSC sequences, control characters and malformed UTF-8. Preserve original sanitized log bytes and offsets. | PASS | Control, OSC, bidi, malformed UTF-8 and boundary tests pass; raw client bytes unchanged. |
| TUI-20 | Cancel client operations, close streams and release resources on exit. Restore terminal state after normal exit and supported interruption paths. Do not stop the daemon or its managed instances. | PASS | Cancellation test, PTY normal quit and stty mode comparison exit 0; daemon independence in integration test. |
| TUI-21 | Demonstrate an end-to-end TUI integration workflow against a real local daemon and simulated servers. | PASS | Real loopback HTTP model test with two simulators plus separate daemon/TUI process PTY smoke exit 0. |
| TUI-22 | Add deterministic unit, model, rendering, interaction, concurrency and integration tests. Preserve existing regression assertions. | PASS | 16 new Go tests cover configuration, model, rendering, interaction, security and real HTTP integration. |
| TUI-23 | Pass applicable dependency, formatting, vet, Go, JavaScript, repeated race, coverage, build and Git whitespace checks. | PASS | Final command table below: tidy, verify, Node, targeted/full race, check, coverage and both builds exit 0. |
| TUI-24 | Document installation/build, configuration, usage, keybindings, security, architecture, acceptance evidence, limitations and Git state. | PASS | TUI.md, this report and README/architecture/status/roadmap/security/testing updated; Git state below. |

## Fixed manual acceptance checklist

| ID | Criterion | Status | Evidence |
| --- | --- | --- | --- |
| TUI-W01 | Launch, authenticate and receive the initial snapshot. | PASS | User reported successful startup, authentication, connection and authoritative snapshot reception. |
| TUI-W02 | Validate split layout, compact fallback and resize. | PASS | User reported final visual acceptance after the layout correction; no new screenshot or measurements supplied. |
| TUI-W03 | Validate keyboard navigation, contextual shortcuts and filtering. | PASS | User reported TUI-W03 PASS and confirmed `n` works when connected. |
| TUI-W04 | Validate mouse selection, focus and scrolling. | PASS | User reported TUI-W04 PASS; no event trace supplied. |
| TUI-W05 | Register two simulated instances. | PASS | User reported TUI-W05 PASS and successful registration. |
| TUI-W06 | Start, Stop and Restart with appropriate confirmation and live state updates. | PASS | User reported TUI-W06 PASS and simulator startup. |
| TUI-W07 | Verify live logs, selection isolation, scrolling and follow mode. | PASS | User reported TUI-W07 PASS; long lines remain horizontally clipped. |
| TUI-W08 | Verify removal confirmation, cancellation and selection recovery. | PASS | User reported TUI-W08 PASS; no separate trace supplied. |
| TUI-W09 | Interrupt connectivity and verify controlled reconnection and authoritative state recovery. | PASS | User reported TUI-W09 PASS; earlier unexpected SSE disconnections remain unexplained. |
| TUI-W10 | Verify safe display of simulated untrusted terminal content. | PASS | User observed OSC, ANSI, BEL and malformed UTF-8 as escaped text and reported unchanged clipboard. |
| TUI-W11 | Validate readable rendering in light, dark and reduced-color terminal configurations. | PASS | User accepted readability; no significant visual difference reported. |
| TUI-W12 | Quit the TUI and verify that the daemon and managed instances remain independent. | PASS | User reported simulator remained running after quitting and reopening the TUI. |

Native CS2 validation remains PENDING.

## Baseline

Branch `feat/tui-mvp`, HEAD `c1b751968905e3cf18628cfb23a2e152e67e1f57`. Before application changes, the tracked tree was clean; only unrelated untracked `scripts/download-skins.py` and `scripts/extract-skins.py` were present. Neither was touched. `bash scripts/check.sh` exited 0: formatting, vet, uncached Go tests, 10 Node tests, race, coverage, build and Git whitespace checks passed. `rg '^func Test' cmd internal | wc -l` counted 127 top-level Go tests. The default Go coverage was 76.3%. `go test -coverpkg=./... -coverprofile=/tmp/cs2m-phase16-baseline.cover -count=1 ./...` exited 0; `go tool cover -func=/tmp/cs2m-phase16-baseline.cover` reported 81.8% cross-package coverage. All 127 original Go test names remain in the current suite.

## Dependency and architecture decisions

The official stable Charm v2 module versions resolved from the Go module registry on Go 1.27.3 are `charm.land/bubbletea/v2 v2.2.0`, `charm.land/lipgloss/v2 v2.1.1` and `charm.land/bubbles/v2 v2.2.2`. All declare Go 1.26 compatibility. They are the only new direct dependencies. `go mod tidy` added the required indirect Charm graph, including clipboard support needed by Bubbles textinput; the SQLite driver remains unchanged. There are no v1 direct imports, unofficial replacements or frontend packages. Official package documentation and local `go doc` were consulted for `tea.View`, mouse messages/mode, background-color messages, Bubbles textinput/viewport and Lip Gloss cell widths.

`cmd/cs2m-tui` loads an existing private token, creates `apiclient.Client`, starts Bubble Tea with a signal-aware context and closes owned resources on exit. `internal/tui` owns model/update/rendering, form, interaction, log presentation and a small production `LiveService` adapter over public `apiclient` operations. Production TUI code imports neither manager nor storage. Rendering contains no network calls; commands carry bounded REST requests or a single pull-based SSE read. The daemon has no TUI dependency.

The screen uses a 40/60 split at 100 columns, a focused one-panel layout from 60–99 columns, and a resize message below 60×18. State is always text-labeled; a terminal background query chooses light/dark header accents and `NO_COLOR` provides monochrome output. Mouse cell-motion reporting supports click and wheel without administration. Bubbles textinput owns editing, and viewport owns selected-log scrolling. The registration form uses one field per positional argument, never a shell parser. Mutations use the captured target ID and one-pending-request policy; no mutation result is applied as newer state than SSE.

The connection model shows CONNECTING, CONNECTED, DISCONNECTED, RECONNECTING and AUTH FAILED. Only a validated initial snapshot makes state authoritative. On stream failure, generation checks reject stale messages, the stream closes and a cancellable 1/2/4/8/16/30-second backoff schedules a fresh subscription. Short-lived successful streams do not reset backoff; a connection stable for at least 30 seconds does. Authentication failures wait for explicit user action. A fresh snapshot restores selection by ID, establishes `BeginLog` before another read, and fetches retained bytes asynchronously. The existing client state queues and reconciles increments by byte offset; stale tickets cannot cross selection, removal, recreation or reconnect. Up to three immediate local recovery attempts precede an explicit `L` retry. There is no automatic mutation retry.

TUI-SEC01 uses a presentation-only sanitizer. It escapes ESC, C0/C1, OSC/CSI/DCS sequences, BEL, carriage return, backspace, malformed UTF-8 and Unicode formatting controls, including bidi marks. The same guard is applied to public error and state text. It keeps newlines, expands tabs, bounds retained input to the existing 64 KiB client buffer and clips by terminal cells. API client bytes, offsets and server redaction remain unchanged. Trusted Lip Gloss styling is applied after untrusted content is escaped.

## Integration and acceptance limits

`TestRealHTTPModelWorkflow` uses a private temporary runtime, a real loopback HTTP listener, the production API handler and manager, and two isolated simulator working directories. It drives the production TUI model and client commands through initial snapshot, two registrations, start, redacted log and retained-log recovery, selection switch, stop, restart, removal and TUI close. It verifies the HTTP service and the second running instance remain after TUI close. It does not launch native CS2. An additional isolated process smoke launched the built daemon binary and TUI binary against a private temporary runtime, waited for the TUI to display CONNECTED over a PTY, sent `q`, and verified the daemon remained alive; the inline Python 3 standard-library harness exited 0 and preserved artifacts at `/tmp/cs2m-phase16-process-_b_c1s7k`. This process smoke did not execute the full administrative workflow; the Go integration test covers those operations. A separate PTY startup/quit smoke with no daemon exited 0. A PTY wrapper compared `stty -g` before and after normal TUI quit and exited 0; this verifies terminal mode restoration in that run, not all terminal emulators or interruption paths. The user confirmed TUI-W01. The supplied screenshot identified layout defects in TUI-W02; corrected rendering awaits user revalidation. Other physical keyboard, mouse and appearance checks were not performed. Exact isolated host-side steps and expected observations are in [TUI.md](../TUI.md).

## Final automated validation

| Command | Exit | Evidence |
| --- | ---: | --- |
| `go mod tidy` | 0 | Only three approved direct Charm v2 modules and their required indirect graph were added. |
| `go mod verify` | 0 | All modules verified. |
| `node cmd/cs2m/ui.test.cjs` | 0 | All 10 original JavaScript scenarios pass. |
| `go test -race -count=3 -timeout=180s ./internal/tui ./internal/apiclient ./cmd/cs2m-tui` | 0 | Repeated targeted model/client race tests pass. |
| `go test -race -count=3 -timeout=180s ./...` | 0 | Every package passes three race runs. |
| `go test -coverpkg=./... -coverprofile=/tmp/cs2m-phase16-final2.cover -count=1 ./...` | 0 | Cross-package coverage command passes. |
| `go tool cover -func=/tmp/cs2m-phase16-final2.cover` | 0 | 78.5% cross-package statement coverage. |
| `bash scripts/check.sh` | 0 | Formatting, vet, uncached Go and Node tests, race, default coverage, build and Git whitespace check pass. |
| `go build -o /tmp/cs2m-phase16-daemon ./cmd/cs2m` | 0 | Daemon builds separately. |
| `go build -o /tmp/cs2m-phase16-tui ./cmd/cs2m-tui` | 0 | TUI builds separately. |
| `git diff --check` | 0 | No tracked whitespace errors. |
| Inline Python 3 standard-library PTY process smoke (isolated daemon/TUI binaries) | 0 | TUI displayed CONNECTED; quitting it left the daemon process alive. |

There are 143 top-level Go tests (all 127 baseline names retained, 16 added) and 10 JavaScript tests. Final default coverage is 74.0% versus baseline 76.3%; cross-package coverage is 78.5% versus baseline 81.8%. Both declines reflect the addition of a new TUI executable and package; coverage remains diagnostic. The Go integration test assembles the production daemon manager and API handler in-process; the separate process smoke verifies independent binaries, authentication and daemon survival, while the separate PTY mode check covers normal terminal restoration. No automated test substitutes for a full physical keyboard/mouse/appearance session. At the original automated closeout, the twelve manual checks were PENDING; the interactive follow-up below supersedes that status. A phase with pending manual checks is implementation-complete and automatically validated, not fully functionally accepted. Native CS2 validation remains PENDING.

Final Git state: branch `feat/tui-mvp` at `c1b751968905e3cf18628cfb23a2e152e67e1f57`, with unstaged edits to `README.md`, `docs/ARCHITECTURE.md`, `docs/PROJECT_STATUS.md`, `docs/ROADMAP.md`, `docs/SECURITY.md`, `docs/TESTING.md`, `go.mod`, and `go.sum`. New phase files are `cmd/cs2m-tui/main.go`, `cmd/cs2m-tui/main_test.go`, `internal/tui/model.go`, `internal/tui/interaction.go`, `internal/tui/view.go`, `internal/tui/sanitize.go`, `internal/tui/token.go`, `internal/tui/tui_test.go`, `internal/tui/integration_test.go`, `docs/TUI.md`, and this report. The two untracked skin scripts remain untouched and excluded. No commit, push, merge, deployment or Phase 1.7 work occurred.

## Interactive layout correction

The user reported TUI-W01 PASS after startup, authentication, connection and authoritative snapshot reception. The supplied native screenshot showed the PORT heading missing, a five-digit port clipped to three digits, no clear boundary between list and detail content, and weak focus indication. TUI-W02 remains PENDING. No timing or network trace was supplied.

The table used fixed ID and state columns inside a 40% panel and clipped the completed row at the panel width. The pre-fix regression `go test -count=1 -run '^TestSplitTableKeepsPortAndPanelBoundaries$' ./internal/tui` exited 1 with `PORT heading clipped`. The corrected table reserves the complete port and state columns first and truncates long IDs by terminal cell width. Both panels now have visible borders, a one-cell separator, and a FOCUSED title marker with stronger border glyphs. Header, section and footer labels are distinct. The panel geometry also drives mouse hit testing, including the separator and Logs/Details title. The compact layout and transparent terminal background remain.

`TestSplitTableKeepsPortAndPanelBoundaries` checks a 100-column split with a long ID and full five-digit port, a compact layout, Unicode cell widths and resized bounds. `TestMouseUsesRenderedPanelBounds` checks row selection, separator behavior, focus and title coordinates. After the fix, `go test -count=1 -timeout=90s ./internal/tui ./cmd/cs2m-tui` exited 0 and `go test -race -count=3 -timeout=180s ./internal/tui ./cmd/cs2m-tui` exited 0. `bash scripts/check.sh` exited 0, including formatting, vet, all Go tests, 10 JavaScript tests, race, 74.6% default coverage, build and Git whitespace checks. A separate `git diff --check` exited 0. There are now 145 top-level Go tests, including the two new rendering regressions; the 127 original scenarios remain. No physical layout revalidation has occurred. These tests verify rendering geometry; TUI-W02 still requires the user's visual recheck in the actual terminal.

Follow-up Git state: `feat/tui-mvp` at `c1b751968905e3cf18628cfb23a2e152e67e1f57`. Previous Phase 1.6 work remains unstaged; this correction edits `internal/tui/view.go`, `internal/tui/interaction.go`, `internal/tui/tui_test.go`, this report, `docs/PROJECT_STATUS.md` and `docs/TESTING.md`. The two unrelated untracked skin scripts remain untouched. No commit, push or merge occurred.

## Final user-reported interactive acceptance

The user subsequently reported TUI-W01 through TUI-W12 PASS (12/12). This supersedes the earlier manual PENDING results above; it is user-reported physical validation, not an automated test run. The user specifically confirmed successful registration and simulator startup, `n` opening registration while connected, escaped OSC/ANSI/BEL and malformed UTF-8 output with an unchanged clipboard, acceptable theme readability without a significant visual difference, and a simulator still running after TUI quit and reopen. The remaining manual criteria were confirmed by the user without separate traces. The original 24 fixed TUI criteria and 12 manual criteria are therefore 36/36 PASS for simulated TUI acceptance. Native CS2 validation remains PENDING and is outside this checklist.

Long log lines remain horizontally clipped; horizontal scrolling or wrapping is a future usability improvement. Earlier unexpected SSE disconnections have no established cause, although explicit reconnection and TUI-W09 passed. See [Phase 1.6.1](phase-01-6-1.md) for paste acceptance and the unreproduced keyboard report. Final checks and Git state for the combined closeout are recorded there.
