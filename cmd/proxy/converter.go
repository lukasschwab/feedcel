package main

import (
	"time"

	"github.com/gorilla/feeds"
	"github.com/mmcdole/gofeed"
)

// ToGorillaFeed converts a gofeed.Feed (likely filtered) into a gorilla/feeds.Feed
// for serialization.
func ToGorillaFeed(gf *gofeed.Feed) *feeds.Feed {
	feed := &feeds.Feed{
		Title:       gf.Title,
		Description: gf.Description,
		Link:        &feeds.Link{Href: gf.Link},
		Items:       make([]*feeds.Item, 0, len(gf.Items)),
	}

	if gf.UpdatedParsed != nil {
		feed.Updated = *gf.UpdatedParsed
	} else {
		feed.Updated = time.Now()
	}

	if gf.PublishedParsed != nil {
		feed.Created = *gf.PublishedParsed
	}

	if gf.Author != nil {
		feed.Author = &feeds.Author{Name: gf.Author.Name, Email: gf.Author.Email}
	}

	if gf.Image != nil {
		feed.Image = &feeds.Image{Url: gf.Image.URL, Title: gf.Image.Title}
	}

	for _, item := range gf.Items {
		newItem := &feeds.Item{
			Title:       item.Title,
			Link:        &feeds.Link{Href: item.Link},
			Description: item.Description,
			Content:     item.Content,
			Id:          item.GUID,
		}

		if item.Author != nil {
			newItem.Author = &feeds.Author{Name: item.Author.Name, Email: item.Author.Email}
		}

		if item.PublishedParsed != nil {
			newItem.Created = *item.PublishedParsed
		}
		if item.UpdatedParsed != nil {
			newItem.Updated = *item.UpdatedParsed
		}

		feed.Items = append(feed.Items, newItem)
	}

	return feed
}
