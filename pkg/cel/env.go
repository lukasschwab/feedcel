package cel

import (
	"fmt"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// Item fields jointly derivable from JSON and Atom feeds. This struct is a
// subset of the item fields stored by [reader].
//
// # This package is a cel environment for filtering
//
// [reader]: https://github.com/lukasschwab/reader/blob/main/pkg/models/item.go
type Item struct {
	URL    string
	Title  *string
	Author *string
	Tags   *string // Comma-separated, feed-defined tags for the item

	Content       *string
	ContentLength int

	Published time.Time
	Updated   time.Time
}

// NewEnv creates a new CEL environment configured for filtering Items.
func NewEnv() (*cel.Env, error) {
	return cel.NewEnv(
		// Load standard library extensions (strings, math, etc.)
		cel.StdLib(),
		cel.OptionalTypes(),
		ext.Strings(),

		// Define the primary 'item' variable.
		//
		// We use cel.AnyType to allow reflection-based access to the struct
		// fields. For stricter typing, we could define the object type, but Any
		// is flexible for a start.
		//
		// TODO: define an object type here!
		cel.Variable("item", cel.AnyType),
		cel.Variable("now", cel.TimestampType),
	)
}

// Compile compiles a CEL expression string.
func Compile(env *cel.Env, expr string) (cel.Program, error) {
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	return env.Program(ast)
}

// Evaluate evaluates a compiled CEL program against an item.
func Evaluate(prg cel.Program, item Item) (bool, error) {
	// Convert Item to map[string]any for CEL compatibility.
	// We do this manually to ensure correct handling of fields.
	itemMap := map[string]any{
		"URL":           item.URL,
		"Title":         item.Title,
		"Author":        item.Author,
		"ContentLength": item.ContentLength,
		// TODO: want this to be a slice rather than a string
		"Tags": item.Tags,
	}

	out, _, err := prg.Eval(map[string]any{
		"item": itemMap,
		"now":  time.Now(),
	})
	if err != nil {
		return false, err
	}

	// Expect a boolean result
	if b, ok := out.Value().(bool); ok {
		return b, nil
	}

	return false, fmt.Errorf("expression did not return a boolean")
}
