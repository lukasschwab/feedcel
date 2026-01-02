package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
	"encoding/json"
	"runtime/debug"

	"github.com/charmbracelet/huh"
	"github.com/lukasschwab/feedcel/pkg/cel"
	gf "github.com/lukasschwab/feedcel/pkg/gofeed"
	"github.com/mmcdole/gofeed"
)

const (
	ANSIGray   = "\033[90m"
	ANSIYellow = "\033[33m"
	ANSIReset  = "\033[0m"
)

func main() {
	version := flag.Bool("version", false, "Display tool version info (JSON)")
	feedRef := flag.String("feed", "", "URL or path to the feed")
	expr := flag.String("expr", "", "CEL expression to filter items")
	flag.Parse()

	if *version {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			fmt.Println("Error reading build info")
			os.Exit(1)
		}
		settings := make(map[string]string, len(info.Settings))
		for _, pair := range info.Settings {
			settings[pair.Key] = pair.Value
		}
		b, err := json.MarshalIndent(map[string]any{
			"go": info.GoVersion,
			"path": info.Path,
			"settings": settings,
		}, "", "\t")
		if err != nil {
			fmt.Printf("Error marshaling build info: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(b))
		return
	}

	if *feedRef == "" {
		fmt.Println("Usage: feedcel -feed <url_or_path> [-expr <cel_expression>]")
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

	// Initialize CEL environment early for validation
	env, err := cel.NewEnv()
	if err != nil {
		log.Fatalf("Failed to create CEL env: %v", err)
	}

	// If expr not provided, ask interactively
	if *expr == "" {
		err := huh.NewInput().
			Title("Enter CEL Filter Expression").
			Placeholder("e.g. item.Title.contains('Go')").
			Value(expr).
			Validate(func(s string) error {
				if s == "" {
					return errors.New("expression cannot be empty")
				}
				_, err := env.Compile(s)
				return err
			}).
			Run()

		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(ANSIYellow + *expr + ANSIReset + "\n")
	}

	prg, err := env.Compile(*expr)
	if err != nil {
		fmt.Printf("Invalid CEL expression: %v\n", err)
		os.Exit(1)
	}

	var filtered []*gofeed.Item
	now := time.Now()
	for _, item := range parsed.Items {
		celItem := gf.Transform(item)
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
