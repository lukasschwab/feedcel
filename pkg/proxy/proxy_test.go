package proxy_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lukasschwab/feedcel/pkg/proxy"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mockRSS = `<?xml version="1.0" encoding="UTF-8" ?>
<rss version="2.0">
<channel>
  <title>Test Feed</title>
  <item>
    <title>Go is great</title>
    <link>http://example.com/go</link>
    <description>Go description</description>
  </item>
  <item>
    <title>Rust is safe</title>
    <link>http://example.com/rust</link>
    <description>Rust description</description>
  </item>
</channel>
</rss>`

type mockTransport struct {
	responseBody string
	statusCode   int
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(strings.NewReader(m.responseBody)),
		Header:     make(http.Header),
	}, nil
}

type testCase struct {
	name           string
	method         string
	urlParams      map[string]string
	body           any
	wantStatusCode int
	wantItems      int
	wantFormat     string // "json", "rss", "atom"
}

func (tc testCase) request(t *testing.T) *http.Request {
	var req *http.Request
	if tc.method == http.MethodPost {
		jsonBody, err := json.Marshal(tc.body)
		require.NoError(t, err)
		req = httptest.NewRequest(tc.method, "/filter", bytes.NewReader(jsonBody))
	} else {
		req = httptest.NewRequest(tc.method, "/filter", nil)
		q := req.URL.Query()
		for k, v := range tc.urlParams {
			q.Add(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}
	return req
}

func TestHandle(t *testing.T) {
	// Setup Filterer with mock client
	client := &http.Client{
		Transport: &mockTransport{
			responseBody: mockRSS,
			statusCode:   http.StatusOK,
		},
	}
	f, err := proxy.NewFilterer(client)
	if err != nil {
		t.Fatalf("NewFilterer failed: %v", err)
	}

	tests := []testCase{
		{
			name:   "GET simple filter",
			method: http.MethodGet,
			urlParams: map[string]string{
				"url":        "http://mock/feed",
				"expression": `item.Title.contains("Go")`,
			},
			wantStatusCode: http.StatusOK,
			wantItems:      1,
			wantFormat:     "json",
		},
		{
			name:   "POST simple filter",
			method: http.MethodPost,
			body: proxy.FilterRequest{
				URL:        "http://mock/feed",
				Expression: `item.Title.contains("Rust")`,
			},
			wantStatusCode: http.StatusOK,
			wantItems:      1,
			wantFormat:     "json",
		},
		{
			name:   "GET empty expression",
			method: http.MethodGet,
			urlParams: map[string]string{
				"url": "http://mock/feed",
			},
			wantStatusCode: http.StatusOK,
			wantItems:      2,
			wantFormat:     "json",
		},
		{
			name:   "GET no match",
			method: http.MethodGet,
			urlParams: map[string]string{
				"url":        "http://mock/feed",
				"expression": `item.Title.contains("Java")`,
			},
			wantStatusCode: http.StatusOK,
			wantItems:      0,
			wantFormat:     "json",
		},
		{
			name:   "GET format rss",
			method: http.MethodGet,
			urlParams: map[string]string{
				"url":        "http://mock/feed",
				"expression": "true",
				"format":     "rss",
			},
			wantStatusCode: http.StatusOK,
			wantItems:      2,
			wantFormat:     "rss",
		},
		{
			name:   "GET format atom",
			method: http.MethodGet,
			urlParams: map[string]string{
				"url":        "http://mock/feed",
				"expression": "true",
				"format":     "atom",
			},
			wantStatusCode: http.StatusOK,
			wantItems:      2,
			wantFormat:     "atom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			f.Handle(w, tt.request(t))

			resp := w.Result()
			assert.Equal(t, tt.wantStatusCode, resp.StatusCode)
			if resp.StatusCode != http.StatusOK {
				return
			}

			// Verify Content-Type
			switch tt.wantFormat {
			case "json":
				assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			case "rss":
				assert.Equal(t, "application/rss+xml", resp.Header.Get("Content-Type"))
			case "atom":
				assert.Equal(t, "application/atom+xml", resp.Header.Get("Content-Type"))
			}

			fp := gofeed.NewParser()
			parsedFeed, err := fp.Parse(resp.Body)
			require.NoError(t, err)
			assert.Len(t, parsedFeed.Items, tt.wantItems)
		})
	}
}
