package feed

import (
	"strings"

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
