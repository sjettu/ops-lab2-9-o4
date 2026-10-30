package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeHTTP(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
		status int
	}{
		{"valid", http.MethodPost, `{"text":"Hello hello world"}`, http.StatusOK},
		{"whitespace after JSON", http.MethodPost, "{\"text\":\"hello\"} \n", http.StatusOK},
		{"invalid JSON", http.MethodPost, `{"text":`, http.StatusBadRequest},
		{"empty text", http.MethodPost, `{"text":"  "}`, http.StatusBadRequest},
		{"missing text", http.MethodPost, `{}`, http.StatusBadRequest},
		{"unknown field", http.MethodPost, `{"text":"hi","extra":1}`, http.StatusBadRequest},
		{"two JSON values", http.MethodPost, `{"text":"hi"} {"text":"bye"}`, http.StatusBadRequest},
		{"trailing garbage", http.MethodPost, `{"text":"hi"} no`, http.StatusBadRequest},
		{"wrong method", http.MethodGet, "", http.StatusMethodNotAllowed},
		{"oversized body", http.MethodPost, `{"text":"` + strings.Repeat("a", maxBodyBytes) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "/api/analyze", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			newMux().ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d, want=%d; body=%s", response.Code, test.status, response.Body.String())
			}
			if test.status == http.StatusOK {
				var result Analysis
				if err := json.NewDecoder(response.Body).Decode(&result); err != nil || result.WordCount == 0 {
					t.Fatalf("invalid result: %#v; err=%v", result, err)
				}
			}
		})
	}
}

func TestHealthAndVersion(t *testing.T) {
	for _, path := range []string{"/healthz", "/version"} {
		response := httptest.NewRecorder()
		newMux().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, response.Code)
		}
		if path == "/healthz" && response.Body.String() != "ok\n" {
			t.Fatalf("unexpected health response: %q", response.Body.String())
		}
		if path == "/version" {
			var info versionInfo
			if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
				t.Fatal(err)
			}
			if info.Name != "textstats" || info.PID <= 0 || info.GOMAXPROCS <= 0 || info.Go == "" {
				t.Fatalf("missing runtime provenance: %#v", info)
			}
		}
	}
}

func TestHTTPRoundTrip(t *testing.T) {
	server := httptest.NewServer(newMux())
	defer server.Close()
	client := server.Client()
	client.Timeout = 2 * time.Second
	response, err := client.Post(server.URL+"/api/analyze", "application/json",
		strings.NewReader(`{"text":"Hello hello world"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result Analysis
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || result.WordCount != 3 || result.UniqueWords != 2 {
		t.Fatalf("unexpected response: status=%d, result=%#v", response.StatusCode, result)
	}
}

func TestUnknownRoute(t *testing.T) {
	response := httptest.NewRecorder()
	newMux().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	body, _ := io.ReadAll(response.Result().Body)
	if response.Code != http.StatusNotFound || len(body) == 0 {
		t.Fatalf("unexpected unknown route response: %d %s", response.Code, body)
	}
}
