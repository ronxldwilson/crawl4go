package content

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

type DiffResult struct {
	Added      []string `json:"added"`
	Removed    []string `json:"removed"`
	Unchanged  int      `json:"unchanged"`
	TotalOld   int      `json:"total_old"`
	TotalNew   int      `json:"total_new"`
	Similarity float64  `json:"similarity"`
}

func DiffText(oldText, newText string) DiffResult {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	oldSet := make(map[string]int)
	for _, line := range oldLines {
		t := strings.TrimSpace(line)
		if t != "" {
			oldSet[t]++
		}
	}

	newSet := make(map[string]int)
	for _, line := range newLines {
		t := strings.TrimSpace(line)
		if t != "" {
			newSet[t]++
		}
	}

	var added, removed []string
	unchanged := 0

	for line, count := range newSet {
		oldCount := oldSet[line]
		if oldCount == 0 {
			for range count {
				added = append(added, line)
			}
		} else if count > oldCount {
			for range count - oldCount {
				added = append(added, line)
			}
			unchanged += oldCount
		} else {
			unchanged += count
		}
	}

	for line, count := range oldSet {
		newCount := newSet[line]
		if newCount == 0 {
			for range count {
				removed = append(removed, line)
			}
		} else if count > newCount {
			for range count - newCount {
				removed = append(removed, line)
			}
		}
	}

	totalOld := len(oldSet)
	totalNew := len(newSet)
	total := totalOld + totalNew
	similarity := 0.0
	if total > 0 {
		similarity = float64(2*unchanged) / float64(total)
	}

	return DiffResult{
		Added:      added,
		Removed:    removed,
		Unchanged:  unchanged,
		TotalOld:   totalOld,
		TotalNew:   totalNew,
		Similarity: similarity,
	}
}

func ContentHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}
