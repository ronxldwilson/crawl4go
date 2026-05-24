<p align="center">
  <pre align="center">
                         _  _
  ___ _ __ __ ___      _| || |   __ _  ___
 / __| '__/ _' \ \ /\ / / || |_ / _' |/ _ \
| (__| | | (_| |\ V  V /|__   _| (_| | (_) |
 \___|_|  \__,_| \_/\_/    |_|  \__, |\___/
                                 |___/
  </pre>
</p>

<p align="center">
  <strong>High-performance web crawler and content extraction service in Go</strong>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#api-reference">API Reference</a> &middot;
  <a href="#deep-crawl-strategies">Strategies</a> &middot;
  <a href="#architecture">Architecture</a> &middot;
  <a href="#configuration">Configuration</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white" alt="Go 1.25" />
  <img src="https://img.shields.io/badge/Docker-~15MB-2496ED?logo=docker&logoColor=white" alt="Docker ~15MB" />
  <img src="https://img.shields.io/badge/License-Apache%202.0-orange" alt="License" />
  <img src="https://img.shields.io/badge/Dependencies-4-brightgreen" alt="Dependencies" />
  <img src="https://img.shields.io/docker/pulls/ronxldwilson/crawl4go?logo=docker&label=pulls" alt="Docker Pulls" />
</p>

---

crawl4go is a Go rewrite of [Crawl4AI](https://github.com/unclecode/crawl4ai) -- the same algorithms, rebuilt as a single statically-linked binary (~15 MB Docker image). It plugs into a headless Chromium (ZenPanda) and rotating Tor proxy pool to deliver LLM-ready content with privacy-first design.

## Features

| Category | Capabilities |
|----------|-------------|
| **Rendering** | HTTP + CDP race (parallel fetch, take fastest), ZenPanda headless Chromium, scroll injection for lazy-loaded content |
| **Deep Crawling** | 4 strategies: BFS, DFS, Best-First (priority queue), Adaptive (statistical convergence) |
| **Content Processing** | HTML pruning by text/link density, BM25 relevance scoring with Snowball stemming, HTML-to-Markdown with citation-style links |
| **Extraction** | CSS selector, XPath, and regex extraction; JSON-LD / OpenGraph / Twitter Card metadata; table extraction with data-table scoring; media extraction with quality scoring; link previews |
| **Anti-Bot Detection** | 3-tier detection: structural markers, generic terms, structural integrity (Cloudflare, Akamai, PerimeterX, DataDome, Imperva) |
| **URL Intelligence** | Robots.txt checking, sitemap discovery, URL scoring (keyword relevance, path depth, freshness), filter chains (pattern, domain, content-type, extension) |
| **Rate Limiting** | Per-domain adaptive exponential backoff |
| **Cache Validation** | HTTP conditional requests (ETag, Last-Modified) |
| **Text Chunking** | Fixed-size, sliding window, semantic, markdown-aware |
| **SSL Inspection** | TLS certificate chain analysis (subject, issuer, expiry, fingerprint, SAN) |
| **Stealth** | Navigator property overrides, consent popup removal, overlay removal, shadow DOM flattening |
| **Infrastructure** | ZenPanda CDP, Tor proxy with 500 rotating circuits, user-agent rotation |

## Quick Start

```bash
docker compose up -d
```

Three services start together:

| Service | Port | Description |
|---------|------|-------------|
| **crawl4go** | `8082` | Crawler and extraction API |
| **zenpanda** | `9222` | Headless Chromium via CDP |
| **tor-proxy** | `3128` | 500 rotating Tor circuits |

Verify everything is running:

```bash
curl http://localhost:8082/health
# {"status":"ok","zenpanda":true}
```

## API Reference

crawl4go exposes **17 endpoints** spanning crawling, extraction, content processing, and infrastructure.

---

### POST `/crawl`

Crawl a single URL, render it via CDP or HTTP (raced in parallel), and return processed content.

**Request:**

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
    "proxy": true,
    "extract_meta": true,
    "extract_tables": true,
    "extract_media": true
  }'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | URL to crawl |
