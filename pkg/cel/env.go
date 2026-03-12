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
// A filter program (bool) keeps or drops items.
// A transform program (optional<string>) can rewrite titles and/or drop items.
type Program struct {
	program cel.Program
	kind    programKind
}

// Result describes how to process an item after program evaluation.
type Result struct {
	Drop  bool    // If true, remove the item from the feed.
	Title *string // If non-nil, update the item's title.
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

// NewEnv creates a new CEL environment configured for processing Items.
func NewEnv() (Env, error) {
	inner, err := cel.NewEnv(
		cel.StdLib(),
		cel.OptionalTypes(),
		ext.Strings(),

		// Register the Item type for strict typing
		ext.NativeTypes(reflect.TypeOf(Item{})),

		// Define the primary 'item' variable.
		cel.Variable("item", cel.ObjectType("cel.Item")),
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
	return Env{inner}, err
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

// isOptionalStringType checks whether a CEL type is optional<string>.
func isOptionalStringType(t *cel.Type) bool {
	want := cel.OptionalType(cel.StringType)
	return t == want || t.String() == want.String()
}

// Compile compiles a CEL expression string. The expression must evaluate to
// either bool (filter: true keeps, false drops) or optional<string> (transform:
// some(s) keeps item and sets title to s, none drops item).
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
	case isOptionalStringType(outType):
		kind = transformProgram
	default:
		return Program{}, fmt.Errorf("expression must evaluate to bool or optional<string>, got %v", outType)
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
		s, ok := opt.GetValue().Value().(string)
		if !ok {
			return Result{}, fmt.Errorf("optional value is not a string")
		}
		return Result{Drop: false, Title: &s}, nil

	default:
		return Result{}, fmt.Errorf("unknown program kind")
	}
}
