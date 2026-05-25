#!/usr/bin/env bash
set -euo pipefail

# Benchmark: crawl4go vs crawl4ai (upstream Python)
# Compares: latency, throughput, memory, image size

CRAWL4GO="http://localhost:8082"
CRAWL4AI="http://localhost:11235"

# Test URLs — static pages for reproducible results
URLS=(
  "https://example.com"
  "https://httpbin.org/html"
  "https://www.w3.org/TR/2018/SPSD-html401-20180327/"
)

ITERATIONS=10
CONCURRENT=4
RESULTS_DIR="$(dirname "$0")/results"
mkdir -p "$RESULTS_DIR"

# Colors
G='\033[0;32m' R='\033[0;31m' B='\033[0;34m' Y='\033[0;33m' NC='\033[0m'

now_ms() { python3 -c 'import time; print(int(time.time()*1000))'; }

header() { echo -e "\n${B}═══════════════════════════════════════════════════════════${NC}"; echo -e "${B}  $1${NC}"; echo -e "${B}═══════════════════════════════════════════════════════════${NC}\n"; }
info()   { echo -e "${Y}→${NC} $1"; }
ok()     { echo -e "${G}✓${NC} $1"; }
fail()   { echo -e "${R}✗${NC} $1"; }

# ─── Wait for services ───────────────────────────────────────────────
wait_for() {
  local name=$1 url=$2 max=${3:-60}
  info "Waiting for $name..."
  for i in $(seq 1 $max); do
    if curl -sf "$url" > /dev/null 2>&1; then
      ok "$name ready (${i}s)"
      return 0
    fi
    sleep 1
  done
  fail "$name not ready after ${max}s"
  return 1
}

# ─── 1. Image Size ───────────────────────────────────────────────────
bench_image_size() {
  header "1. Docker Image Size"

  c4go_size=$(docker images ronxldwilson/crawl4go:latest --format "{{.Size}}")
  c4ai_size=$(docker images unclecode/crawl4ai:latest --format "{{.Size}}")
  zen_size=$(docker images ronxldwilson/zenpanda:latest --format "{{.Size}}" 2>/dev/null || echo "N/A")

  printf "  %-20s %s\n" "crawl4go:" "$c4go_size"
  printf "  %-20s %s\n" "  + zenpanda:" "$zen_size"
  printf "  %-20s %s\n" "crawl4ai:" "$c4ai_size"
}

# ─── 2. Startup Time ─────────────────────────────────────────────────
bench_startup() {
  header "2. Cold Start Time"

  # crawl4go
  start=$(now_ms)
  docker compose -f "$(dirname "$0")/docker-compose.yml" up -d crawl4go zenpanda 2>/dev/null
  wait_for "crawl4go" "$CRAWL4GO/health"
  c4go_startup=$(( $(now_ms) - start ))

  # crawl4ai
  start=$(now_ms)
  docker compose -f "$(dirname "$0")/docker-compose.yml" up -d crawl4ai 2>/dev/null
  wait_for "crawl4ai" "$CRAWL4AI/health" 120
  c4ai_startup=$(( $(now_ms) - start ))

  printf "  %-20s %d ms\n" "crawl4go:" "$c4go_startup"
  printf "  %-20s %d ms\n" "crawl4ai:" "$c4ai_startup"
}

# ─── 3. Memory Usage ─────────────────────────────────────────────────
bench_memory() {
  header "3. Memory Usage (idle)"

  sleep 2  # let things settle

  c4go_mem=$(docker stats --no-stream --format "{{.MemUsage}}" "$(docker compose -f "$(dirname "$0")/docker-compose.yml" ps -q crawl4go)" 2>/dev/null | head -1)
  zen_mem=$(docker stats --no-stream --format "{{.MemUsage}}" "$(docker compose -f "$(dirname "$0")/docker-compose.yml" ps -q zenpanda)" 2>/dev/null | head -1)
  c4ai_mem=$(docker stats --no-stream --format "{{.MemUsage}}" "$(docker compose -f "$(dirname "$0")/docker-compose.yml" ps -q crawl4ai)" 2>/dev/null | head -1)

  printf "  %-20s %s\n" "crawl4go:" "$c4go_mem"
  printf "  %-20s %s\n" "  + zenpanda:" "$zen_mem"
  printf "  %-20s %s\n" "crawl4ai:" "$c4ai_mem"
}

