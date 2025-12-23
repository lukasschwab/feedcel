package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"slices"
	"time"

	celgo "github.com/google/cel-go/cel"
	"github.com/lukasschwab/feedcel/pkg/cel"
	"github.com/lukasschwab/feedcel/pkg/feed"
	"github.com/mmcdole/gofeed"
)

func main() {
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	filterer, err := NewFilterer(nil)
	if err != nil {
		log.Fatalf("Failed to initialize filterer: %v", err)
	}

	http.HandleFunc("/filter", filterer.Handle)
	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting proxy server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

type FilterRequest struct {
	URL        string `json:"url"`
	Expression string `json:"expression"`
}

func NewFilterer(client *http.Client) (*Filterer, error) {
	fp := gofeed.NewParser()
	if client != nil {
		fp.Client = client
	}

	env, err := cel.NewEnv()
	if err != nil {
		log.Printf("Error creating CEL env: %v", err)
		return nil, err
	}

	return &Filterer{
		parser: fp,
		env:    env,
	}, nil
}

type Filterer struct {
	parser *gofeed.Parser
	env    *celgo.Env
}

func (f *Filterer) Filter(
	parsed *gofeed.Feed,
	now time.Time,
	url, expr string,
) (*gofeed.Feed, error) {
	prg, err := cel.Compile(f.env, expr)
	if err != nil {
		return nil, err
	}
	parsed.Items = slices.DeleteFunc(parsed.Items, func(i *gofeed.Item) bool {
		celItem := feed.Transform(i)
		match, err := cel.Evaluate(prg, celItem, now)
		if err != nil {
			log.Printf("Evaluation failed for item '%v': %v", i.GUID, err)
		}
		// DeleteFunc deletes when predicate is true.
		return !match
	})
	return parsed, nil
}

// TODO: do we have a sensible behavior when there isn't a cel expression provided?
func (f *Filterer) Handle(w http.ResponseWriter, r *http.Request) {
	// Support both GET query params and POST JSON body
	var urlStr, exprStr string
	if r.Method == http.MethodPost {
		var req FilterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
		urlStr = req.URL
		exprStr = req.Expression
	} else {
		urlStr = r.URL.Query().Get("url")
		exprStr = r.URL.Query().Get("expression")
	}

	if urlStr == "" {
		http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
		return
	}
	if exprStr == "" {
		exprStr = "true"
	}

	parsed, err := f.parser.ParseURLWithContext(urlStr, r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch feed: %v", err), http.StatusBadGateway)
	}

	filtered, err := f.Filter(parsed, time.Now(), urlStr, exprStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to filter feed: %v", err), http.StatusInternalServerError)
	}

	outFeed := ToGorillaFeed(filtered)

	// Determine output format. Default to JSON.
	// We can support an optional 'format' query param: rss, atom, json
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	var content string
	var contentType string
	var encodeErr error

	switch format {
	case "atom":
		contentType = "application/atom+xml"
		content, encodeErr = outFeed.ToAtom()
	case "rss":
		contentType = "application/rss+xml"
		content, encodeErr = outFeed.ToRss()
	case "json":
		fallthrough
	default:
		contentType = "application/json"
		content, encodeErr = outFeed.ToJSON()
	}

	if encodeErr != nil {
		http.Error(w, fmt.Sprintf("failed to encode feed: %v", encodeErr), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", contentType)
	if _, err := w.Write([]byte(content)); err != nil {
		log.Printf("Error writing response: %v", err)
	}
}