| `wait_ms` | int | `1500` | Milliseconds to wait after page load |
| `scroll` | bool | `false` | Scroll page to trigger lazy-loaded content |
| `max_scroll_steps` | int | `10` | Maximum scroll iterations |
| `output` | string | `"markdown"` | Output format: `markdown`, `text`, or `html` |
| `prune` | bool | `false` | Remove boilerplate (nav, footer, ads, sidebars) |
| `proxy` | bool | `false` | Route through Tor proxy |
| `extract_meta` | bool | `false` | Extract OpenGraph, Twitter Card, JSON-LD metadata |
| `extract_tables` | bool | `false` | Extract and score HTML tables |
| `extract_media` | bool | `false` | Extract images, videos, and audio with quality scoring |

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
  "metadata": {
    "title": "Example Domain",
    "description": "An example page",
    "open_graph": {"title": "Example Domain", "type": "website"},
    "twitter_card": {},
    "json_ld": [{"@type": "WebSite", "@context": "https://schema.org"}],
    "canonical": "https://example.com"
  },
  "tables": [
    {
      "headers": ["Name", "Value"],
      "rows": [["key", "123"]],
      "caption": "Sample data",
      "score": 0.85,
      "is_data_table": true
    }
  ],
  "media": {
    "images": [{"url": "https://example.com/hero.jpg", "type": "image", "alt": "Hero image", "width": 1200, "height": 630, "source_tag": "img", "score": 0.92}],
    "videos": [],
    "audio": []
  },
  "render_time_ms": 1823,
  "render_source": "cdp"
}
```

> Fields `metadata`, `tables`, and `media` are only present when their respective `extract_*` flags are set to `true`.

---

### POST `/deep-crawl`

Crawl a site recursively using one of four traversal strategies. Includes link discovery, URL filtering, scoring, robots.txt checking, and per-domain rate limiting between each level.

**Request:**

```bash
curl -X POST http://localhost:8082/deep-crawl \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "strategy": "best-first",
    "max_depth": 3,
    "max_pages": 50,
    "include_external": false,
    "filters": {
      "url_patterns": ["*.html", "/blog/*"],
      "blocked_domains": ["ads.example.com"],
      "allowed_domains": ["example.com"],
      "allowed_extensions": [".html", ".htm"]
    },
    "scorer": {
      "keywords": ["python", "tutorial"],
      "keyword_weight": 0.5,
      "freshness_weight": 0.3,
      "depth_weight": 0.2
    },
    "score_threshold": 0.3,
    "query_terms": ["python web scraping"],
    "output": "markdown",
    "prune": true,
    "scroll": false,
    "wait_ms": 1000
  }'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | Starting URL |
| `strategy` | string | `"bfs"` | Traversal strategy: `bfs`, `dfs`, `best-first`, or `adaptive` |
| `max_depth` | int | `3` | Maximum link depth from seed URL |
| `max_pages` | int | `100` | Maximum pages to crawl |
| `include_external` | bool | `false` | Follow links to external domains |
| `filters` | object | `null` | URL filter configuration (see below) |
| `scorer` | object | `null` | URL scoring weights (see below) |
| `score_threshold` | float | `0.0` | Minimum score for a URL to be crawled |
| `query_terms` | []string | `null` | Query terms for adaptive strategy convergence |
| `output` | string | `"markdown"` | Output format per page |
| `prune` | bool | `false` | Prune boilerplate per page |
| `scroll` | bool | `false` | Scroll pages for lazy content |
| `wait_ms` | int | `1500` | Per-page render wait |

**Filter fields:**

| Field | Type | Description |
|-------|------|-------------|
| `url_patterns` | []string | Glob patterns URLs must match |
| `blocked_domains` | []string | Domains to skip |
| `allowed_domains` | []string | Only crawl these domains |
| `allowed_extensions` | []string | Only crawl URLs with these file extensions |

**Scorer fields:**

