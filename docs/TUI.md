# Local TUI

`cs2m-tui` is the first official interactive client. It uses the existing authenticated REST and SSE API through `internal/apiclient`; the headless daemon does not depend on it. The TUI never starts or stops the daemon. Quitting it does not stop managed instances.

## Build and start

Go 1.27 and the pinned Charm v2 modules are required to build. From the repository root:

```sh
go build -o /tmp/cs2m-tui ./cmd/cs2m-tui
/tmp/cs2m-tui --help
CS2M_DATA=/path/to/private/data /tmp/cs2m-tui --addr http://127.0.0.1:8080
```

The default endpoint is `http://127.0.0.1:8080`. `--addr` accepts only an explicit numeric loopback HTTP endpoint; the API client rejects remote hosts, URL credentials and extra URL components. The default credential path is `$CS2M_DATA/admin.token`, or `~/.local/share/cs2m/admin.token` when `CS2M_DATA` is unset. `--token-file /absolute/private/path/admin.token` selects an existing credential. The TUI does not create a token. The credential and its directory must be owned by the current user and inaccessible to group or others; the final file must be regular, 0600 or stricter, and cannot be a symlink. Keep the token out of command arguments, URLs and terminal output. The daemon must already be running.

## Screen and controls

At 100 columns or wider, the instance list and details/logs use an approximate 40/60 split. From 60 through 99 columns, the focused panel fills the screen; `Tab` switches panels. At less than 60 columns or 18 rows, a resize message appears and quitting remains available. The terminal background query chooses light or dark foreground accents; `NO_COLOR=1` uses a monochrome presentation. State names are always printed as text. Use a terminal with ordinary Unicode cell support; no special font is required.

| Context | Keys or mouse | Action |
| --- | --- | --- |
| Normal | `q`, `Ctrl+C` | Quit the TUI only. |
| Normal | `Tab`, `Shift+Tab` | Switch list and detail/log focus. |
| Normal | `?`, `Esc` | Show or dismiss help. |
| List | `j`/`k`, Down/Up | Select next/previous visible instance. |
| List | `/` | Edit the ID filter; `Enter` keeps it, `Esc` clears it. |
| List | `Enter` | Focus the detail/log panel. |
| Detail | `v` | Switch Logs and Details. |
| Logs | `j`/`k`, Down/Up, PageDown/PageUp | Scroll retained output; scrolling up pauses follow. |
| Logs | `g`, `G`, `f` | Top, bottom/resume follow, toggle follow. |
| Connected normal mode | `n`, `s`, `x`, `r`, `d` | Register, start, confirm stop, confirm restart, confirm removal. |
| Recovery | `R`, `L` | Explicit reconnect; refetch selected retained logs. |
| Confirmation | `y`, `n`/`Esc` | Confirm or cancel. Other keys cannot trigger administration. |
| Registration | `Tab`/`Shift+Tab`, `Enter`, `Esc` | Navigate fields, advance/submit from last field, cancel. |
| Registration | `Ctrl+A`, `Ctrl+D` | Add one positional argument field; remove the current argument field. |
| Mouse | Click row/panel, wheel | Select instance, focus panel, switch Logs/Details from its title, scroll list or logs. |

Mouse reporting can affect ordinary terminal text selection; use the terminal's modifier key for copy/paste selection if needed. Every mouse action is nonadministrative. No workflow requires a mouse.

The registration form accepts ID, executable path, working directory, port and up to 32 ordered positional arguments, one per field. It never interprets a shell command. Terminal paste, including bracketed paste, inserts into the focused field; a paste with control characters, line separators or invalid UTF-8 is rejected as a whole. Paste does not act as a shortcut or submit the form. Path spaces and Unicode are preserved. Local checks enforce documented sizes, absolute clean paths, ID/port syntax and argument characters; the server remains authoritative for executable trust, execution policy and uniqueness. A rejected registration keeps the form open. Stop, restart and remove require `y` confirmation and capture the original target ID. A single pending mutation prevents conflicting duplicates. The TUI waits for authoritative SSE changes rather than inserting optimistic state. Network mutation failures have an ambiguous outcome; inspect the fresh snapshot or reconnect before retrying.

## Connection and logs

The header displays CONNECTING, CONNECTED, DISCONNECTED, RECONNECTING or AUTH FAILED. CONNECTED appears only after an authoritative snapshot. Stream failures retain the prior view marked stale, close the old stream and retry with bounded backoff (1, 2, 4, 8, 16 and at most 30 seconds). Short-lived connections do not reset backoff; a connection lasting at least 30 seconds does. Authentication failure stops automatic retry. `R` is an explicit retry with the already loaded token; restart the TUI to load an updated token file. Administrative mutations are disabled until the view is authoritative.

The selected log is loaded from the 64 KiB retained REST snapshot, then reconciled with incremental SSE bytes using `apiclient.State` tickets and byte offsets. A new stream snapshot establishes the log recovery ticket before the next event read. Selection changes, removal, same-ID recreation and reconnect invalidate stale responses. Gaps, backlog and failed retained-log requests trigger at most three immediate recovery attempts; `L` retries explicitly afterward. Logs older than the server's retained ring cannot be recovered. The TUI does not persist logs.

The server redacts known secrets, but sanitized output remains untrusted terminal content. The TUI renders a separate presentation copy that visibly escapes ESC, ANSI/OSC and other controls, malformed UTF-8 and Unicode formatting controls. It retains the original sanitized bytes and offsets in the API client. Unknown secret values remain outside the server's redaction guarantee.

