# Debug & Profiling

cortex-cli is a single binary (no separate daemon). It exposes a pprof
HTTP server when launched with `--pprof-port`. `make run-x` enables it
automatically.

| Process | Default pprof port | Flag              | Env var            |
|---------|--------------------|-------------------|--------------------|
| cortex  | 6061               | `--pprof-port`    | `CORTEX_PPROF_PORT` |

---

## Capture a snapshot

Run all commands **while cortex is live**.

### Goroutine dump (blocking / backpressure issues)

```bash
# cortex TUI
curl -s "http://localhost:6061/debug/pprof/goroutine?debug=2" > /tmp/cortex-goroutines.txt
```

### Heap snapshot

```bash
curl -s "http://localhost:6061/debug/pprof/heap" > /tmp/cortex-heap.pprof

go tool pprof -http=:8080 /tmp/cortex-heap.pprof
```

### CPU profile (30-second sample)

```bash
curl -s "http://localhost:6061/debug/pprof/profile?seconds=30" > /tmp/cortex-cpu.pprof

go tool pprof -http=:8080 /tmp/cortex-cpu.pprof
```

### All-in-one snapshot script

```bash
TS=$(date +%Y%m%d_%H%M%S)
OUT="/tmp/cortex-debug-$TS"
mkdir -p "$OUT"

for PORT in 6061; do
  NAME="cortex"
  curl -s "http://localhost:$PORT/debug/pprof/goroutine?debug=2" > "$OUT/$NAME-goroutines.txt"
  curl -s "http://localhost:$PORT/debug/pprof/heap"               > "$OUT/$NAME-heap.pprof"
  curl -s "http://localhost:$PORT/debug/pprof/allocs"             > "$OUT/$NAME-allocs.pprof"
  echo "[$NAME] snapshots saved to $OUT/"
done
```

---

## Debug environment variables

Set by `make run-x` automatically.

| Variable              | Value            | Effect                                         |
|-----------------------|------------------|------------------------------------------------|
| `GOTRACEBACK=crash`   | crash            | Full goroutine stacks + core dump on panic     |
| `GORACE=halt_on_error=1`  | 1            | Halt immediately on first data race detected   |

To capture stderr logs to a file while running manually:

```bash
GOTRACEBACK=crash \
./bin/cortex --pprof-port 6061 2>/tmp/cortex-debug.log
```