| Field | Type | Description |
|-------|------|-------------|
| `keywords` | []string | Terms to match in URL path segments |
| `keyword_weight` | float | Weight for keyword relevance (0-1) |
| `freshness_weight` | float | Weight for URL freshness signals (0-1) |
| `depth_weight` | float | Weight for path depth (0-1, shallower = higher) |

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
      "content": "# Example Domain\n\n...",
      "links": {
        "internal": [{"href": "https://example.com/about", "text": "About"}],
        "external": []
      },
      "score": 0.85,
      "render_time_ms": 1234
    },
    {
      "url": "https://example.com/about",
      "depth": 1,
      "parent_url": "https://example.com",
      "status_code": 200,
      "blocked": false,
      "content": "# About\n\n...",
      "links": {"internal": [], "external": []},
      "score": 0.72,
      "render_time_ms": 987
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

---

### POST `/extract`

Extract structured data from a page using CSS selectors. Useful for scraping product listings, article feeds, directories, and any repeating HTML structures.

**Request:**

```bash
curl -X POST http://localhost:8082/extract \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://news.ycombinator.com",
    "schema": {
      "base_selector": ".athing",
      "fields": [
        {"name": "rank", "selector": ".rank", "type": "text"},
        {"name": "title", "selector": ".titleline > a", "type": "text"},
        {"name": "link", "selector": ".titleline > a", "type": "attribute", "attribute": "href"},
        {"name": "site", "selector": ".sitebit a", "type": "text"}
      ]
    },
    "wait_ms": 2000,
    "proxy": false
  }'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | URL to render and extract from |
| `schema` | object | **required** | Extraction schema (see below) |
| `wait_ms` | int | `1500` | Render wait time |
| `proxy` | bool | `false` | Route through Tor proxy |

**Schema fields:**

| Field | Type | Description |
|-------|------|-------------|
| `base_selector` | string | CSS selector for repeating container elements |
| `fields[].name` | string | Output key name |
| `fields[].selector` | string | CSS selector relative to base element |
| `fields[].type` | string | `text`, `attribute`, `html`, `nested`, or `list` |
| `fields[].attribute` | string | HTML attribute name (required when type is `attribute`) |
| `fields[].fields` | []field | Nested fields (when type is `nested`) |

**Response:**

```json
{
  "url": "https://news.ycombinator.com",
  "results": [
    {
      "rank": "1.",
      "title": "Show HN: A faster alternative to pandas",
      "link": "https://github.com/example/project",
      "site": "github.com"
    },
    {
      "rank": "2.",
      "title": "Why Rust is taking over systems programming",
      "link": "https://blog.example.com/rust-systems",
      "site": "blog.example.com"
    }
  ]
}
```

---

### POST `/link-preview`

Fetch OpenGraph/meta preview data for a batch of URLs concurrently. Uses HEAD requests first, then a limited GET (50 KB) for HTML pages to extract metadata.

**Request:**

```bash
curl -X POST http://localhost:8082/link-preview \
  -H "Content-Type: application/json" \
  -d '{
    "urls": [
      "https://github.com/ronxldwilson/crawl4go",
      "https://example.com"
    ],
    "max_concurrent": 5
  }'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `urls` | []string | **required** | URLs to preview |
| `max_concurrent` | int | `5` | Maximum concurrent preview fetches |

**Response:**

```json
{
  "previews": [
    {
      "url": "https://github.com/ronxldwilson/crawl4go",
      "title": "ronxldwilson/crawl4go",
      "description": "High-performance web crawler in Go",
      "image_url": "https://opengraph.githubassets.com/...",
      "site_name": "GitHub",
      "type": "object",
      "status_code": 200,
      "content_type": "text/html; charset=utf-8",
      "content_length": 0
    },
    {
      "url": "https://example.com",
      "title": "Example Domain",
      "description": "",
      "image_url": "",
      "site_name": "",
      "type": "",
      "status_code": 200,
      "content_type": "text/html; charset=UTF-8",
      "content_length": 1256
    }
  ]
}
```

---

### POST `/sitemap`

Discover URLs from a site's sitemap.xml (including sitemap indexes and compressed sitemaps).

**Request:**

