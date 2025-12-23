package feed

import (
	"context"
	"strings"

	"github.com/lukasschwab/feedcel/pkg/cel"
	"github.com/mmcdole/gofeed"
)

// FetchAndParse fetches a feed from a URL and parses it into Items.
func FetchAndParse(ctx context.Context, url string) ([]cel.Item, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURLWithContext(url, ctx)
	if err != nil {
		return nil, err
	}

	items := make([]cel.Item, 0, len(feed.Items))
	for _, fItem := range feed.Items {
		item := cel.Item{
			URL: fItem.Link,
		}

		if fItem.Title != "" {
			item.Title = &fItem.Title
		}

		if fItem.Author != nil {
			item.Author = &fItem.Author.Name
		} else if len(fItem.Authors) > 0 {
			item.Author = &fItem.Authors[0].Name
		}

		if len(fItem.Categories) > 0 {
			tags := strings.Join(fItem.Categories, ",")
			item.Tags = &tags
		}

		if fItem.Content != "" {
			item.ContentLength = len(fItem.Content)
		}

		items = append(items, item)
	}

	return items, nil
}
