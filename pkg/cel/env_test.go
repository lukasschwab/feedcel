package cel_test

import (
	"testing"
	"time"

	"github.com/lukasschwab/feedcel/pkg/cel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO: inline these after Go 1.26 upgrade (use new keyword).
var (
	LearningGo   = "Learning Go"
	Hello        = "Hello"
	RustGoPython = "rust, go, python"
	LoremIpsum   = "Lorem ipsum dolor sit amet"
)

func TestCompile(t *testing.T) {
	env, err := cel.NewEnv()
	require.NoError(t, err)

	tests := []struct {
		name      string
		expr      string
		wantError bool
	}{
		{
			name: "Valid filter expression (bool, deprecated)",
			expr: `item.Title.contains("Go")`,
		},
		{
			name: "Valid transform: keep item unchanged",
			expr: `optional.of(item)`,
		},
		{
			name: "Valid transform: withTitle",
			expr: `optional.of(item.withTitle("new"))`,
		},
		{
			name: "Valid transform: chained with*",
			expr: `optional.of(item.withTitle("new").withAuthor("someone"))`,
		},
		{
			name: "Valid transform: conditional drop",
			expr: `item.Title.contains("Go") ? optional.of(item) : optional.none()`,
		},
		{
			name: "Valid transform: withTitle using custom functions",
			expr: `optional.of(item.withTitle(htmlUnescape(stripTags(item.Title))))`,
		},
		{
			name:      "Invalid expression (syntax error)",
			expr:      `item.Title.contains("Go"`,
			wantError: true,
		},
		{
			name:      "Invalid expression (unknown field)",
			expr:      `item.NonExistentField == "Foo"`,
			wantError: true,
		},
		{
			name:      "Invalid expression (wrong return type: string)",
			expr:      `"just a string"`,
			wantError: true,
		},
		{
			name:      "Invalid expression (wrong return type: optional<string>)",
			expr:      `optional.of("a string")`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := env.Compile(tt.expr)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEvaluate_Filter(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)

	env, err := cel.NewEnv()
	require.NoError(t, err)

	tests := []struct {
		name     string
		expr     string
		item     cel.Item
		wantDrop bool
	}{
		{
			name:     "Title match",
			expr:     `item.Title.contains("Go")`,
			item:     cel.Item{Title: &LearningGo},
			wantDrop: false,
		},
		{
			name:     "Title no match",
			expr:     `item.Title.contains("Rust")`,
			item:     cel.Item{Title: &LearningGo},
			wantDrop: true,
		},
		{
			name:     "URL match",
			expr:     `item.URL.endsWith(".com")`,
			item:     cel.Item{URL: "https://example.com"},
			wantDrop: false,
		},
		{
			name:     "Title equality",
			expr:     `item.Title == "Hello"`,
			item:     cel.Item{Title: &Hello},
			wantDrop: false,
		},
		{
			name:     "Tags check",
			expr:     `item.Tags.split(",").exists(t, t.trim() == "go")`,
			item:     cel.Item{Tags: &RustGoPython},
			wantDrop: false,
		},
		{
			name:     "Timestamp check (recent)",
			expr:     `now - item.Published < duration("2h")`,
			item:     cel.Item{Published: oneHourAgo},
			wantDrop: false,
		},
		{
			name:     "Timestamp check (old)",
			expr:     `now - item.Published < duration("30m")`,
			item:     cel.Item{Published: oneHourAgo},
			wantDrop: true,
		},
		{
			name:     "Content match",
			expr:     `item.Content.contains("ipsum")`,
			item:     cel.Item{Content: &LoremIpsum},
			wantDrop: false,
		},
		{
			name:     "Content no match",
			expr:     `!item.Content.contains("dolor")`,
			item:     cel.Item{Content: &LoremIpsum},
			wantDrop: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prg, err := env.Compile(tt.expr)
			require.NoError(t, err)

			result, err := cel.Evaluate(prg, tt.item, now)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDrop, result.Drop)
			assert.Nil(t, result.Item, "filter should not set Item")
		})
	}
}

func TestEvaluate_Transform(t *testing.T) {
	now := time.Now()
	env, err := cel.NewEnv()
	require.NoError(t, err)

	t.Run("withTitle modifies title", func(t *testing.T) {
		prg, err := env.Compile(`optional.of(item.withTitle(item.Title.upperAscii()))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, cel.Item{Title: &Hello}, now)
		require.NoError(t, err)
		assert.False(t, result.Drop)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, "HELLO", *result.Item.Title)
	})

	t.Run("withAuthor modifies author", func(t *testing.T) {
		prg, err := env.Compile(`optional.of(item.withAuthor("New Author"))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, cel.Item{Title: &Hello}, now)
		require.NoError(t, err)
		assert.False(t, result.Drop)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Author)
		assert.Equal(t, "New Author", *result.Item.Author)
		// Title preserved from original.
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, Hello, *result.Item.Title)
	})

	t.Run("chained with* modifies multiple fields", func(t *testing.T) {
		prg, err := env.Compile(`optional.of(item.withTitle("New").withAuthor("Someone"))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, cel.Item{Title: &Hello}, now)
		require.NoError(t, err)
		assert.False(t, result.Drop)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, "New", *result.Item.Title)
		require.NotNil(t, result.Item.Author)
		assert.Equal(t, "Someone", *result.Item.Author)
	})

	t.Run("optional.none drops item", func(t *testing.T) {
		prg, err := env.Compile(`item.Title.contains("Go") ? optional.of(item) : optional.none()`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, cel.Item{Title: &Hello}, now)
		require.NoError(t, err)
		assert.True(t, result.Drop)
	})

	t.Run("conditional keep returns item", func(t *testing.T) {
		prg, err := env.Compile(`item.Title.contains("Go") ? optional.of(item) : optional.none()`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, cel.Item{Title: &LearningGo}, now)
		require.NoError(t, err)
		assert.False(t, result.Drop)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, LearningGo, *result.Item.Title)
	})

	t.Run("optional.of(item) keeps item unchanged", func(t *testing.T) {
		prg, err := env.Compile(`optional.of(item)`)
		require.NoError(t, err)

		original := cel.Item{
			URL:   "https://example.com",
			Title: &Hello,
			Tags:  &RustGoPython,
		}
		result, err := cel.Evaluate(prg, original, now)
		require.NoError(t, err)
		assert.False(t, result.Drop)
		require.NotNil(t, result.Item)
		assert.Equal(t, original.URL, result.Item.URL)
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, Hello, *result.Item.Title)
	})
}

func TestCustomFunctions(t *testing.T) {
	now := time.Now()
	env, err := cel.NewEnv()
	require.NoError(t, err)

	t.Run("stripTags in transform", func(t *testing.T) {
		htmlTitle := `&ldquo;Sapiens?&rdquo; <br><small>by Hunter Dukes</small>`
		item := cel.Item{Title: &htmlTitle}

		prg, err := env.Compile(`optional.of(item.withTitle(stripTags(item.Title)))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Title)
		// stripTags removes tags, keeps text. x/net/html tokenizer also
		// decodes HTML entities as a side effect.
		assert.Equal(t, "\u201cSapiens?\u201d by Hunter Dukes", *result.Item.Title)
	})

	t.Run("htmlUnescape in transform", func(t *testing.T) {
		htmlTitle := `&ldquo;Sapiens?&rdquo;`
		item := cel.Item{Title: &htmlTitle}

		prg, err := env.Compile(`optional.of(item.withTitle(htmlUnescape(item.Title)))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, "\u201cSapiens?\u201d", *result.Item.Title)
	})

	t.Run("Cabinet Magazine: split + htmlUnescape + withTitle", func(t *testing.T) {
		htmlTitle := `&ldquo;Sapiens?&rdquo; <br><small>by Hunter Dukes</small>`
		item := cel.Item{Title: &htmlTitle}

		prg, err := env.Compile(`optional.of(item.withTitle(htmlUnescape(item.Title.split("<br>")[0].trim())))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, "\u201cSapiens?\u201d", *result.Item.Title)
	})

	t.Run("stripTags in filter expression", func(t *testing.T) {
		htmlTitle := `<b>Learning Go</b>`
		item := cel.Item{Title: &htmlTitle}

		prg, err := env.Compile(`stripTags(item.Title).contains("Learning")`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		assert.False(t, result.Drop)
	})

	t.Run("stripTags with plain text (no tags)", func(t *testing.T) {
		plainTitle := `Just plain text`
		item := cel.Item{Title: &plainTitle}

		prg, err := env.Compile(`optional.of(item.withTitle(stripTags(item.Title)))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, "Just plain text", *result.Item.Title)
	})

	t.Run("stripTags with empty string", func(t *testing.T) {
		empty := ``
		item := cel.Item{Title: &empty}

		prg, err := env.Compile(`optional.of(item.withTitle(stripTags(item.Title)))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, "", *result.Item.Title)
	})

	t.Run("htmlUnescape with no entities", func(t *testing.T) {
		plain := `No entities here`
		item := cel.Item{Title: &plain}

		prg, err := env.Compile(`optional.of(item.withTitle(htmlUnescape(item.Title)))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, "No entities here", *result.Item.Title)
	})

	t.Run("htmlUnescape with numeric entities", func(t *testing.T) {
		numeric := `&#60;div&#62;`
		item := cel.Item{Title: &numeric}

		prg, err := env.Compile(`optional.of(item.withTitle(htmlUnescape(item.Title)))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Item)
		require.NotNil(t, result.Item.Title)
		assert.Equal(t, "<div>", *result.Item.Title)
	})
}

func TestEvaluate_WithURL(t *testing.T) {
	now := time.Now()
	env, err := cel.NewEnv()
	require.NoError(t, err)

	prg, err := env.Compile(`optional.of(item.withURL("https://new.example.com"))`)
	require.NoError(t, err)

	result, err := cel.Evaluate(prg, cel.Item{URL: "https://old.example.com", Title: &Hello}, now)
	require.NoError(t, err)
	assert.False(t, result.Drop)
	require.NotNil(t, result.Item)
	assert.Equal(t, "https://new.example.com", result.Item.URL)
}

func TestEvaluate_WithContent(t *testing.T) {
	now := time.Now()
	env, err := cel.NewEnv()
	require.NoError(t, err)

	prg, err := env.Compile(`optional.of(item.withContent("new content"))`)
	require.NoError(t, err)

	result, err := cel.Evaluate(prg, cel.Item{Title: &Hello, Content: &LoremIpsum}, now)
	require.NoError(t, err)
	assert.False(t, result.Drop)
	require.NotNil(t, result.Item)
	require.NotNil(t, result.Item.Content)
	assert.Equal(t, "new content", *result.Item.Content)
}

func TestEvaluate_WithTags(t *testing.T) {
	now := time.Now()
	env, err := cel.NewEnv()
	require.NoError(t, err)

	prg, err := env.Compile(`optional.of(item.withTags("new,tags"))`)
	require.NoError(t, err)

	result, err := cel.Evaluate(prg, cel.Item{Title: &Hello, Tags: &RustGoPython}, now)
	require.NoError(t, err)
	assert.False(t, result.Drop)
	require.NotNil(t, result.Item)
	require.NotNil(t, result.Item.Tags)
	assert.Equal(t, "new,tags", *result.Item.Tags)
}

func TestEvaluate_NowVariable(t *testing.T) {
	env, err := cel.NewEnv()
	require.NoError(t, err)

	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	oneHourAgo := now.Add(-1 * time.Hour)
	twoDaysAgo := now.Add(-48 * time.Hour)

	prg, err := env.Compile(`now - item.Published < duration("24h")`)
	require.NoError(t, err)

	t.Run("recent item passes", func(t *testing.T) {
		result, err := cel.Evaluate(prg, cel.Item{Title: &Hello, Published: oneHourAgo}, now)
		require.NoError(t, err)
		assert.False(t, result.Drop)
	})

	t.Run("old item fails", func(t *testing.T) {
		result, err := cel.Evaluate(prg, cel.Item{Title: &Hello, Published: twoDaysAgo}, now)
		require.NoError(t, err)
		assert.True(t, result.Drop)
	})
}
