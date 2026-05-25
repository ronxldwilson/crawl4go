#!/usr/bin/env bash
set -euo pipefail

CRAWL4GO="http://localhost:8082"
CRAWL4AI="http://localhost:11235"
ITERATIONS=5
RESULTS_DIR="$(dirname "$0")/results"
mkdir -p "$RESULTS_DIR"

G='\033[0;32m' R='\033[0;31m' B='\033[0;34m' Y='\033[0;33m' NC='\033[0m'

NAMES=(
  "Hacker News"
  "Wikipedia"
  "BBC News"
  "GitHub (repo)"
  "MDN Web Docs"
  "Reddit"
  "Stack Overflow"
  "Amazon (product)"
)
URLS=(
  "https://news.ycombinator.com"
  "https://en.wikipedia.org/wiki/Web_scraping"
  "https://www.bbc.com/news"
  "https://github.com/unclecode/crawl4ai"
  "https://developer.mozilla.org/en-US/docs/Web/HTML"
  "https://old.reddit.com/r/golang"
  "https://stackoverflow.com/questions/tagged/web-scraping"
  "https://www.amazon.com/dp/B0D1XD1ZV3"
)

echo -e "${B}"
echo "  ┌─────────────────────────────────────────────────┐"
echo "  │  crawl4go vs crawl4ai — Real Sites Benchmark     │"
echo "  └─────────────────────────────────────────────────┘"
echo -e "${NC}"

printf "  %-22s │ %-28s │ %-28s │ %s\n" "Site" "crawl4go (avg / p50)" "crawl4ai (avg / p50)" "Winner"
printf "  %-22s─┼─%-28s─┼─%-28s─┼─%s\n" "──────────────────────" "────────────────────────────" "────────────────────────────" "───────"

for idx in "${!NAMES[@]}"; do
  name="${NAMES[$idx]}"
  url="${URLS[$idx]}"

  # crawl4go
  c4go_times=()
  for i in $(seq 1 $ITERATIONS); do
    t=$(curl -sf -o /dev/null -w '%{time_total}' --max-time 30 -X POST "$CRAWL4GO/crawl" \
      -H 'Content-Type: application/json' \
      -d "{\"url\":\"$url\",\"output\":\"markdown\",\"prune\":true,\"wait_ms\":500}" 2>/dev/null || echo "30")
    c4go_times+=("$t")
  done

  # crawl4ai
  c4ai_times=()
  for i in $(seq 1 $ITERATIONS); do
    t=$(curl -sf -o /dev/null -w '%{time_total}' --max-time 30 -X POST "$CRAWL4AI/crawl" \
      -H 'Content-Type: application/json' \
      -d "{\"urls\":[\"$url\"],\"crawler_config\":{\"type\":\"CrawlerRunConfig\",\"params\":{\"cache_mode\":\"bypass\"}}}" 2>/dev/null || echo "30")
    c4ai_times+=("$t")
  done

  c4go_avg=$(echo "${c4go_times[@]}" | tr ' ' '\n' | awk '{s+=$1;n++} END{printf "%.2f", s/n}')
  c4go_p50=$(echo "${c4go_times[@]}" | tr ' ' '\n' | sort -n | awk -v n=$ITERATIONS 'NR==int(n/2+0.5){printf "%.2f", $1}')

  c4ai_avg=$(echo "${c4ai_times[@]}" | tr ' ' '\n' | awk '{s+=$1;n++} END{printf "%.2f", s/n}')
  c4ai_p50=$(echo "${c4ai_times[@]}" | tr ' ' '\n' | sort -n | awk -v n=$ITERATIONS 'NR==int(n/2+0.5){printf "%.2f", $1}')

  faster=$(echo "$c4ai_avg $c4go_avg" | awk '{if($1>$2) print "go"; else print "ai"}')
  if [ "$faster" = "go" ]; then
    speedup=$(echo "$c4ai_avg $c4go_avg" | awk '{if($2>0 && $2<29) printf "%.1fx", $1/$2; else print "N/A"}')
    winner="${G}${speedup} → Go${NC}"
  else
    rev=$(echo "$c4go_avg $c4ai_avg" | awk '{if($2>0 && $2<29) printf "%.1fx", $1/$2; else print "N/A"}')
    winner="${R}${rev} → Py${NC}"
  fi

  printf "  %-22s │ avg=%5ss  p50=%5ss  │ avg=%5ss  p50=%5ss  │ " "$name" "$c4go_avg" "$c4go_p50" "$c4ai_avg" "$c4ai_p50"
  echo -e "$winner"
