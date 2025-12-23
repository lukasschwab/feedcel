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

	http.HandleFunc("/filter", new(Filterer).Handle)
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

	if urlStr == "" || exprStr == "" {
		http.Error(w, "Missing 'url' or 'expression' parameter", http.StatusBadRequest)
		return
	}

	parsed, err := f.parser.ParseURLWithContext(urlStr, r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch feed: %v", err), http.StatusBadGateway)
	}

	filtered, err := f.Filter(parsed, time.Now(), urlStr, exprStr)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to filter feed: %v", err), http.StatusInternalServerError)
	}

	// TODO: render this as a feed insteadd of simple JSON encoding.
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(filtered); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
