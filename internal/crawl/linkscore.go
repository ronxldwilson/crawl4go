package crawl

import (
	"math"
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/kljensen/snowball"
)

// LinkScore holds intrinsic and contextual relevance scores for a URL.
type LinkScore struct {
	URL             string  `json:"url"`
	IntrinsicScore  float64 `json:"intrinsic_score"`
	ContextualScore float64 `json:"contextual_score"`
	TotalScore      float64 `json:"total_score"`
}

// LinkScorer scores links using configurable weights for intrinsic
// URL features and contextual BM25 relevance.
type LinkScorer struct {
	IntrinsicWeight  float64
	ContextualWeight float64
	OptimalDepth     int
}

// NewLinkScorer returns a LinkScorer with sensible defaults.
func NewLinkScorer() *LinkScorer {
	return &LinkScorer{
		IntrinsicWeight:  0.4,
		ContextualWeight: 0.6,
		OptimalDepth:     2,
	}
}

// ScoreLink computes intrinsic and contextual scores for a single URL.
// Intrinsic score is derived from URL depth, query parameter count, and
// file extension. Contextual score uses BM25 over anchor text and
// surrounding text matched against the query.
func (ls *LinkScorer) ScoreLink(rawURL, anchorText, surroundingText, query string) LinkScore {
	result := LinkScore{URL: rawURL}
	result.IntrinsicScore = ls.intrinsicScore(rawURL)

	if query != "" {
		combined := anchorText + " " + surroundingText
		result.ContextualScore = ls.bm25Score(combined, query)
	}

	result.TotalScore = ls.IntrinsicWeight*result.IntrinsicScore +
		ls.ContextualWeight*result.ContextualScore

	return result
}

// ScoreLinks scores a batch of links in place and sorts by TotalScore descending.
func (ls *LinkScorer) ScoreLinks(links []LinkScore) {
	sort.Slice(links, func(i, j int) bool {
		return links[i].TotalScore > links[j].TotalScore
	})
}

// intrinsicScore evaluates URL features: path depth, query parameters,
// and file extension.
func (ls *LinkScorer) intrinsicScore(rawURL string) float64 {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}

	// Depth score: inversely proportional to distance from optimal depth
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	depth := len(parts)
	if len(parts) == 1 && parts[0] == "" {
		depth = 0
	}
	depthScore := 1.0 / (1.0 + math.Abs(float64(depth-ls.OptimalDepth)))

	// Parameter penalty: more query parameters = lower score
	paramCount := len(u.Query())
	paramScore := 1.0 / (1.0 + float64(paramCount)*0.1)

	// Extension score
	ext := strings.ToLower(path.Ext(u.Path))
	extScore := extensionScore(ext)

	return (depthScore + paramScore + extScore) / 3.0
}

// extensionScore returns a quality score based on file extension.
func extensionScore(ext string) float64 {
	scores := map[string]float64{
		"":      1.0,
		".html": 1.0,
		".htm":  1.0,
		".php":  0.8,
		".asp":  0.7,
		".aspx": 0.7,
		".jsp":  0.7,
		".pdf":  0.5,
		".xml":  0.3,
		".json": 0.3,
		".txt":  0.2,
		".css":  0.0,
		".js":   0.0,
		".png":  0.0,
		".jpg":  0.0,
		".gif":  0.0,
		".svg":  0.0,
	}
	if s, ok := scores[ext]; ok {
		return s
	}
	return 0.5
}

// bm25Score computes a simple BM25-like relevance score for text against a query.
func (ls *LinkScorer) bm25Score(text, query string) float64 {
	queryTokens := linkTokenize(query)
	docTokens := linkTokenize(text)
	if len(queryTokens) == 0 || len(docTokens) == 0 {
		return 0
	}

	const k1 = 2.0
	const b = 0.75

	// Treat as a single-document corpus for simplicity (IDF = log(1.5))
	tf := make(map[string]int)
	for _, t := range docTokens {
		tf[t]++
	}

	dl := float64(len(docTokens))
	score := 0.0

	for _, qt := range queryTokens {
		termFreq := float64(tf[qt])
		if termFreq == 0 {
			continue
		}
		idf := math.Log(1.5) // single doc approximation
		tfNorm := (termFreq * (k1 + 1)) / (termFreq + k1*(1-b+b*dl/dl))
		score += idf * tfNorm
	}

	return score
}

// linkStopWords is a set of common English stop words for tokenization.
var linkStopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"is": true, "it": true, "be": true, "as": true, "was": true, "with": true,
	"by": true, "that": true, "this": true, "from": true, "are": true,
}

// linkTokenize splits text into stemmed tokens, filtering stop words.
func linkTokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var tokens []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}")
		if len(w) < 2 || linkStopWords[w] {
			continue
		}
		stemmed, err := snowball.Stem(w, "english", true)
		if err != nil {
			stemmed = w
		}
		tokens = append(tokens, stemmed)
	}
	return tokens
}