done

# Quality check on Wikipedia
echo ""
echo -e "${B}═══ Content Quality (Wikipedia) ═══${NC}"
c4go_resp=$(curl -sf --max-time 30 -X POST "$CRAWL4GO/crawl" \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://en.wikipedia.org/wiki/Web_scraping","output":"markdown","prune":true,"wait_ms":500,"extract_meta":true,"extract_tables":true}' 2>/dev/null)

c4ai_resp=$(curl -sf --max-time 30 -X POST "$CRAWL4AI/crawl" \
  -H 'Content-Type: application/json' \
  -d '{"urls":["https://en.wikipedia.org/wiki/Web_scraping"],"crawler_config":{"type":"CrawlerRunConfig","params":{"cache_mode":"bypass"}}}' 2>/dev/null)

echo "$c4go_resp" | python3 -m json.tool > "$RESULTS_DIR/real_crawl4go_wiki.json" 2>/dev/null || true
echo "$c4ai_resp" | python3 -m json.tool > "$RESULTS_DIR/real_crawl4ai_wiki.json" 2>/dev/null || true

c4go_len=$(echo "$c4go_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('content','')))" 2>/dev/null || echo "0")
c4ai_len=$(echo "$c4ai_resp" | python3 -c "
import sys,json
r=json.load(sys.stdin)
results=r.get('results',r.get('result',[]))
d=results[0] if isinstance(results,list) and results else (results if isinstance(results,dict) else {})
md = d.get('markdown','') or d.get('markdown_v2',{}).get('raw_markdown','') or d.get('html','') or ''
print(len(md))
" 2>/dev/null || echo "0")

c4go_links=$(echo "$c4go_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); l=d.get('links',{}); print(len(l.get('internal',[]))+len(l.get('external',[])))" 2>/dev/null || echo "0")
c4ai_links=$(echo "$c4ai_resp" | python3 -c "
import sys,json
r=json.load(sys.stdin)
results=r.get('results',r.get('result',[]))
d=results[0] if isinstance(results,list) and results else (results if isinstance(results,dict) else {})
l=d.get('links',{})
internal=l.get('internal',[]) if isinstance(l.get('internal'), list) else []
external=l.get('external',[]) if isinstance(l.get('external'), list) else []
print(len(internal)+len(external))
" 2>/dev/null || echo "0")

c4go_tables=$(echo "$c4go_resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('tables',[])))" 2>/dev/null || echo "0")

printf "  %-20s content=%s chars, links=%s, tables=%s\n" "crawl4go:" "$c4go_len" "$c4go_links" "$c4go_tables"
printf "  %-20s content=%s chars, links=%s\n" "crawl4ai:" "$c4ai_len" "$c4ai_links"

# Memory snapshot after all the real-site crawling
echo ""
echo -e "${B}═══ Memory After Real-Site Workload ═══${NC}"
c4go_mem=$(docker stats --no-stream --format "{{.MemUsage}}" "$(docker compose -f "$(dirname "$0")/docker-compose.yml" ps -q crawl4go)" 2>/dev/null | head -1)
zen_mem=$(docker stats --no-stream --format "{{.MemUsage}}" "$(docker compose -f "$(dirname "$0")/docker-compose.yml" ps -q zenpanda)" 2>/dev/null | head -1)
c4ai_mem=$(docker stats --no-stream --format "{{.MemUsage}}" "$(docker compose -f "$(dirname "$0")/docker-compose.yml" ps -q crawl4ai)" 2>/dev/null | head -1)

printf "  %-20s %s\n" "crawl4go:" "$c4go_mem"
printf "  %-20s %s\n" "  + zenpanda:" "$zen_mem"
printf "  %-20s %s\n" "crawl4ai:" "$c4ai_mem"

echo ""
echo "  Results saved to: $RESULTS_DIR/"
