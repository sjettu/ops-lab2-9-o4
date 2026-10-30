package main

import (
	"reflect"
	"testing"
)

func TestAnalyzeCountsAndSorts(t *testing.T) {
	got := Analyze("Hello, HELLO world! 42 42")
	want := Analysis{
		WordCount: 5, UniqueWords: 3,
		TopWords: []WordCount{{"42", 2}, {"hello", 2}, {"world", 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestAnalyzeUnicode(t *testing.T) {
	got := Analyze("Café CAFÉ Go")
	want := Analysis{
		WordCount: 3, UniqueWords: 2,
		TopWords: []WordCount{{"café", 2}, {"go", 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestAnalyzeTopTen(t *testing.T) {
	got := Analyze("l k j i h g f e d c b a")
	if got.WordCount != 12 || got.UniqueWords != 12 || len(got.TopWords) != 10 {
		t.Fatalf("unexpected counts: %#v", got)
	}
	if got.TopWords[0].Word != "a" || got.TopWords[9].Word != "j" {
		t.Fatalf("unstable tie ordering: %#v", got.TopWords)
	}
}

func TestAnalyzeEmpty(t *testing.T) {
	got := Analyze(" \n\t ")
	if got.WordCount != 0 || got.UniqueWords != 0 || got.TopWords == nil || len(got.TopWords) != 0 {
		t.Fatalf("empty analysis must contain an empty array: %#v", got)
	}
}