```bash
curl -X POST http://localhost:8082/sitemap \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "max_urls": 500
  }'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `url` | string | **required** | Site URL (sitemap.xml is auto-discovered) |
| `max_urls` | int | `1000` | Maximum URLs to return |

**Response:**

```json
{
  "url": "https://example.com",
  "urls": [
    "https://example.com/",
    "https://example.com/about",
    "https://example.com/blog/post-1",
    "https://example.com/blog/post-2"
  ],
  "url_count": 4
}
```

---

### GET `/cert/{host}`

Inspect the TLS certificate chain for a host. Connects with `InsecureSkipVerify` so expired and self-signed certificates can also be analyzed.

**Request:**

```bash
curl http://localhost:8082/cert/example.com
```

**Response:**

```json
{
  "host": "example.com:443",
  "subject": "CN=example.com",
  "issuer": "CN=DigiCert Global G2 TLS RSA SHA256 2020 CA1,O=DigiCert Inc,C=US",
  "not_before": "2024-01-30T00:00:00Z",
  "not_after": "2025-03-01T23:59:59Z",
  "fingerprint": "a1b2c3d4e5f6...",
  "dns_names": ["example.com", "www.example.com"],
  "is_expired": false,
  "is_self_signed": false,
  "serial_number": "0A1B2C3D4E5F",
  "signature_algorithm": "SHA256-RSA"
}
```

---

### POST `/screenshot`

Capture a viewport or full-page PNG screenshot of a rendered page.

**Request:**

```bash
curl -X POST http://localhost:8082/screenshot \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com", "wait_ms": 2000, "full_page": true}'
```

**Response:**

```json
{
  "url": "https://example.com",
  "data": "iVBORw0KGgoAAAANSUhEUgAA..."
}
```

> `data` is a base64-encoded PNG image.

---

### POST `/chunk`

Chunk page content into segments for LLM context windows.

**Request:**

```bash
curl -X POST http://localhost:8082/chunk \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com", "strategy": "semantic", "chunk_size": 4000, "prune": true}'
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `strategy` | string | `"fixed"` | Chunking strategy: `fixed`, `sliding`, `semantic`, `markdown` |
| `chunk_size` | int | `4000` | Target characters per chunk |
| `overlap` | int | `0` | Overlap between chunks (fixed/sliding only) |

---

### POST `/bm25`

Score page content chunks by BM25 relevance to a query.

**Request:**

```bash
curl -X POST http://localhost:8082/bm25 \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com", "query": "machine learning", "threshold": 1.0}'
```

**Response:**

```json
{
  "url": "https://example.com",
  "query": "machine learning",
  "total_chunks": 42,
  "relevant": 8,
  "chunks": [{"index": 3, "text": "...", "tag_name": "p"}]
}
```

---

### POST `/extract-xpath`

Extract structured data using XPath expressions.

**Request:**

```bash
curl -X POST http://localhost:8082/extract-xpath \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "schema": {
      "base_xpath": "//div[@class=\"product\"]",
      "fields": [
        {"name": "title", "xpath": ".//h2", "type": "text"},
        {"name": "link", "xpath": ".//a/@href", "type": "attribute"}
      ]
    }
  }'
```

---

### POST `/extract-regex`

Extract data using regex patterns with named capture groups.

**Request:**

```bash
curl -X POST http://localhost:8082/extract-regex \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "schema": {
      "patterns": [
        {"name": "emails", "pattern": "[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}", "group": 0},
        {"name": "prices", "pattern": "\\$(?P<amount>[0-9]+\\.?[0-9]*)", "group": 0}
      ]
    }
  }'
```

---

### POST `/execute`

Run arbitrary JavaScript on a rendered page via CDP.

**Request:**

```bash
curl -X POST http://localhost:8082/execute \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "expression": "document.querySelectorAll(\"a\").length",
    "await_promise": false,
    "wait_ms": 1500
  }'
```

**Response:**

```json
{
  "url": "https://example.com",
  "result": {"value": 42, "type": "number"}
}
```

---

### POST `/diff`

Compare two text documents and compute similarity.

**Request:**

```bash
curl -X POST http://localhost:8082/diff \
  -H "Content-Type: application/json" \
  -d '{"old_text": "Hello world\nFoo bar", "new_text": "Hello world\nBaz qux"}'
```

**Response:**

```json
{
  "diff": {
    "added": ["Baz qux"],
    "removed": ["Foo bar"],
    "unchanged": 1,
    "total_old": 2,
    "total_new": 2,
    "similarity": 0.5
  },
  "old_hash": "a1b2c3...",
  "new_hash": "d4e5f6..."
}
```

---

### POST `/cdx`

Discover URLs from the Common Crawl CDX index.

**Request:**

```bash
curl -X POST http://localhost:8082/cdx \
  -H "Content-Type: application/json" \
  -d '{"domain": "example.com", "max_urls": 500}'
```

**Response:**

```json
{
  "domain": "example.com",
  "records": [{"url": "https://example.com/page", "timestamp": "20241201120000", "mime_type": "text/html", "status_code": 200}],
  "url_count": 42
}
```

---

### POST `/robots`

Check if a URL is allowed by the site's robots.txt.

**Request:**

```bash
curl -X POST http://localhost:8082/robots \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/admin", "user_agent": "crawl4go"}'
```

**Response:**

```json
{
  "url": "https://example.com/admin",
  "allowed": false
}
```

---

