package cel

import (
	"fmt"
	gohtml "html"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"
	nethtml "golang.org/x/net/html"
)

type Env struct{ *cel.Env }

type programKind int

const (
	filterProgram    programKind = iota
	transformProgram programKind = iota
)

// Program is a compiled CEL expression for processing feed items.
// A filter program (bool) keeps or drops items. Deprecated: prefer transform.
// A transform program (optional<cel.Item>) can modify items and/or drop them.
type Program struct {
	program cel.Program
	kind    programKind
}

// Result describes how to process an item after program evaluation.
type Result struct {
	Drop bool  // If true, remove the item from the feed.
	Item *Item // If non-nil, use this item's fields to update the feed item.
}

// Item fields jointly derivable from JSON and Atom feeds. This struct is a
// subset of the item fields stored by [reader].
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

var itemType = cel.ObjectType("cel.Item")

// NewEnv creates a new CEL environment configured for processing Items.
func NewEnv() (Env, error) {
	base, err := cel.NewEnv(
		cel.StdLib(),
		cel.OptionalTypes(),
		ext.Strings(),

		// Register the Item type for strict typing.
		ext.NativeTypes(reflect.TypeOf(Item{})),

		// Define the primary 'item' variable.
		cel.Variable("item", itemType),
		cel.Variable("now", cel.TimestampType),

		// Custom string functions for HTML processing.
		cel.Function("stripTags",
			cel.Overload("stripTags_string",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(stripTagsBinding),
			),
		),
		cel.Function("htmlUnescape",
			cel.Overload("htmlUnescape_string",
				[]*cel.Type{cel.StringType},
				cel.StringType,
				cel.UnaryBinding(htmlUnescapeBinding),
			),
		),
	)
	if err != nil {
		return Env{}, err
	}

	// Extend with item.with* member functions. These need the type adapter
	// from the base env to convert Go Item values back to CEL values.
	adapter := base.CELTypeAdapter()

	inner, err := base.Extend(
		cel.Function("withTitle",
			cel.MemberOverload("item_withTitle",
				[]*cel.Type{itemType, cel.StringType}, itemType,
				cel.BinaryBinding(withFieldBinding(adapter, func(i *Item, s string) { i.Title = &s })),
			),
		),
		cel.Function("withAuthor",
			cel.MemberOverload("item_withAuthor",
				[]*cel.Type{itemType, cel.StringType}, itemType,
				cel.BinaryBinding(withFieldBinding(adapter, func(i *Item, s string) { i.Author = &s })),
			),
		),
		cel.Function("withURL",
			cel.MemberOverload("item_withURL",
				[]*cel.Type{itemType, cel.StringType}, itemType,
				cel.BinaryBinding(withFieldBinding(adapter, func(i *Item, s string) { i.URL = s })),
			),
		),
		cel.Function("withContent",
			cel.MemberOverload("item_withContent",
				[]*cel.Type{itemType, cel.StringType}, itemType,
				cel.BinaryBinding(withFieldBinding(adapter, func(i *Item, s string) { i.Content = &s })),
			),
		),
		cel.Function("withTags",
			cel.MemberOverload("item_withTags",
				[]*cel.Type{itemType, cel.StringType}, itemType,
				cel.BinaryBinding(withFieldBinding(adapter, func(i *Item, s string) { i.Tags = &s })),
			),
		),
	)
	if err != nil {
		return Env{}, err
	}

	return Env{inner}, nil
}

// withFieldBinding creates a BinaryBinding that copies an Item, applies a
// setter to one field, and returns the modified copy as a CEL value.
func withFieldBinding(adapter types.Adapter, setter func(*Item, string)) func(ref.Val, ref.Val) ref.Val {
	return func(lhs, rhs ref.Val) ref.Val {
		item, ok := lhs.Value().(Item)
		if !ok {
			return types.NewErr("expected Item receiver")
		}
		s, ok := rhs.Value().(string)
		if !ok {
			return types.NewErr("expected string argument")
		}
		setter(&item, s)
		return adapter.NativeToValue(item)
	}
}

func stripTagsBinding(val ref.Val) ref.Val {
	s, ok := val.Value().(string)
	if !ok {
		return types.NewErr("stripTags: expected string argument")
	}
	return types.String(stripTags(s))
}

func htmlUnescapeBinding(val ref.Val) ref.Val {
	s, ok := val.Value().(string)
	if !ok {
		return types.NewErr("htmlUnescape: expected string argument")
	}
	return types.String(gohtml.UnescapeString(s))
}

// stripTags removes all HTML tags from s, keeping only text content.
func stripTags(s string) string {
	tok := nethtml.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	for {
		tt := tok.Next()
		switch tt {
		case nethtml.ErrorToken:
			if tok.Err() == io.EOF {
				return b.String()
			}
			return s // on unexpected error, return original
		case nethtml.TextToken:
			b.Write(tok.Text())
		}
	}
}

// isOptionalItemType checks whether a CEL type is optional<cel.Item>.
func isOptionalItemType(t *cel.Type) bool {
	want := cel.OptionalType(itemType)
	return t == want || t.String() == want.String()
}

// Compile compiles a CEL expression string. The expression must evaluate to
// either:
//   - bool: Deprecated filter shorthand. true keeps the item, false drops it.
//   - optional<cel.Item>: Transform. some(item) keeps (with modifications),
//     none drops. Use item.withTitle(...) etc. to modify fields.
func (e *Env) Compile(expr string) (Program, error) {
	ast, issues := e.Env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return Program{}, issues.Err()
	}

	outType := ast.OutputType()
	var kind programKind
	switch {
	case outType == cel.BoolType:
		kind = filterProgram
	case isOptionalItemType(outType):
		kind = transformProgram
	default:
		return Program{}, fmt.Errorf("expression must evaluate to bool or optional<cel.Item>, got %v", outType)
	}

	prg, err := e.Env.Program(ast)
	if err != nil {
		return Program{}, err
	}
	return Program{program: prg, kind: kind}, nil
}

// Evaluate evaluates a compiled CEL program against an item.
func Evaluate(prg Program, item Item, now time.Time) (Result, error) {
	out, _, err := prg.program.Eval(map[string]any{
		"item": item,
		"now":  now,
	})
	if err != nil {
		return Result{}, err
	}

	switch prg.kind {
	case filterProgram:
		b, ok := out.Value().(bool)
		if !ok {
			return Result{}, fmt.Errorf("expression did not return a boolean")
		}
		return Result{Drop: !b}, nil

	case transformProgram:
		opt, ok := out.(*types.Optional)
		if !ok {
			return Result{}, fmt.Errorf("expression did not return an optional, got %T", out)
		}
		if !opt.HasValue() {
			return Result{Drop: true}, nil
		}
		newItem, err := extractItem(opt.GetValue())
		if err != nil {
			return Result{}, err
		}
		return Result{Drop: false, Item: newItem}, nil

	default:
		return Result{}, fmt.Errorf("unknown program kind")
	}
}

func extractItem(val ref.Val) (*Item, error) {
	switch v := val.Value().(type) {
	case Item:
		return &v, nil
	case *Item:
		return v, nil
	default:
		return nil, fmt.Errorf("optional value is not an Item, got %T", v)
	}
}
