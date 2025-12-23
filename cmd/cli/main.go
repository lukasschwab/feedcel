package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/lukasschwab/feedcel/pkg/cel"
	"github.com/lukasschwab/feedcel/pkg/feed"
	"github.com/mmcdole/gofeed"
)

const (
	ANSIGray   = "\033[90m"
	ANSIYellow = "\033[33m"
	ANSIReset  = "\033[0m"
)

func main() {
	feedRef := flag.String("feed", "", "URL or path to the feed")
	expr := flag.String("expr", "", "CEL expression to filter items")
	flag.Parse()

	if *feedRef == "" || *expr == "" {
		fmt.Println("Usage: feedcel -feed <url_or_path> -expr <cel_expression>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	fp := gofeed.NewParser()
	var parsed *gofeed.Feed
	var err error

	// Try opening as file first
	if f, errFile := os.Open(*feedRef); errFile == nil {
		defer f.Close()
		parsed, err = fp.Parse(f)
	} else {
		// Assume URL
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		parsed, err = fp.ParseURLWithContext(*feedRef, ctx)
	}

	if err != nil {
		log.Fatalf("Failed to parse feed: %v", err)
	}

	env, err := cel.NewEnv()
	if err != nil {
		log.Fatalf("Failed to create CEL env: %v", err)
	}

	prg, err := cel.Compile(env, *expr)
	if err != nil {
		fmt.Printf("Invalid CEL expression: %v\n", err)
		os.Exit(1)
	}

	var filtered []*gofeed.Item
	now := time.Now()
	for _, item := range parsed.Items {
		celItem := feed.Transform(item)
		match, err := cel.Evaluate(prg, celItem, now)
		if err != nil {
			log.Printf("Evaluation error for item '%s': %v", item.Title, err)
			continue
		}
		if match {
			filtered = append(filtered, item)
			fmt.Printf("Included %v\n", item.Title)
		} else {
			fmt.Printf(ANSIGray+"Excluded %v\n"+ANSIReset, item.Title)
		}
	}

	fmt.Printf("\nFiltered %d → %d items\n", len(parsed.Items), len(filtered))
}
