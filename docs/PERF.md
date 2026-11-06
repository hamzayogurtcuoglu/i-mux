# Performance

Budgets are in ARCHITECTURE §17. Numbers below were measured on the development machine and are
re-checked before each release.

## Machine

- 2026-11-06, Windows 11 with WSL2 (Ubuntu 22.04, Linux 5.15, x86_64), Go 1.27.3, tmux 3.2a.
- The binary and fake agents ran from the Linux file system (e2e tests copy them there; WSL's
  `/mnt/c` adds about 65 ms to every exec).

## Watcher (`scripts/measure-watch.sh 600`)

Four fake-agent panes; two of them redraw a spinner five times a second, like real agents. Each
phase ran for 10 minutes, sampling every 5 s. CPU is the process's CPU time over the phase.

| Phase | Max RSS | Mean CPU (one core) | tmux execs | Budget |
|---|---|---|---|---|
| client attached | 10.6 MB | 0.035 % | 480 in 600 s (2 per 2.5 s tick) | < 15 MB (target < 10 MB), < 0.5 %, ≤ 2 execs per tick |
| no client | 10.7 MB | 0.003 % | 40 in 601 s (1 per 15 s) | ≤ 1 exec per 15 s |

Within budget, so the control-mode client (`tmux -C`) planned in §17 is not needed (Q21). RSS
is slightly above the 10 MB target and well under the 15 MB budget.

## Other measurements (e2e and benchmarks)

| Metric | Measured | Budget | Where |
|---|---|---|---|
| Menu open latency (process start → menu shown) | p50 3.6 ms, p95 5.3 ms (50 runs) | < 100 ms p95 | `e2e/menu_test.go` |
| Session picker open, 50 candidate files | p50 5.3 ms, p95 8.1 ms (50 runs) | < 150 ms p95 | `e2e/picker_test.go` |
| Switch from a 64 MB Claude session (command → new agent's exec) | 95 ms | < 1.5 s | `e2e/m2_test.go` |
| `imux handoff --print` on a 64 MB session | 107 ms, peak RSS 17.5 MB | < 1 s, < 80 MB | `e2e/perf_test.go` |
| Claude 5-hour usage on the 64 MB fixture | 48 ms | < 300 ms | `internal/limits` |
| Agent bar re-render on focus change | 0 i-mux execs | 0 | `e2e/bar_test.go` |
| Resident i-mux processes per pane | 0 (only the watcher) | 0 | `e2e/m2_test.go` |

Reproduce:

```sh
make build
scripts/measure-watch.sh 600          # 2 × 10 minutes
make e2e                               # latency checks log their numbers with -v
```