### GET `/health`

Health check endpoint. Reports service status and ZenPanda CDP connectivity.

**Request:**

```bash
curl http://localhost:8082/health
```

**Response:**

```json
{
  "status": "ok",
  "zenpanda": true
}
```

---

## Deep Crawl Strategies

crawl4go ships with four traversal strategies, selectable via the `strategy` field:

| Strategy | Key | Behavior |
|----------|-----|----------|
| **Breadth-First** | `bfs` | Level-by-level traversal. Crawls all pages at depth N before depth N+1. Parallel within each level. Best general-purpose default. |
| **Depth-First** | `dfs` | Stack-based. Follows each branch to its maximum depth before backtracking. Useful for deeply nested documentation sites. |
| **Best-First** | `best-first` | Priority queue ordered by URL score. Crawls the highest-value pages first regardless of depth. Ideal when you have strong keyword signals. |
| **Adaptive** | `adaptive` | Statistical convergence detection. Uses BM25 relevance scoring against `query_terms` and stops crawling a branch when new pages stop yielding relevant content. Best for focused research crawls. |

### Strategy selection guide

```
Need a full site mirror?                    --> bfs
Need to follow a specific deep path?        --> dfs
Have keywords, want highest-value pages?    --> best-first
Researching a topic, unsure of site layout? --> adaptive (+ query_terms)
```

## Architecture

```
                          +-----------------------+
                          |       Client          |
                          +-----------+-----------+
                                      |
                                      v
                    +----------------------------------+
                    |        crawl4go (:8082)           |
                    |                                  |
                    |   /crawl   /deep-crawl           |
                    |   /extract /link-preview          |
                    |   /sitemap /cert/{host}           |
                    |   /health                        |
                    +-------+-------+----------+-------+
                            |       |          |
              +-------------+   +---+---+   +--+--------+
              |                 |       |   |           |
              v                 v       |   v           |
     +----------------+  +---------+   |  +----------+ |
     | HTTP Fetch     |  |  CDP    |   |  | Tor      | |
     | (direct/proxy) |  | Render  |   |  | Proxy    | |
     +--------+-------+  | (Zen-  |   |  | Pool     | |
              |           | Panda) |   |  | (:3128)  | |
              |           +---+----+   |  +----------+ |
              |               |        |                |
              +-------+-------+        |                |
                      |                |                |
                      v                |                |
            Race: take fastest         |                |
                      |                |                |
                      v                v                |
              +-------+--------+  +---+--------+       |
              | Anti-bot check |  | Rate       |       |
              +-------+--------+  | Limiter    |       |
                      |           +---+--------+       |
                      v               |                |
              +-------+--------+      |                |
              | Content        |<-----+                |
              | Pipeline:      |                       |
              |  - Prune HTML  |                       |
              |  - BM25 score  |                       |
              |  - Markdown    |                       |
              |  - Extract     |                       |
              +-------+--------+                       |
                      |                                |
                      v                                |
              +-------+--------+                       |
              |    Response    +--->  Deep-crawl loop:  |
              |                |     strategy engine    |
              +----------------+     (BFS/DFS/Best-    |
                                      First/Adaptive)  |
                                     + robots.txt      |
                                     + URL filtering   |
                                     + URL scoring     |
                                     + sitemap seeding |
```

### Source layout

