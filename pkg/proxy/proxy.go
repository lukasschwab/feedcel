package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/lukasschwab/feedcel/pkg/cel"
	gf "github.com/lukasschwab/feedcel/pkg/gofeed"
	"github.com/mmcdole/gofeed"
)

type FilterRequest struct {
	URL         string   `json:"url"`
	Expression  string   `json:"expression,omitempty"`
	Expressions []string `json:"expressions,omitempty"`
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
	env    cel.Env
}

func (f *Filterer) Handle(w http.ResponseWriter, r *http.Request) {
	var req FilterRequest
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}
	} else {
		req.URL = r.URL.Query().Get("url")
		req.Expression = r.URL.Query().Get("expression")
	}

	if req.URL == "" {
		http.Error(w, "Missing 'url' parameter", http.StatusBadRequest)
		return
	}

	// Build expression pipeline: Expression (backward compat) then Expressions.
	var exprs []string
	if req.Expression != "" {
		exprs = append(exprs, req.Expression)
	}
	exprs = append(exprs, req.Expressions...)
	if len(exprs) == 0 {
		exprs = []string{"true"}
	}

	// Compile all expressions up front.
	prgs := make([]cel.Program, 0, len(exprs))
	for _, expr := range exprs {
		prg, err := f.env.Compile(expr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid expression %q: %v", expr, err), http.StatusInternalServerError)
			return
		}
		prgs = append(prgs, prg)
	}

	// Parse feed.
	parsed, err := f.parser.ParseURLWithContext(req.URL, r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch feed: %v", err), http.StatusBadGateway)
		return
	}

	// Apply pipeline.
	now := time.Now()
	for _, prg := range prgs {
		gf.Apply(prg, parsed, now)
	}

	outFeed := toGorillaFeed(parsed)

	// Determine output format. Default to JSON.
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
