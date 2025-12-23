package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/lukasschwab/feedcel/pkg/cel"
	"github.com/lukasschwab/feedcel/pkg/feed"
)

func main() {
	port := flag.Int("port", 8080, "Port to listen on")
	flag.Parse()

	http.HandleFunc("/filter", handleFilter)
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

func handleFilter(w http.ResponseWriter, r *http.Request) {
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

	// 1. Fetch and Parse Feed
	ctx := r.Context()
	items, err := feed.FetchAndParse(ctx, urlStr)
	if err != nil {
		log.Printf("Error fetching feed: %v", err)
		http.Error(w, "Failed to fetch feed: "+err.Error(), http.StatusBadGateway)
		return
	}

	// 2. Setup CEL
	env, err := cel.NewEnv()
	if err != nil {
		log.Printf("Error creating CEL env: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	prg, err := cel.Compile(env, exprStr)
	if err != nil {
		log.Printf("Error compiling expression: %v", err)
		http.Error(w, "Invalid CEL expression: "+err.Error(), http.StatusBadRequest)
		return
	}

	// 3. Filter Items
	now := time.Now()
	filteredItems := make([]cel.Item, 0)
	for _, item := range items {
		match, err := cel.Evaluate(prg, item, now)
		if err != nil {
			log.Printf("Error evaluating item %s: %v", item.URL, err)
			continue
		}
		if match {
			filteredItems = append(filteredItems, item)
		}
	}

	// 4. Return Result
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(filteredItems); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}
