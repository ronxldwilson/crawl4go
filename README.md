# crawl4go

A lightweight, high-performance web crawler and content extraction service written in Go. Built for LLM-ready output with privacy-first design.

crawl4go ports the best algorithms from [Crawl4AI](https://github.com/unclecode/crawl4ai) into a single Go binary (~15MB Docker image) that plugs into an existing Tor + headless Chrome stack.

## Features

- **CDP rendering** via ZenPanda (headless Chromium) with stealth anti-detection
- **HTTP + CDP race** — parallel fetch for fastest content extraction
- **Deep crawling** — BFS, DFS, and Best-First (priority queue) traversal strategies
- **Anti-bot detection** — 3-tier detection (Cloudflare, Akamai, PerimeterX, DataDome, Imperva, etc.)
- **Content pruning** — recursive HTML tree pruning by text density, link density, and tag weight
- **Markdown output** — clean Markdown with citation-style links
- **BM25 relevance scoring** — Okapi BM25 with Snowball stemming and tag priority weights
- **URL filtering** — pattern, domain, and content-type filter chains
- **URL scoring** — keyword relevance, path depth, freshness, and composite scorers
- **Lazy-load handling** — scroll injection via CDP to trigger dynamic content
- **Tor integration** — routes through rotating Tor proxy pool for IP anonymity

## Quick Start

```bash
docker compose up -d
```

This starts three services:

| Service | Port | Description |
|---------|------|-------------|
| **crawl4go** | 8082 | Crawler API |
| **zenpanda** | 9222 | Headless Chromium (CDP) |
| **tor-proxy** | 3128 | 500 rotating Tor circuits |

## API

### POST /crawl

Crawl a single URL and extract content.

```bash
curl -X POST http://localhost:8082/crawl \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "wait_ms": 1500,
    "scroll": true,
    "max_scroll_steps": 10,
    "output": "markdown",
    "prune": true,
    "proxy": true
  }'
```

**Parameters:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | required | URL to crawl |
| `wait_ms` | int | 1500 | Milliseconds to wait after page load |
| `scroll` | bool | false | Scroll page to trigger lazy-loaded content |
| `max_scroll_steps` | int | 10 | Maximum scroll iterations |
| `output` | string | "markdown" | Output format: `markdown`, `text`, or `html` |
| `prune` | bool | false | Remove boilerplate (nav, footer, ads, etc.) |
| `proxy` | bool | false | Route through Tor proxy |

**Response:**

```json
{
  "url": "https://example.com",
  "status_code": 200,
  "blocked": false,
  "content": "# Example Domain\n\nThis domain is for use in illustrative examples...",
  "links": {
    "internal": [{"href": "https://example.com/about", "text": "About"}],
    "external": [{"href": "https://iana.org", "text": "IANA"}]
  },
  "render_time_ms": 1823,
  "render_source": "cdp"
}
```

### POST /deep-crawl

Crawl a site recursively using BFS, DFS, or Best-First strategy.

```bash
curl -X POST http://localhost:8082/deep-crawl \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "strategy": "bfs",
    "max_depth": 3,
    "max_pages": 100,
    "include_external": false,
    "filters": {
      "url_patterns": ["*.html", "/blog/*"],
      "blocked_domains": ["ads.example.com"]
    },
    "scorer": {
      "keywords": ["python", "tutorial"],
      "keyword_weight": 0.5,
      "freshness_weight": 0.3,
      "depth_weight": 0.2
    },
    "score_threshold": 0.3,
    "output": "markdown",
    "prune": true,
    "wait_ms": 1000
  }'
```

**Strategies:**

| Strategy | Description |
|----------|-------------|
| `bfs` | Breadth-first, level-by-level. Parallel crawl per level. Good default. |
| `dfs` | Depth-first, stack-based. Explores branches fully before backtracking. |
| `best-first` | Priority queue by URL score. Crawls highest-value pages first. |

**Response:**

```json
{
  "results": [
    {
      "url": "https://example.com",
      "depth": 0,
      "parent_url": "",
      "status_code": 200,
      "blocked": false,
      "content": "...",
      "links": {"internal": [], "external": []},
      "score": 0.85,
      "render_time_ms": 1234
    }
  ],
  "stats": {
    "pages_crawled": 42,
    "pages_blocked": 3,
    "max_depth_reached": 2,
    "total_time_ms": 15000
  }
}
```

### GET /health

```bash
curl http://localhost:8082/health
# {"status":"ok"}
```

## Configuration

All configuration via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `CRAWL4GO_PORT` | 8082 | Service port |
| `ZENPANDA_URL` | http://zenpanda:9222 | Headless Chrome CDP endpoint |
| `TOR_PROXY_URL` | http://tor-proxy:3128 | Tor SOCKS proxy |
| `DEFAULT_WAIT_MS` | 1500 | Default page render wait time |
| `MAX_CONCURRENT` | 4 | Max concurrent CDP sessions |
| `REQUEST_TIMEOUT_MS` | 30000 | Overall request timeout |

## Architecture

```
Client
  |
  v
crawl4go (:8082)
  |
  |--- HTTP fetch (through Tor) ---|
  |                                |--> race, take best
  |--- CDP render (ZenPanda) ------|
  |
  v
Anti-bot check --> Prune --> Markdown/Text --> Response
```

For deep crawl, the strategy engine (BFS/DFS/Best-First) orchestrates multiple crawl cycles with link discovery, filtering, and scoring between each level.

## Dependencies

4 external Go modules:

- `github.com/gorilla/websocket` — CDP WebSocket communication
- `golang.org/x/net/html` — HTML parsing and tree walking
- `github.com/JohannesKaufmann/html-to-markdown/v2` — Markdown conversion
- `github.com/kljensen/snowball` — Snowball stemming for BM25

## Part of the TipStat Sourcer Stack

crawl4go is designed to work alongside:

- **[single-leaf](https://github.com/ronxldwilson/single-leaf)** — Search aggregator + deep search
- **[zenpanda](https://hub.docker.com/r/ronxldwilson/zenpanda)** — Headless Chromium container
- **[tor-proxy-pool](https://hub.docker.com/r/ronxldwilson/tor-proxy-pool)** — Rotating Tor circuit pool

## License

MIT