# ─── 4. Single-page crawl latency ────────────────────────────────────
bench_crawl_latency() {
  header "4. Single-Page Crawl Latency (${ITERATIONS} iterations)"

  for url in "${URLS[@]}"; do
    echo -e "\n  ${Y}URL:${NC} $url"

    # crawl4go
    c4go_times=()
    for i in $(seq 1 $ITERATIONS); do
      t=$(curl -sf -o /dev/null -w '%{time_total}' -X POST "$CRAWL4GO/crawl" \
        -H 'Content-Type: application/json' \
        -d "{\"url\":\"$url\",\"output\":\"markdown\",\"prune\":true}" 2>/dev/null || echo "0")
      c4go_times+=("$t")
    done

    # crawl4ai
    c4ai_times=()
    for i in $(seq 1 $ITERATIONS); do
      t=$(curl -sf -o /dev/null -w '%{time_total}' -X POST "$CRAWL4AI/crawl" \
        -H 'Content-Type: application/json' \
        -d "{\"urls\":[\"$url\"],\"crawler_config\":{\"type\":\"CrawlerRunConfig\",\"params\":{\"cache_mode\":\"bypass\"}}}" 2>/dev/null || echo "0")
      c4ai_times+=("$t")
    done

    # Calculate stats
    c4go_avg=$(echo "${c4go_times[@]}" | tr ' ' '\n' | awk '{s+=$1;n++} END{printf "%.3f", s/n}')
    c4go_p50=$(echo "${c4go_times[@]}" | tr ' ' '\n' | sort -n | awk -v n=$ITERATIONS 'NR==int(n/2+0.5){print}')
    c4go_min=$(echo "${c4go_times[@]}" | tr ' ' '\n' | sort -n | head -1)

    c4ai_avg=$(echo "${c4ai_times[@]}" | tr ' ' '\n' | awk '{s+=$1;n++} END{printf "%.3f", s/n}')
    c4ai_p50=$(echo "${c4ai_times[@]}" | tr ' ' '\n' | sort -n | awk -v n=$ITERATIONS 'NR==int(n/2+0.5){print}')
    c4ai_min=$(echo "${c4ai_times[@]}" | tr ' ' '\n' | sort -n | head -1)

    printf "  %-12s avg=%-8s p50=%-8s min=%-8s\n" "crawl4go:" "${c4go_avg}s" "${c4go_p50}s" "${c4go_min}s"
    printf "  %-12s avg=%-8s p50=%-8s min=%-8s\n" "crawl4ai:" "${c4ai_avg}s" "${c4ai_p50}s" "${c4ai_min}s"

    speedup=$(echo "$c4ai_avg $c4go_avg" | awk '{if($2>0) printf "%.1fx", $1/$2; else print "N/A"}')
    echo -e "  ${G}speedup: ${speedup}${NC}"
  done
}

# ─── 5. Concurrent crawl throughput ──────────────────────────────────
bench_concurrent() {
  header "5. Concurrent Crawl Throughput (${CONCURRENT} parallel)"

  url="https://example.com"

  # crawl4go — N parallel curls
  start=$(now_ms)
  for i in $(seq 1 $CONCURRENT); do
    curl -sf -o /dev/null -X POST "$CRAWL4GO/crawl" \
      -H 'Content-Type: application/json' \
      -d "{\"url\":\"$url\",\"output\":\"markdown\",\"prune\":true}" &
  done
  wait
  c4go_concurrent=$(( $(now_ms) - start ))

  # crawl4ai — single request with N URLs
  urls_json=$(printf '"%s",' $(for i in $(seq 1 $CONCURRENT); do echo "$url"; done) | sed 's/,$//')
  start=$(now_ms)
  curl -sf -o /dev/null -X POST "$CRAWL4AI/crawl" \
    -H 'Content-Type: application/json' \
    -d "{\"urls\":[$urls_json],\"crawler_config\":{\"type\":\"CrawlerRunConfig\",\"params\":{\"cache_mode\":\"bypass\"}}}" 2>/dev/null || true
  c4ai_concurrent=$(( $(now_ms) - start ))

  printf "  %-20s %d ms (${CONCURRENT} pages)\n" "crawl4go:" "$c4go_concurrent"
  printf "  %-20s %d ms (${CONCURRENT} pages)\n" "crawl4ai:" "$c4ai_concurrent"
}

# ─── 6. Memory under load ────────────────────────────────────────────
bench_memory_load() {
  header "6. Memory Usage (under load)"

  # Fire off requests in background
  for i in $(seq 1 $CONCURRENT); do
    curl -sf -o /dev/null -X POST "$CRAWL4GO/crawl" \
      -H 'Content-Type: application/json' \
      -d '{"url":"https://www.w3.org/TR/2018/SPSD-html401-20180327/","output":"markdown","prune":true}' &
    curl -sf -o /dev/null -X POST "$CRAWL4AI/crawl" \
      -H 'Content-Type: application/json' \
      -d '{"urls":["https://www.w3.org/TR/2018/SPSD-html401-20180327/"],"crawler_config":{"type":"CrawlerRunConfig","params":{"cache_mode":"bypass"}}}' &
  done

  sleep 2  # let requests start processing

  c4go_mem=$(docker stats --no-stream --format "{{.MemUsage}}" "$(docker compose -f "$(dirname "$0")/docker-compose.yml" ps -q crawl4go)" 2>/dev/null | head -1)
  zen_mem=$(docker stats --no-stream --format "{{.MemUsage}}" "$(docker compose -f "$(dirname "$0")/docker-compose.yml" ps -q zenpanda)" 2>/dev/null | head -1)
  c4ai_mem=$(docker stats --no-stream --format "{{.MemUsage}}" "$(docker compose -f "$(dirname "$0")/docker-compose.yml" ps -q crawl4ai)" 2>/dev/null | head -1)

  printf "  %-20s %s\n" "crawl4go:" "$c4go_mem"
  printf "  %-20s %s\n" "  + zenpanda:" "$zen_mem"
  printf "  %-20s %s\n" "crawl4ai:" "$c4ai_mem"

  wait 2>/dev/null  # wait for background curls
}

