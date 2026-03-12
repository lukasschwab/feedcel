// package gofeed provides adapters for feed items parsed by
// [github.com/mmcdole/gofeed].
package gofeed

import (
	"slices"
	"strings"
	"time"

	"github.com/lukasschwab/feedcel/pkg/cel"
	"github.com/mmcdole/gofeed"
)

// Transform gofeed item into cel.Item: adapter for gofeed. See usage in proxy.
func Transform(i *gofeed.Item) (result cel.Item) {
	result.URL = i.Link
	if i.Title != "" {
		result.Title = &i.Title
	}
	if i.Author != nil {
		result.Author = &i.Author.Name
	} else if len(i.Authors) > 0 {
		result.Author = &i.Authors[0].Name
	}
	if len(i.Categories) > 0 {
		tags := strings.Join(i.Categories, ",")
		result.Tags = &tags
	}
	result.Content = &i.Content
	return result
}

// Apply applies a compiled CEL program to each item in the feed.
// Filter programs (bool) remove non-matching items.
// Transform programs (optional<string>) may update titles and/or remove items.
// Items that produce evaluation errors are kept unchanged.
func Apply(prg cel.Program, feed *gofeed.Feed, now time.Time) {
	feed.Items = slices.DeleteFunc(feed.Items, func(i *gofeed.Item) bool {
		result, err := cel.Evaluate(prg, Transform(i), now)
		if err != nil {
			return false // keep on error
		}
		if result.Title != nil {
			i.Title = *result.Title
		}
		return result.Drop
	})
}
