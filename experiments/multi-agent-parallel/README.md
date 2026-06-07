# multi-agent-parallel

Run `jade`, `amber`, and `onyx` in parallel against the same current arena state.

## Usage

```bash
env GOCACHE=/root/ai-arena/.cache/go-build /usr/local/go/bin/go run ./experiments/multi-agent-parallel \
  -duration 90s \
  -out-dir tmp/multi-agent-parallel
```

Optional:

- `-residents jade,amber,onyx`
- `-reset-resident=false`
- `-verbose`
