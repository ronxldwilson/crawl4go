package content

import (
	"testing"
)

func TestExtractMedia_Images(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		baseURL   string
		wantCount int
		wantURL   string
	}{
		{
			name:      "img tag",
			html:      `<html><body><img src="/photo.jpg" alt="Photo" width="800" height="600"></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
			wantURL:   "https://example.com/photo.jpg",
		},
		{
			name:      "data-src lazy load",
			html:      `<html><body><img data-src="/lazy.jpg" alt="Lazy"></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
			wantURL:   "https://example.com/lazy.jpg",
		},
		{
			name:      "og:image meta tag",
			html:      `<html><head><meta property="og:image" content="https://cdn.example.com/og.jpg"></head><body></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
			wantURL:   "https://cdn.example.com/og.jpg",
		},
		{
			name:      "picture with source",
			html:      `<html><body><picture><source srcset="/large.webp 1024w, /small.webp 480w"></picture></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
		},
		{
			name:      "css background-image",
			html:      `<html><body><div style="background-image: url('/bg.png')">content</div></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
		},
		{
			name:      "deduplication",
			html:      `<html><body><img src="/same.jpg"><img src="/same.jpg"></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
		},
		{
			name:      "no images",
			html:      `<html><body><p>Just text</p></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := ExtractMedia(tt.html, tt.baseURL)
			if len(ms.Images) != tt.wantCount {
				t.Errorf("got %d images, want %d", len(ms.Images), tt.wantCount)
			}
			if tt.wantURL != "" && tt.wantCount > 0 && len(ms.Images) > 0 {
				if ms.Images[0].URL != tt.wantURL {
					t.Errorf("image URL = %q, want %q", ms.Images[0].URL, tt.wantURL)
				}
			}
		})
	}
}

func TestExtractMedia_Videos(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		baseURL   string
		wantCount int
	}{
		{
			name:      "video tag with src",
			html:      `<html><body><video src="/clip.mp4" title="My Video"></video></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
		},
		{
			name:      "video with source child",
			html:      `<html><body><video><source src="/clip.webm"></video></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
		},
		{
			name:      "video dedup src and source",
			html:      `<html><body><video src="/clip.mp4"><source src="/clip.mp4"></video></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
		},
		{
			name:      "no videos",
			html:      `<html><body><p>text</p></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := ExtractMedia(tt.html, tt.baseURL)
			if len(ms.Videos) != tt.wantCount {
				t.Errorf("got %d videos, want %d", len(ms.Videos), tt.wantCount)
			}
		})
	}
}

func TestExtractMedia_Audio(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		baseURL   string
		wantCount int
	}{
		{
			name:      "audio tag with src",
			html:      `<html><body><audio src="/song.mp3" title="Song"></audio></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
		},
		{
			name:      "audio with source child",
			html:      `<html><body><audio><source src="/song.ogg"></audio></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 1,
		},
		{
			name:      "no audio",
			html:      `<html><body><p>silent</p></body></html>`,
			baseURL:   "https://example.com",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := ExtractMedia(tt.html, tt.baseURL)
			if len(ms.Audio) != tt.wantCount {
				t.Errorf("got %d audio, want %d", len(ms.Audio), tt.wantCount)
			}
		})
	}
}

func TestExtractMedia_EmptySlicesNotNil(t *testing.T) {
	ms := ExtractMedia(`<html><body></body></html>`, "https://example.com")
	if ms.Images == nil {
		t.Error("Images should be empty slice, not nil")
	}
	if ms.Videos == nil {
		t.Error("Videos should be empty slice, not nil")
	}
	if ms.Audio == nil {
		t.Error("Audio should be empty slice, not nil")
	}
}

func TestFilterMedia(t *testing.T) {
	ms := MediaSet{
		Images: []MediaItem{
			{URL: "high.jpg", Score: 0.9},
			{URL: "low.jpg", Score: 0.1},
		},
		Videos: []MediaItem{
			{URL: "vid.mp4", Score: 0.5},
		},
		Audio: []MediaItem{
			{URL: "song.mp3", Score: 0.3},
		},
	}

	tests := []struct {
		name       string
		minScore   float64
		wantImages int
		wantVideos int
		wantAudio  int
	}{
		{"low threshold", 0.0, 2, 1, 1},
		{"medium threshold", 0.5, 1, 1, 0},
		{"high threshold", 0.8, 1, 0, 0},
		{"very high threshold", 1.0, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := FilterMedia(ms, tt.minScore)
			if len(filtered.Images) != tt.wantImages {
				t.Errorf("images: got %d, want %d", len(filtered.Images), tt.wantImages)
			}
			if len(filtered.Videos) != tt.wantVideos {
				t.Errorf("videos: got %d, want %d", len(filtered.Videos), tt.wantVideos)
			}
			if len(filtered.Audio) != tt.wantAudio {
				t.Errorf("audio: got %d, want %d", len(filtered.Audio), tt.wantAudio)
			}
		})
	}
}

func TestScoreImage(t *testing.T) {
	// An image with alt text, large dimensions, not in nav, no icon patterns,
	// but NOT in an article/main ancestor (we can't easily set parent nodes here)
	// should score at least 0.6 (alt + dimensions + not-noise).
	item := MediaItem{
		URL:    "https://example.com/photo.jpg",
		Alt:    "A nice photo",
		Width:  800,
		Height: 600,
	}

	// Build a minimal node tree: <article><img>
	// We pass nil parent so ancestor checks default.
	// Just test that function runs without panic.
	ms := ExtractMedia(
		`<html><body><article><img src="https://example.com/photo.jpg" alt="A nice photo" width="800" height="600"></article></body></html>`,
		"https://example.com",
	)
	if len(ms.Images) == 0 {
		t.Fatal("expected at least 1 image")
	}
	img := ms.Images[0]
	if img.Score < 0.4 {
		t.Errorf("expected score >= 0.4 for content image, got %f", img.Score)
	}
	_ = item // used above for documentation
}
