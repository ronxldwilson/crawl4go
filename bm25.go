package main

import (
	"math"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/kljensen/snowball"
)

var tagPriorityWeights = map[string]float64{
	"h1": 5.0, "h2": 4.0, "h3": 3.0, "h4": 2.5, "h5": 2.0, "h6": 2.0,
	"title": 4.0, "strong": 2.0, "b": 1.5, "em": 1.5, "code": 2.0,
	"pre": 1.5, "th": 1.5, "blockquote": 2.0,
}

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"is": true, "it": true, "be": true, "as": true, "was": true, "with": true,
	"by": true, "that": true, "this": true, "from": true, "are": true, "were": true,
	"been": true, "have": true, "has": true, "had": true, "not": true, "no": true,
	"do": true, "does": true, "did": true, "will": true, "would": true,
	"can": true, "could": true, "should": true, "may": true, "might": true,
	"i": true, "you": true, "he": true, "she": true, "we": true, "they": true,
	"me": true, "him": true, "her": true, "us": true, "them": true,
	"my": true, "your": true, "his": true, "its": true, "our": true, "their": true,
}

type TextChunk struct {
	Index   int
	Text    string
	TagName string
	Tokens  []string
}

type BM25Filter struct {
	K1        float64
	B         float64
	Threshold float64
}

func NewBM25Filter() *BM25Filter {
	return &BM25Filter{K1: 2.0, B: 0.75, Threshold: 1.0}
}

// FilterByRelevance scores and filters text chunks by BM25 relevance to a query.
func (f *BM25Filter) FilterByRelevance(chunks []TextChunk, query string) []TextChunk {
	if len(chunks) == 0 || query == "" {
		return chunks
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return chunks
	}

	// Compute average document length
	totalLen := 0
	for i := range chunks {
		chunks[i].Tokens = tokenize(chunks[i].Text)
		totalLen += len(chunks[i].Tokens)
	}
	avgDL := float64(totalLen) / float64(len(chunks))

	// Compute document frequency for each query term
	df := make(map[string]int)
	for _, qt := range queryTokens {
		for _, chunk := range chunks {
			for _, t := range chunk.Tokens {
				if t == qt {
					df[qt]++
					break
				}
			}
		}
	}

	n := float64(len(chunks))
	var filtered []TextChunk

	for _, chunk := range chunks {
		score := 0.0

		// Term frequency map for this chunk
		tf := make(map[string]int)
		for _, t := range chunk.Tokens {
			tf[t]++
		}

		dl := float64(len(chunk.Tokens))

		for _, qt := range queryTokens {
			termFreq := float64(tf[qt])
			if termFreq == 0 {
				continue
			}
			docFreq := float64(df[qt])

			// IDF
			idf := math.Log((n-docFreq+0.5)/(docFreq+0.5) + 1)

			// BM25 TF component
			tfNorm := (termFreq * (f.K1 + 1)) / (termFreq + f.K1*(1-f.B+f.B*dl/avgDL))

			score += idf * tfNorm
		}

		// Apply tag priority weight
		if weight, ok := tagPriorityWeights[chunk.TagName]; ok {
			score *= weight
		}

		if score >= f.Threshold {
			filtered = append(filtered, chunk)
		}
	}

	return filtered
}

// ExtractTextChunks walks an HTML tree and extracts text blocks with their tag context.
func ExtractTextChunks(htmlContent string) []TextChunk {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil
	}

	var chunks []TextChunk
	index := 0

	var walk func(*html.Node, string)
	walk = func(n *html.Node, parentTag string) {
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if len(strings.Fields(text)) >= 2 {
				chunks = append(chunks, TextChunk{
					Index:   index,
					Text:    text,
					TagName: parentTag,
				})
				index++
			}
			return
		}

		if n.Type == html.ElementNode {
			tag := n.Data
			if isBlockElement(n) {
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c, tag)
				}
				return
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, parentTag)
		}
	}

	body := findBody(doc)
	if body == nil {
		body = doc
	}
	walk(body, "body")

	return chunks
}

// ExtractPageQuery extracts a search query from page metadata as a fallback.
func ExtractPageQuery(htmlContent string) string {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return ""
	}

	var title, h1, metaKeywords, metaDesc, firstParagraph string

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Title:
				if title == "" {
					title = extractText(n)
				}
			case atom.H1:
				if h1 == "" {
					h1 = extractText(n)
				}
			case atom.Meta:
				name := strings.ToLower(getAttr(n, "name"))
				content := getAttr(n, "content")
				if name == "keywords" && metaKeywords == "" {
					metaKeywords = content
				}
				if name == "description" && metaDesc == "" {
					metaDesc = content
				}
			case atom.P:
				if firstParagraph == "" {
					text := extractText(n)
					if len(text) > 150 {
						firstParagraph = text
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if title != "" {
		return title
	}
	if h1 != "" {
		return h1
	}
	if metaKeywords != "" {
		return metaKeywords
	}
	if metaDesc != "" {
		return metaDesc
	}
	return firstParagraph
}

func tokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var tokens []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}")
		if len(w) < 2 || stopWords[w] {
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

func isBlockElement(n *html.Node) bool {
	switch n.DataAtom {
	case atom.P, atom.Div, atom.Section, atom.Article, atom.Main,
		atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6,
		atom.Blockquote, atom.Pre, atom.Ul, atom.Ol, atom.Li,
		atom.Table, atom.Tr, atom.Td, atom.Th,
		atom.Dl, atom.Dt, atom.Dd, atom.Figure, atom.Figcaption,
		atom.Details, atom.Summary:
		return true
	}
	return false
}
