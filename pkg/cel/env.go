package cel

import (
	"fmt"
	"reflect"
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

	Content *string

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
		// Register the Item type for strict typing
		ext.NativeTypes(reflect.TypeOf(Item{})),

		// Define the primary 'item' variable.
		cel.Variable("item", cel.ObjectType("cel.Item")),
		cel.Variable("now", cel.TimestampType),
	)
}

// Compile compiles a CEL expression string.
func Compile(env *cel.Env, expr string) (cel.Program, error) {
	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("expression must evaluate to bool, got %v", ast.OutputType())
	}
	return env.Program(ast)
}

// Evaluate evaluates a compiled CEL program against an item.
func Evaluate(prg cel.Program, item Item, now time.Time) (bool, error) {
	out, _, err := prg.Eval(map[string]any{
		"item": item,
		"now":  now,
	})
	if err != nil {
		return false, err
	}
	if b, ok := out.Value().(bool); ok {
		return b, nil
	}
	return false, fmt.Errorf("expression did not return a boolean")
}
