#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-/root/ai-arena}"
MODEL="${MODEL:-gpt-5.4-mini}"
SCENARIO="${SCENARIO:-baseline}"
LAYER="${LAYER:-permanent}"

cd "$ROOT"

for resident in jade amber onyx; do
  env GOCACHE=/root/ai-arena/.cache/go-build /usr/local/go/bin/go run ./experiments/memory-runtime \
    --resident "$resident" \
    --scenario "$SCENARIO" \
    --layer "$LAYER" \
    --model "$MODEL"
done

env GOCACHE=/root/ai-arena/.cache/go-build /usr/local/go/bin/go run ./experiments/reflection-quality \
  --dir experiments/memory-runtime/output \
  --limit 24