# ─── 7. Response quality comparison ──────────────────────────────────
bench_quality() {
  header "7. Response Quality Comparison"

  url="https://example.com"

  c4go_resp=$(curl -sf -X POST "$CRAWL4GO/crawl" \
    -H 'Content-Type: application/json' \
    -d "{\"url\":\"$url\",\"output\":\"markdown\",\"prune\":true}" 2>/dev/null)

  c4ai_resp=$(curl -sf -X POST "$CRAWL4AI/crawl" \
    -H 'Content-Type: application/json' \
    -d "{\"urls\":[\"$url\"],\"crawler_config\":{\"type\":\"CrawlerRunConfig\",\"params\":{\"cache_mode\":\"bypass\"}}}" 2>/dev/null)

  # Save full responses
  echo "$c4go_resp" | python3 -m json.tool > "$RESULTS_DIR/crawl4go_response.json" 2>/dev/null || echo "$c4go_resp" > "$RESULTS_DIR/crawl4go_response.json"
  echo "$c4ai_resp" | python3 -m json.tool > "$RESULTS_DIR/crawl4ai_response.json" 2>/dev/null || echo "$c4ai_resp" > "$RESULTS_DIR/crawl4ai_response.json"

  c4go_content_len=$(echo "$c4go_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('content','')))" 2>/dev/null || echo "0")
  c4ai_content_len=$(echo "$c4ai_resp" | python3 -c "import sys,json; r=json.load(sys.stdin); results=r.get('results',r.get('result',[])); d=results[0] if isinstance(results,list) and results else results; print(len(d.get('markdown','') or d.get('markdown_v2',{}).get('raw_markdown','') or ''))" 2>/dev/null || echo "0")

  c4go_links=$(echo "$c4go_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); l=d.get('links',{}); print(len(l.get('internal',[]))+len(l.get('external',[])))" 2>/dev/null || echo "0")
  c4ai_links=$(echo "$c4ai_resp" | python3 -c "import sys,json; r=json.load(sys.stdin); results=r.get('results',r.get('result',[])); d=results[0] if isinstance(results,list) and results else results; l=d.get('links',{}); print(len(l.get('internal',[]))+len(l.get('external',[])))" 2>/dev/null || echo "0")

  printf "  %-20s content=%s chars, links=%s\n" "crawl4go:" "$c4go_content_len" "$c4go_links"
  printf "  %-20s content=%s chars, links=%s\n" "crawl4ai:" "$c4ai_content_len" "$c4ai_links"

  ok "Full responses saved to $RESULTS_DIR/"
}

# ─── 8. /diff endpoint (crawl4go-only, pure compute) ─────────────────
bench_diff() {
  header "8. Pure Compute: /diff endpoint (crawl4go only)"

  old_text="The quick brown fox jumps over the lazy dog. This is a test of the diff algorithm with enough text to be meaningful."
  new_text="The quick brown fox leaps over the lazy cat. This is a test of the diff algorithm with some modifications to be meaningful."

  times=()
  for i in $(seq 1 20); do
    t=$(curl -sf -o /dev/null -w '%{time_total}' -X POST "$CRAWL4GO/diff" \
      -H 'Content-Type: application/json' \
      -d "{\"old_text\":\"$old_text\",\"new_text\":\"$new_text\"}" 2>/dev/null || echo "0")
    times+=("$t")
  done

  avg=$(echo "${times[@]}" | tr ' ' '\n' | awk '{s+=$1;n++} END{printf "%.4f", s/n}')
  min=$(echo "${times[@]}" | tr ' ' '\n' | sort -n | head -1)
  printf "  avg=%-10s min=%-10s (20 iterations)\n" "${avg}s" "${min}s"
}

# ─── Main ─────────────────────────────────────────────────────────────
main() {
  echo -e "${B}"
  echo "  ┌─────────────────────────────────────────────┐"
  echo "  │     crawl4go vs crawl4ai — Benchmark         │"
  echo "  └─────────────────────────────────────────────┘"
  echo -e "${NC}"

  bench_image_size
  bench_startup
  bench_memory
  bench_crawl_latency
  bench_concurrent
  bench_memory_load
  bench_quality
  bench_diff

  header "Done"
  echo "  Results saved to: $RESULTS_DIR/"
  echo ""

  # Cleanup
  info "Stopping benchmark containers..."
  docker compose -f "$(dirname "$0")/docker-compose.yml" down 2>/dev/null
  ok "Cleanup complete"
}

main "$@" 2>&1 | tee "$RESULTS_DIR/bench_$(date +%Y%m%d_%H%M%S).log"
