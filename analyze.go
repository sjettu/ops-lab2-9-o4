package main

import (
	"sort"
	"strings"
	"unicode"
)

const topLimit = 10

type WordCount struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

type Analysis struct {
	WordCount   int         `json:"word_count"`
	UniqueWords int         `json:"unique_words"`
	TopWords    []WordCount `json:"top_words"`
}

// Analyze counts case-insensitive runs of Unicode letters and digits.
// Ties are ordered lexicographically so identical inputs produce identical output.
func Analyze(text string) Analysis {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	counts := make(map[string]int)
	for _, word := range words {
		counts[word]++
	}
	top := make([]WordCount, 0, len(counts))
	for word, count := range counts {
		top = append(top, WordCount{Word: word, Count: count})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Count != top[j].Count {
			return top[i].Count > top[j].Count
		}
		return top[i].Word < top[j].Word
	})
	if len(top) > topLimit {
		top = top[:topLimit]
	}
	return Analysis{WordCount: len(words), UniqueWords: len(counts), TopWords: top}
}
