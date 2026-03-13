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
// Transform programs (optional<cel.Item>) may update fields and/or remove items.
// Items that produce evaluation errors are kept unchanged.
func Apply(prg cel.Program, feed *gofeed.Feed, now time.Time) {
	feed.Items = slices.DeleteFunc(feed.Items, func(i *gofeed.Item) bool {
		result, err := cel.Evaluate(prg, Transform(i), now)
		if err != nil {
			return false // keep on error
		}
		if result.Item != nil {
			writeBack(result.Item, i)
		}
		return result.Drop
	})
}

// writeBack applies fields from a cel.Item back onto a gofeed.Item.
func writeBack(src *cel.Item, dst *gofeed.Item) {
	dst.Link = src.URL
	if src.Title != nil {
		dst.Title = *src.Title
	}
	if src.Author != nil {
		if dst.Author == nil {
			dst.Author = &gofeed.Person{}
		}
		dst.Author.Name = *src.Author
	}
	if src.Content != nil {
		dst.Content = *src.Content
	}
}
