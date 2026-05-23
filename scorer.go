package main

import (
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type URLScorer interface {
	Score(rawURL string) float64
}

// CompositeScorer combines multiple scorers with weights.
type CompositeScorer struct {
	scorers []weightedScorer
}

type weightedScorer struct {
	scorer URLScorer
	weight float64
}

func NewCompositeScorer(scorers []weightedScorer) *CompositeScorer {
	return &CompositeScorer{scorers: scorers}
}

func (cs *CompositeScorer) Score(rawURL string) float64 {
	if len(cs.scorers) == 0 {
		return 0
	}
	totalWeight := 0.0
	totalScore := 0.0
	for _, ws := range cs.scorers {
		totalScore += ws.scorer.Score(rawURL) * ws.weight
		totalWeight += ws.weight
	}
	if totalWeight == 0 {
		return 0
	}
	return totalScore / totalWeight
}

// KeywordRelevanceScorer scores based on keyword presence in the URL.
type KeywordRelevanceScorer struct {
	keywords []string
}

func NewKeywordRelevanceScorer(keywords []string) *KeywordRelevanceScorer {
	lower := make([]string, len(keywords))
	for i, k := range keywords {
		lower[i] = strings.ToLower(k)
	}
	return &KeywordRelevanceScorer{keywords: lower}
}

func (s *KeywordRelevanceScorer) Score(rawURL string) float64 {
	if len(s.keywords) == 0 {
		return 0
	}
	lower := strings.ToLower(rawURL)
	matches := 0
	for _, kw := range s.keywords {
		if strings.Contains(lower, kw) {
			matches++
		}
	}
	return float64(matches) / float64(len(s.keywords))
}

// PathDepthScorer scores based on URL path depth relative to an optimal depth.
type PathDepthScorer struct {
	optimalDepth int
}

func NewPathDepthScorer(optimalDepth int) *PathDepthScorer {
	if optimalDepth <= 0 {
		optimalDepth = 2
	}
	return &PathDepthScorer{optimalDepth: optimalDepth}
}

func (s *PathDepthScorer) Score(rawURL string) float64 {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	depth := len(parts)
	if parts[0] == "" {
		depth = 0
	}
	distance := math.Abs(float64(depth - s.optimalDepth))
	return 1.0 / (1.0 + distance)
}

// ContentTypeScorer scores based on file extension.
type ContentTypeScorer struct {
	scores map[string]float64
}

func NewContentTypeScorer() *ContentTypeScorer {
	return &ContentTypeScorer{
		scores: map[string]float64{
			"":      1.0,
			".html": 1.0,
			".htm":  1.0,
			".php":  0.8,
			".asp":  0.8,
			".aspx": 0.8,
			".jsp":  0.8,
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
			".ico":  0.0,
			".woff": 0.0,
			".woff2": 0.0,
		},
	}
}

func (s *ContentTypeScorer) Score(rawURL string) float64 {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	ext := strings.ToLower(strings.Split(u.Path, "?")[0])
	for e, score := range s.scores {
		if strings.HasSuffix(ext, e) {
			return score
		}
	}
	return 0.5
}

// FreshnessScorer scores based on date patterns found in the URL path.
var dateInURLRe = regexp.MustCompile(`(?:^|/)(\d{4})[-/](\d{1,2})(?:[-/](\d{1,2}))?(?:/|$)`)

type FreshnessScorer struct {
	currentYear int
}

func NewFreshnessScorer() *FreshnessScorer {
	return &FreshnessScorer{currentYear: time.Now().Year()}
}

func (s *FreshnessScorer) Score(rawURL string) float64 {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0
	}
	matches := dateInURLRe.FindStringSubmatch(u.Path)
	if len(matches) < 2 {
		return 0.5 // no date = neutral
	}

	year, err := strconv.Atoi(matches[1])
	if err != nil || year < 1990 || year > s.currentYear+1 {
		return 0.5
	}

	age := s.currentYear - year
	switch {
	case age == 0:
		return 1.0
	case age == 1:
		return 0.8
	case age == 2:
		return 0.6
	case age <= 4:
		return 0.4
	default:
		return 0.2
	}
}

func BuildScorer(config *ScorerConfig) URLScorer {
	var scorers []weightedScorer

	if len(config.Keywords) > 0 && config.KeywordWeight > 0 {
		scorers = append(scorers, weightedScorer{
			scorer: NewKeywordRelevanceScorer(config.Keywords),
			weight: config.KeywordWeight,
		})
	}
	if config.FreshnessWeight > 0 {
		scorers = append(scorers, weightedScorer{
			scorer: NewFreshnessScorer(),
			weight: config.FreshnessWeight,
		})
	}
	if config.DepthWeight > 0 {
		scorers = append(scorers, weightedScorer{
			scorer: NewPathDepthScorer(2),
			weight: config.DepthWeight,
		})
	}

	if len(scorers) == 0 {
		return nil
	}
	return NewCompositeScorer(scorers)
}
