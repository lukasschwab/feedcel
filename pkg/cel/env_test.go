package cel_test

import (
	"testing"
	"time"

	"github.com/lukasschwab/feedcel/pkg/cel"
)

var (
	LearningGo   = "Learning Go"
	Hello        = "Hello"
	RustGoPython = "rust, go, python"
)

// TODO: add a separate TestCompile function confirming treatment of invalid cel
// expressions.

func TestEvaluate(t *testing.T) {
	now := time.Now()

	env, err := cel.NewEnv()
	if err != nil {
		t.Fatalf("NewEnv failed: %v", err)
	}

	tests := []struct {
		name      string
		expr      string
		item      cel.Item
		want      bool
		wantError bool
	}{
		{
			name: "Title match",
			expr: `item.Title.contains("Go")`,
			item: cel.Item{
				Title: &LearningGo,
			},
			want: true,
		},
		{
			name: "Title no match",
			expr: `item.Title.contains("Rust")`,
			item: cel.Item{
				Title: &LearningGo,
			},
			want: false,
		},
		{
			name: "URL match",
			expr: `item.URL.endsWith(".com")`,
			item: cel.Item{
				URL: "https://example.com",
			},
			want: true,
		},
		{
			name: "Valid Title check",
			expr: `item.Title == "Hello"`,
			item: cel.Item{
				Title: &Hello,
			},
			want: true,
		},
		{
			name: "Tags check",
			expr: `item.Tags.split(",").exists(t, t.trim() == "go")`,
			item: cel.Item{
				Tags: &RustGoPython,
			},
			want: true,
		},
		// TODO: test content length filter.
		// TODO: test filtering timestamps using the now cel variable.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prg, err := cel.Compile(env, tt.expr)
			if err != nil {
				t.Fatalf("Compile failed: %v", err)
			}

			got, err := cel.Evaluate(prg, tt.item, now)
			if (err != nil) != tt.wantError {
				t.Errorf("Evaluate() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}
