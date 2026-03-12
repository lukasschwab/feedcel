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
			name: "Valid filter expression",
			expr: `item.Title.contains("Go")`,
		},
		{
			name: "Valid transform expression",
			expr: `optional.of(item.Title)`,
		},
		{
			name: "Valid transform with functions",
			expr: `optional.of(htmlUnescape(stripTags(item.Title)))`,
		},
		{
			name: "Valid transform that drops",
			expr: `item.Title.contains("Go") ? optional.of(item.Title) : optional.none()`,
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
			name:      "Invalid expression (type mismatch)",
			expr:      `item.Title == 123`,
			wantError: true,
		},
		{
			name:      "Invalid expression (wrong return type: string)",
			expr:      `"just a string"`,
			wantError: true,
		},
		{
			name:      "Invalid expression (wrong return type: int)",
			expr:      `1 + 2`,
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
			assert.Nil(t, result.Title, "filter should not set title")
		})
	}
}

func TestEvaluate_Transform(t *testing.T) {
	now := time.Now()
	env, err := cel.NewEnv()
	require.NoError(t, err)

	tests := []struct {
		name      string
		expr      string
		item      cel.Item
		wantDrop  bool
		wantTitle *string
	}{
		{
			name:      "Transform keeps and sets title",
			expr:      `optional.of(item.Title.upperAscii())`,
			item:      cel.Item{Title: &Hello},
			wantDrop:  false,
			wantTitle: ptr("HELLO"),
		},
		{
			name:     "Transform drops with optional.none",
			expr:     `item.Title.contains("Go") ? optional.of(item.Title) : optional.none()`,
			item:     cel.Item{Title: &Hello},
			wantDrop: true,
		},
		{
			name:      "Transform conditional keep",
			expr:      `item.Title.contains("Go") ? optional.of(item.Title) : optional.none()`,
			item:      cel.Item{Title: &LearningGo},
			wantDrop:  false,
			wantTitle: &LearningGo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prg, err := env.Compile(tt.expr)
			require.NoError(t, err)

			result, err := cel.Evaluate(prg, tt.item, now)
			require.NoError(t, err)
			assert.Equal(t, tt.wantDrop, result.Drop)
			if tt.wantTitle != nil {
				require.NotNil(t, result.Title)
				assert.Equal(t, *tt.wantTitle, *result.Title)
			} else {
				assert.Nil(t, result.Title)
			}
		})
	}
}

func TestCustomFunctions(t *testing.T) {
	now := time.Now()
	env, err := cel.NewEnv()
	require.NoError(t, err)

	t.Run("stripTags", func(t *testing.T) {
		htmlTitle := `&ldquo;Sapiens?&rdquo; <br><small>by Hunter Dukes</small>`
		item := cel.Item{Title: &htmlTitle}

		prg, err := env.Compile(`optional.of(stripTags(item.Title))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Title)
		// stripTags removes tags, keeps text content. The x/net/html
		// tokenizer also decodes HTML entities as a side effect.
		assert.Equal(t, "\u201cSapiens?\u201d by Hunter Dukes", *result.Title)
	})

	t.Run("htmlUnescape", func(t *testing.T) {
		htmlTitle := `&ldquo;Sapiens?&rdquo;`
		item := cel.Item{Title: &htmlTitle}

		prg, err := env.Compile(`optional.of(htmlUnescape(item.Title))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Title)
		assert.Equal(t, "\u201cSapiens?\u201d", *result.Title)
	})

	t.Run("stripTags + htmlUnescape composed", func(t *testing.T) {
		htmlTitle := `&ldquo;Sapiens?&rdquo; <br><small>by Hunter Dukes</small>`
		item := cel.Item{Title: &htmlTitle}

		prg, err := env.Compile(`optional.of(htmlUnescape(stripTags(item.Title)))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Title)
		assert.Equal(t, "\u201cSapiens?\u201d by Hunter Dukes", *result.Title)
	})

	t.Run("Cabinet Magazine: split at br + htmlUnescape", func(t *testing.T) {
		// The real-world approach: split at <br> to get just the title.
		htmlTitle := `&ldquo;Sapiens?&rdquo; <br><small>by Hunter Dukes</small>`
		item := cel.Item{Title: &htmlTitle}

		prg, err := env.Compile(`optional.of(htmlUnescape(item.Title.split("<br>")[0].trim()))`)
		require.NoError(t, err)

		result, err := cel.Evaluate(prg, item, now)
		require.NoError(t, err)
		require.NotNil(t, result.Title)
		assert.Equal(t, "\u201cSapiens?\u201d", *result.Title)
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
}

func ptr(s string) *string { return &s }