```
cmd/crawl4go/main.go              HTTP server, handlers, request/response types

internal/browser/
    client.go                      CDP WebSocket client with session pooling
    scroll.go                      Scroll injection for lazy-loaded content
    stealth.go                     Navigator overrides, consent/overlay removal
    jsinject.go                    Shadow DOM flattening, JS execution

internal/content/
    prune.go                       HTML tree pruning by text/link density
    bm25.go                        Okapi BM25 relevance scoring with Snowball stemming
    markdown.go                    HTML-to-Markdown with citation-style links
    text.go                        HTML-to-plaintext conversion
    extract.go                     CSS selector-based structured extraction
    metadata.go                    OpenGraph, Twitter Card, JSON-LD extraction
    table.go                       HTML table extraction with data-table scoring
    media.go                       Image/video/audio extraction with quality scoring
    preview.go                     Link preview (HEAD + limited GET)
    links.go                       Internal/external link extraction, tracking param removal
    antibot.go                     3-tier anti-bot detection
    ssl.go                         TLS certificate chain inspection
    chunk.go                       Text chunking (fixed, sliding window, semantic, markdown-aware)

internal/crawl/
    bfs.go                         Breadth-first crawl strategy
    dfs.go                         Depth-first crawl strategy
    bestfirst.go                   Best-first (priority queue) crawl strategy
    adaptive.go                    Adaptive (statistical convergence) crawl strategy
    filter.go                      URL filter chains (pattern, domain, extension)
    scorer.go                      URL scoring (keyword, freshness, depth)
    robots.go                      Robots.txt parser and checker
    seeder.go                      Sitemap discovery and URL seeding
    ratelimit.go                   Per-domain adaptive exponential backoff
    cache.go                       HTTP conditional request cache (ETag, Last-Modified)
    discover.go                    Link discovery between crawl levels
    types.go                       Shared types (CrawlOptions, DeepCrawlResult, CrawlStats)

internal/proxy/
    proxy.go                       Tor proxy pool integration

internal/ua/
    ua.go                          User-agent rotation
```

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `CRAWL4GO_PORT` | `8082` | HTTP server port |
| `ZENPANDA_URL` | `http://zenpanda:9222` | ZenPanda headless Chromium CDP endpoint |
| `TOR_PROXY_URL` | `http://tor-proxy:3128` | Tor SOCKS proxy URL |
| `DEFAULT_WAIT_MS` | `1500` | Default page render wait time (ms) |
| `MAX_CONCURRENT` | `4` | Maximum concurrent CDP sessions |
| `REQUEST_TIMEOUT_MS` | `30000` | Overall request timeout (ms) |

## Docker

### Multi-stage build

The Dockerfile uses a two-stage build: compile with `golang:1.25-alpine`, then copy the binary into a minimal `alpine` image with only `ca-certificates`. Final image is approximately 15 MB.

```bash
# Build locally
docker build -t crawl4go .

# Run standalone
docker run -p 8082:8082 \
  -e ZENPANDA_URL=http://host.docker.internal:9222 \
  -e TOR_PROXY_URL=http://host.docker.internal:3128 \
  crawl4go
```

### Docker Compose (full stack)

```bash
docker compose up -d
```

This starts the complete stack:

```yaml
services:
  crawl4go:    # ronxldwilson/crawl4go:latest   :8082
  zenpanda:    # ronxldwilson/zenpanda:latest    :9222
  tor-proxy:   # ronxldwilson/tor-proxy-pool     :3128  (500 Tor circuits)
```

Pre-built multi-arch images (amd64 + arm64) are available on Docker Hub:

- [`ronxldwilson/crawl4go`](https://hub.docker.com/r/ronxldwilson/crawl4go)
- [`ronxldwilson/zenpanda`](https://hub.docker.com/r/ronxldwilson/zenpanda)
- [`ronxldwilson/tor-proxy-pool`](https://hub.docker.com/r/ronxldwilson/tor-proxy-pool)

## Dependencies

crawl4go has only 4 external Go module dependencies:

| Module | Purpose |
|--------|---------|
| [`gorilla/websocket`](https://github.com/gorilla/websocket) | CDP WebSocket communication |
| [`x/net/html`](https://pkg.go.dev/golang.org/x/net/html) | HTML parsing and tree walking |
| [`html-to-markdown/v2`](https://github.com/JohannesKaufmann/html-to-markdown) | HTML-to-Markdown conversion |
| [`snowball`](https://github.com/kljensen/snowball) | Snowball stemming for BM25 scoring |

## Part of the TipStat Sourcer Stack

crawl4go runs as a sidecar in the SingleLeaf search stack, handling all page rendering and content extraction for deep-search results:

```
Client ──> SingleLeaf ──> SearXNG ──[Tor]──> Search Engines
                │
                └──[top results]──> crawl4go ──> ZenPanda (CDP)
```

- **[SingleLeaf](https://github.com/ronxldwilson/SingleLeaf)** -- Privacy-first search aggregator, uses crawl4go for deep-search rendering
- **[ZenPanda](https://hub.docker.com/r/ronxldwilson/zenpanda)** -- Headless Chromium container (CDP)
- **[tor-proxy-pool](https://hub.docker.com/r/ronxldwilson/tor-proxy-pool)** -- Rotating Tor circuit pool

## License

Apache 2.0 -- see [LICENSE](LICENSE).

This project is a derivative work of [Crawl4AI](https://github.com/unclecode/crawl4ai) by UncleCode.
