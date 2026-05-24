package content

import (
	"math"
	"strconv"
	"strings"
)

// ImageScorer scores and filters ScrapedImage instances based on configurable
// criteria such as dimensions, aspect ratio, alt text, and URL patterns.
type ImageScorer struct {
	MinWidth        int      `json:"min_width"`
	MinHeight       int      `json:"min_height"`
	MinAspectRatio  float64  `json:"min_aspect_ratio"`
	MaxAspectRatio  float64  `json:"max_aspect_ratio"`
	BlockedPatterns []string `json:"blocked_patterns"` // substrings in URL that indicate ad/tracking images
}

// DefaultImageScorer returns an ImageScorer with sensible defaults.
func DefaultImageScorer() *ImageScorer {
	return &ImageScorer{
		MinWidth:       50,
		MinHeight:      50,
		MinAspectRatio: 0.2,
		MaxAspectRatio: 5.0,
		BlockedPatterns: []string{
			"pixel", "tracker", "beacon", "analytics",
			"1x1", "spacer", "blank.gif", "ad-",
			"doubleclick", "googlesyndication",
			"facebook.com/tr", "ads.",
		},
	}
}

// ScoreImage computes a relevance score in [0.0, 1.0] for an image.
// Scoring considers dimensions, aspect ratio, alt text presence, and URL patterns.
func (s *ImageScorer) ScoreImage(img ScrapedImage) float64 {
	// Check blocked URL patterns first — blocked images score 0.
	urlLower := strings.ToLower(img.URL)
	for _, pat := range s.BlockedPatterns {
		if strings.Contains(urlLower, strings.ToLower(pat)) {
			return 0
		}
	}

	var score float64

	// Dimension scoring (max 0.35).
	w, _ := strconv.Atoi(img.Width)
	h, _ := strconv.Atoi(img.Height)

	if w > 0 && h > 0 {
		// Penalise images below minimum dimensions.
		if w < s.MinWidth || h < s.MinHeight {
			score += 0.05 // very small bonus — might still be relevant
		} else {
			// Larger images get higher scores (log-scaled).
			area := float64(w * h)
			dimScore := math.Log(area+1) / 15.0
			if dimScore > 0.35 {
				dimScore = 0.35
			}
			score += dimScore
		}

		// Aspect ratio scoring (max 0.2).
		aspect := float64(w) / float64(h)
		if aspect >= s.MinAspectRatio && aspect <= s.MaxAspectRatio {
			score += 0.2
		} else {
			score += 0.05 // unusual aspect ratio
		}
	} else {
		// Unknown dimensions — give moderate baseline.
		score += 0.15
	}

	// Alt text presence (max 0.25).
	alt := strings.TrimSpace(img.Alt)
	if alt != "" {
		score += 0.15
		// Bonus for descriptive alt text (more than a few words).
		if len(strings.Fields(alt)) >= 3 {
			score += 0.10
		}
	}

	// URL quality heuristic (max 0.2).
	// Reward images from common content paths, penalise icon/logo patterns.
	iconPatterns := []string{"icon", "logo", "avatar", "sprite", "favicon", "badge", "button", "arrow", "bullet"}
	isIcon := false
	for _, pat := range iconPatterns {
		if strings.Contains(urlLower, pat) {
			isIcon = true
			break
		}
	}
	if !isIcon {
		score += 0.2
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// FilterImages returns only images whose score meets or exceeds minScore.
func (s *ImageScorer) FilterImages(images []ScrapedImage, minScore float64) []ScrapedImage {
	var kept []ScrapedImage
	for _, img := range images {
		sc := s.ScoreImage(img)
		if sc >= minScore {
			img.Score = sc
			kept = append(kept, img)
		}
	}
	if kept == nil {
		kept = []ScrapedImage{}
	}
	return kept
}
