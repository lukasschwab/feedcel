package gofeed_test

import (
	"testing"
	"time"

	"github.com/lukasschwab/feedcel/pkg/cel"
	adapter "github.com/lukasschwab/feedcel/pkg/gofeed"
	"github.com/mmcdole/gofeed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpers for string pointers.
func strPtr(s string) *string { return &s }

func TestTransform(t *testing.T) {
	tests := []struct {
		name     string
		input    *gofeed.Item
		expected cel.Item
	}{
		{
			name: "all fields populated with Author",
			input: &gofeed.Item{
				Title:      "My Title",
				Link:       "https://example.com/1",
				Author:     &gofeed.Person{Name: "Alice"},
				Categories: []string{"go", "testing"},
				Content:    "Some content",
			},
			expected: cel.Item{
				URL:     "https://example.com/1",
				Title:   strPtr("My Title"),
				Author:  strPtr("Alice"),
				Tags:    strPtr("go,testing"),
				Content: strPtr("Some content"),
			},
		},
		{
			name: "empty title yields nil Title",
			input: &gofeed.Item{
				Title:   "",
				Link:    "https://example.com/2",
				Content: "body",
			},
			expected: cel.Item{
				URL:     "https://example.com/2",
				Title:   nil,
				Author:  nil,
				Tags:    nil,
				Content: strPtr("body"),
			},
		},
		{
			name: "Authors fallback when Author is nil",
			input: &gofeed.Item{
				Title:   "Fallback",
				Link:    "https://example.com/3",
				Authors: []*gofeed.Person{{Name: "Bob"}, {Name: "Charlie"}},
				Content: "",
			},
			expected: cel.Item{
				URL:     "https://example.com/3",
				Title:   strPtr("Fallback"),
				Author:  strPtr("Bob"),
				Content: strPtr(""),
			},
		},
		{
			name: "no author at all",
			input: &gofeed.Item{
				Title:   "No Author",
				Link:    "https://example.com/4",
				Content: "text",
			},
			expected: cel.Item{
				URL:     "https://example.com/4",
				Title:   strPtr("No Author"),
				Author:  nil,
				Content: strPtr("text"),
			},
		},
		{
			name: "single category",
			input: &gofeed.Item{
				Title:      "Tagged",
				Link:       "https://example.com/5",
				Categories: []string{"solo"},
				Content:    "",
			},
			expected: cel.Item{
				URL:     "https://example.com/5",
				Title:   strPtr("Tagged"),
				Tags:    strPtr("solo"),
				Content: strPtr(""),
			},
		},
		{
			name: "no categories",
			input: &gofeed.Item{
				Title:   "Untagged",
				Link:    "https://example.com/6",
				Content: "c",
			},
			expected: cel.Item{
				URL:     "https://example.com/6",
				Title:   strPtr("Untagged"),
				Tags:    nil,
				Content: strPtr("c"),
			},
		},
		{
			name: "Author takes precedence over Authors",
			input: &gofeed.Item{
				Title:   "Precedence",
				Link:    "https://example.com/7",
				Author:  &gofeed.Person{Name: "Primary"},
				Authors: []*gofeed.Person{{Name: "Secondary"}},
				Content: "",
			},
			expected: cel.Item{
				URL:     "https://example.com/7",
				Title:   strPtr("Precedence"),
				Author:  strPtr("Primary"),
				Content: strPtr(""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := adapter.Transform(tt.input)

			assert.Equal(t, tt.expected.URL, result.URL)

			if tt.expected.Title == nil {
				assert.Nil(t, result.Title)
			} else {
				require.NotNil(t, result.Title)
				assert.Equal(t, *tt.expected.Title, *result.Title)
			}

			if tt.expected.Author == nil {
				assert.Nil(t, result.Author)
			} else {
				require.NotNil(t, result.Author)
				assert.Equal(t, *tt.expected.Author, *result.Author)
			}

			if tt.expected.Tags == nil {
				assert.Nil(t, result.Tags)
			} else {
				require.NotNil(t, result.Tags)
				assert.Equal(t, *tt.expected.Tags, *result.Tags)
			}

			if tt.expected.Content == nil {
				assert.Nil(t, result.Content)
			} else {
				require.NotNil(t, result.Content)
				assert.Equal(t, *tt.expected.Content, *result.Content)
			}
		})
	}
}

func TestApply_FilterProgram(t *testing.T) {
	env, err := cel.NewEnv()
	require.NoError(t, err)

	t.Run("filter keeps matching items", func(t *testing.T) {
		prg, err := env.Compile(`item.Title.contains("Go")`)
		require.NoError(t, err)

		feed := &gofeed.Feed{
			Items: []*gofeed.Item{
				{Title: "Learning Go", Link: "https://example.com/go", Content: ""},
				{Title: "Rust Basics", Link: "https://example.com/rust", Content: ""},
				{Title: "Go Patterns", Link: "https://example.com/go2", Content: ""},
			},
		}

		adapter.Apply(prg, feed, time.Now())
		require.Len(t, feed.Items, 2)
		assert.Equal(t, "Learning Go", feed.Items[0].Title)
		assert.Equal(t, "Go Patterns", feed.Items[1].Title)
	})

	t.Run("filter drops all items", func(t *testing.T) {
		prg, err := env.Compile(`item.Title.contains("Java")`)
		require.NoError(t, err)

		feed := &gofeed.Feed{
			Items: []*gofeed.Item{
				{Title: "Go", Link: "https://example.com/1", Content: ""},
				{Title: "Rust", Link: "https://example.com/2", Content: ""},
			},
		}

		adapter.Apply(prg, feed, time.Now())
		assert.Len(t, feed.Items, 0)
	})

	t.Run("filter keeps all items with true", func(t *testing.T) {
		prg, err := env.Compile(`true`)
		require.NoError(t, err)

		feed := &gofeed.Feed{
			Items: []*gofeed.Item{
				{Title: "A", Link: "https://example.com/a", Content: ""},
				{Title: "B", Link: "https://example.com/b", Content: ""},
			},
		}

		adapter.Apply(prg, feed, time.Now())
		assert.Len(t, feed.Items, 2)
	})
}

func TestApply_TransformProgram(t *testing.T) {
	env, err := cel.NewEnv()
	require.NoError(t, err)

	t.Run("withTitle rewrites title", func(t *testing.T) {
		prg, err := env.Compile(`optional.of(item.withTitle("REWRITTEN"))`)
		require.NoError(t, err)

		feed := &gofeed.Feed{
			Items: []*gofeed.Item{
				{Title: "Original", Link: "https://example.com/1", Content: "body"},
			},
		}

		adapter.Apply(prg, feed, time.Now())
		require.Len(t, feed.Items, 1)
		assert.Equal(t, "REWRITTEN", feed.Items[0].Title)
		// Link should be preserved via writeBack (URL is always written back).
		assert.Equal(t, "https://example.com/1", feed.Items[0].Link)
	})

	t.Run("withURL rewrites link", func(t *testing.T) {
		prg, err := env.Compile(`optional.of(item.withURL("https://new.example.com"))`)
		require.NoError(t, err)

		feed := &gofeed.Feed{
			Items: []*gofeed.Item{
				{Title: "Item", Link: "https://old.example.com", Content: "c"},
			},
		}

		adapter.Apply(prg, feed, time.Now())
		require.Len(t, feed.Items, 1)
		assert.Equal(t, "https://new.example.com", feed.Items[0].Link)
	})

	t.Run("withAuthor sets author on item with no author", func(t *testing.T) {
		prg, err := env.Compile(`optional.of(item.withAuthor("New Author"))`)
		require.NoError(t, err)

		feed := &gofeed.Feed{
			Items: []*gofeed.Item{
				{Title: "No Author", Link: "https://example.com/1", Content: ""},
			},
		}

		// dst.Author is nil, so writeBack must create a new Person.
		adapter.Apply(prg, feed, time.Now())
		require.Len(t, feed.Items, 1)
		require.NotNil(t, feed.Items[0].Author)
		assert.Equal(t, "New Author", feed.Items[0].Author.Name)
	})

	t.Run("withAuthor overwrites existing author", func(t *testing.T) {
		prg, err := env.Compile(`optional.of(item.withAuthor("Updated"))`)
		require.NoError(t, err)

		feed := &gofeed.Feed{
			Items: []*gofeed.Item{
				{Title: "Has Author", Link: "https://example.com/1", Content: "", Author: &gofeed.Person{Name: "Old"}},
			},
		}

		adapter.Apply(prg, feed, time.Now())
		require.Len(t, feed.Items, 1)
		require.NotNil(t, feed.Items[0].Author)
		assert.Equal(t, "Updated", feed.Items[0].Author.Name)
	})

	t.Run("withContent rewrites content", func(t *testing.T) {
		prg, err := env.Compile(`optional.of(item.withContent("new content"))`)
		require.NoError(t, err)

		feed := &gofeed.Feed{
			Items: []*gofeed.Item{
				{Title: "Item", Link: "https://example.com/1", Content: "old content"},
			},
		}

		adapter.Apply(prg, feed, time.Now())
		require.Len(t, feed.Items, 1)
		assert.Equal(t, "new content", feed.Items[0].Content)
	})

	t.Run("optional.none drops item", func(t *testing.T) {
		prg, err := env.Compile(`item.Title.contains("keep") ? optional.of(item) : optional.none()`)
		require.NoError(t, err)

		feed := &gofeed.Feed{
			Items: []*gofeed.Item{
				{Title: "keep this", Link: "https://example.com/1", Content: ""},
				{Title: "drop this", Link: "https://example.com/2", Content: ""},
				{Title: "also keep", Link: "https://example.com/3", Content: ""},
			},
		}

		adapter.Apply(prg, feed, time.Now())
		require.Len(t, feed.Items, 2)
		assert.Equal(t, "keep this", feed.Items[0].Title)
		assert.Equal(t, "also keep", feed.Items[1].Title)
	})

	t.Run("transform with stripTags and htmlUnescape", func(t *testing.T) {
		prg, err := env.Compile(`optional.of(item.withTitle(htmlUnescape(stripTags(item.Title))))`)
		require.NoError(t, err)

		feed := &gofeed.Feed{
			Items: []*gofeed.Item{
				{Title: `&ldquo;Hello&rdquo; <b>World</b>`, Link: "https://example.com/1", Content: ""},
			},
		}

		adapter.Apply(prg, feed, time.Now())
		require.Len(t, feed.Items, 1)
		// stripTags decodes entities as a side effect, then htmlUnescape handles the rest.
		assert.Equal(t, "\u201cHello\u201d World", feed.Items[0].Title)
	})
}

// NOTE: The "keep on error" branch in Apply is difficult to trigger through
// normal CEL evaluation because CEL's type system prevents most runtime errors.
// The path exists as a safety net for unexpected eval failures.

func TestApply_EmptyFeed(t *testing.T) {
	env, err := cel.NewEnv()
	require.NoError(t, err)

	prg, err := env.Compile(`true`)
	require.NoError(t, err)

	feed := &gofeed.Feed{Items: []*gofeed.Item{}}
	adapter.Apply(prg, feed, time.Now())
	assert.Len(t, feed.Items, 0)
}