## Isolated host-side interactive validation

These commands create a **new** private temporary runtime and preserve it afterward. Run from the repository root in an Arch Linux shell. Choose a free loopback port; the example uses `18081` and checks it before startup.

```sh
smoke_dir=$(mktemp -d /tmp/cs2m-tui-smoke.XXXXXX)
chmod 700 "$smoke_dir"
printf 'smoke_dir=%s\n' "$smoke_dir"
mkdir -m 700 "$smoke_dir/data" "$smoke_dir/work-a" "$smoke_dir/work-b"
cat > "$smoke_dir/simulator.sh" <<'SH'
#!/bin/sh
trap 'exit 0' TERM INT
n=0
while :; do
  n=$((n+1))
  printf 'sim:%s seq:%d secret=S3CRET\n' "$PWD" "$n"
  if [ $((n % 5)) -eq 0 ]; then printf '\033]52;c;Zm9v\007\033[2J malformed:\377\n'; fi
  sleep 1
done
SH
chmod 700 "$smoke_dir/simulator.sh"
printf '{"executables":["%s/simulator.sh"],"executable_roots":["%s"],"work_roots":["%s"],"base_args":[],"known_secrets":["S3CRET"]}\n' "$smoke_dir" "$smoke_dir" "$smoke_dir" > "$smoke_dir/data/policy.json"
chmod 600 "$smoke_dir/data/policy.json"
ss -H -ltn '( sport = :18081 )'
go build -o "$smoke_dir/cs2m" ./cmd/cs2m
go build -o "$smoke_dir/cs2m-tui" ./cmd/cs2m-tui
CS2M_DATA="$smoke_dir/data" CS2M_ADDR=127.0.0.1:18081 "$smoke_dir/cs2m" > "$smoke_dir/daemon.log" 2>&1 &
daemon_pid=$!
```

The `ss` command must print **no listening socket**. If it does, choose another unused loopback port and use it in both launch commands. Wait for `admin.token` to appear; the daemon log should state that it is listening. The token stays in the private runtime and is loaded directly by the TUI. In a second terminal, assign `smoke_dir` to the path created above, then start the TUI:

```sh
CS2M_DATA="$smoke_dir/data" "$smoke_dir/cs2m-tui" --addr http://127.0.0.1:18081
```

Register `alpha` with executable `$smoke_dir/simulator.sh`, work directory `$smoke_dir/work-a`, port `27015`, no arguments. Register `beta` with the same executable, work directory `$smoke_dir/work-b`, port `27016`, no arguments. Enter the expanded absolute paths in the form. Do not use a production runtime.

| Check | Action and expected observation |
| --- | --- |
| TUI-W01 | Launch TUI; header changes from CONNECTING to CONNECTED and shows an authoritative empty list. |
| TUI-W02 | Resize across 100 columns, 60–99 columns and below 60×18; split, one-panel compact and resize message appear without overflow. |
| TUI-W03 | Use `j/k`, arrows, `Tab`, `/`, `?`, `v` and `Esc`; selection/filter/focus change without accidental mutation. |
| TUI-W04 | Click rows and panels, then wheel-scroll list/logs; only selection, focus or scroll changes. |
| TUI-W05 | Register `alpha` and `beta`; both appear from SSE without manual refresh. |
| TUI-W06 | Start both; stop and restart `alpha` using confirmation; state text updates from SSE. |
| TUI-W07 | Select each instance, verify its own `sim:<workdir> seq:<number>` lines, scroll up, then press `G` or `f` to follow. |
| TUI-W08 | Cancel a removal dialog, then stop and remove `alpha`; it disappears and selection recovers to `beta`. |
| TUI-W09 | Suspend the isolated daemon with `kill -STOP "$daemon_pid"` until the stream reports a disconnect, then resume it with `kill -CONT "$daemon_pid"`; observe DISCONNECTED/RECONNECTING, then CONNECTED with a fresh snapshot and retained logs. |
| TUI-W10 | Observe simulated OSC 52, clear-screen and invalid UTF-8 bytes as visible escapes inside logs; terminal title, clipboard and screen position do not change. |
| TUI-W11 | Compare a light terminal, a dark terminal and `NO_COLOR=1`; all states and controls remain readable as text. |
| TUI-W12 | With `beta` running, press `q`; `kill -0 "$daemon_pid"` succeeds. Reopen the TUI and verify `beta` remains RUNNING before explicitly stopping the isolated daemon. |

When finished, stop the isolated daemon explicitly:

```sh
kill -TERM "$daemon_pid"
wait "$daemon_pid"
```

The temporary directory and artifacts are intentionally preserved. Report each TUI-W result separately; none is inferred from automated tests. Native CS2 validation remains PENDING.

## Final interactive findings

The user reported TUI-W01–TUI-W12 PASS after the layout correction. Registration and simulator startup succeeded; `n` opened registration while connected, and native Ghostty Ctrl+Shift+V pasted into the form. Ctrl+V was not separately confirmed. Simulated OSC, ANSI, BEL and malformed UTF-8 appeared as escaped text with the clipboard unchanged. The user accepted theme readability without a significant visual difference and confirmed a managed simulator remained running after quitting and reopening the TUI. These are user-reported observations, separate from automated and PTY tests. Long log lines are horizontally clipped; horizontal scrolling or wrapping is a future usability improvement. Earlier unexpected SSE disconnections remain unexplained, although explicit and interactive reconnection passed. Native CS2 validation remains PENDING.
