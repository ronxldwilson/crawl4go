package content

import (
	"math"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// SEOScore holds the individual and total SEO scores for a URL.
type SEOScore struct {
	URL           string  `json:"url"`
	TitleScore    float64 `json:"title_score"`
	MetaDescScore float64 `json:"meta_desc_score"`
	HeadingScore  float64 `json:"heading_score"`
	ContentScore  float64 `json:"content_score"`
	TotalScore    float64 `json:"total_score"`
}

// SEOFilter scores and filters URLs based on on-page SEO signals.
type SEOFilter struct {
	MinScore float64
}

// DefaultSEOFilter returns an SEOFilter with a minimum score of 0.3.
func DefaultSEOFilter() *SEOFilter {
	return &SEOFilter{MinScore: 0.3}
}

// ScoreURL parses the HTML and returns an SEOScore based on the presence
// and quality of title, meta description, headings, and content.
func (f *SEOFilter) ScoreURL(url, htmlContent string) SEOScore {
	s := SEOScore{URL: url}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return s
	}

	var (
		title     string
		metaDesc  string
		h1Count   int
		h2Count   int
		bodyText  strings.Builder
	)

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Title:
				if title == "" {
					title = ExtractText(n)
				}
			case atom.Meta:
				name := strings.ToLower(GetAttr(n, "name"))
				if name == "description" && metaDesc == "" {
					metaDesc = GetAttr(n, "content")
				}
			case atom.H1:
				h1Count++
			case atom.H2:
				h2Count++
			case atom.Script, atom.Style, atom.Noscript:
				return
			}
		}
		if n.Type == html.TextNode {
			bodyText.WriteString(n.Data)
			bodyText.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Title scoring: present and between 30-60 chars is ideal.
	if title != "" {
		tLen := len(title)
		switch {
		case tLen >= 30 && tLen <= 60:
			s.TitleScore = 1.0
		case tLen >= 10 && tLen < 30:
			s.TitleScore = 0.6
		case tLen > 60 && tLen <= 90:
			s.TitleScore = 0.7
		case tLen > 90:
			s.TitleScore = 0.4
		default:
			s.TitleScore = 0.3
		}
	}

	// Meta description scoring: present and between 120-160 chars is ideal.
	if metaDesc != "" {
		mLen := len(metaDesc)
		switch {
		case mLen >= 120 && mLen <= 160:
			s.MetaDescScore = 1.0
		case mLen >= 50 && mLen < 120:
			s.MetaDescScore = 0.6
		case mLen > 160 && mLen <= 200:
			s.MetaDescScore = 0.7
		case mLen > 200:
			s.MetaDescScore = 0.4
		default:
			s.MetaDescScore = 0.3
		}
	}

	// Heading scoring: at least one H1 is important, H2s help structure.
	switch {
	case h1Count == 1 && h2Count >= 2:
		s.HeadingScore = 1.0
	case h1Count == 1 && h2Count >= 1:
		s.HeadingScore = 0.8
	case h1Count == 1:
		s.HeadingScore = 0.6
	case h1Count > 1:
		s.HeadingScore = 0.4
	case h2Count > 0:
		s.HeadingScore = 0.3
	}

	// Content scoring: based on word count and keyword density.
	fullText := bodyText.String()
	words := strings.Fields(fullText)
	wordCount := len(words)

	switch {
	case wordCount >= 800:
		s.ContentScore = 1.0
	case wordCount >= 300:
		s.ContentScore = 0.7
	case wordCount >= 100:
		s.ContentScore = 0.4
	case wordCount > 0:
		s.ContentScore = 0.2
	}

	// Keyword density bonus: if the title appears to be reflected in content.
	if title != "" && wordCount > 0 {
		titleWords := strings.Fields(strings.ToLower(title))
		lowerText := strings.ToLower(fullText)
		matches := 0
		for _, tw := range titleWords {
			if len(tw) >= 3 && strings.Contains(lowerText, tw) {
				matches++
			}
		}
		if len(titleWords) > 0 {
			density := float64(matches) / float64(len(titleWords))
			s.ContentScore = math.Min(1.0, s.ContentScore+density*0.2)
		}
	}

	// Total score is the weighted average.
	s.TotalScore = (s.TitleScore*0.25 + s.MetaDescScore*0.20 +
		s.HeadingScore*0.25 + s.ContentScore*0.30)

	return s
}

// FilterURLs returns only the SEOScores that meet or exceed MinScore.
func (f *SEOFilter) FilterURLs(scores []SEOScore) []SEOScore {
	var kept []SEOScore
	for _, s := range scores {
		if s.TotalScore >= f.MinScore {
			kept = append(kept, s)
		}
	}
	return kept
}
