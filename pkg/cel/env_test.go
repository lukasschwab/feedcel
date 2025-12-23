package cel_test

import (
	"testing"
	"time"

	"github.com/lukasschwab/feedcel/pkg/cel"
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
	if err != nil {
		t.Fatalf("NewEnv failed: %v", err)
	}

	tests := []struct {
		name      string
		expr      string
		wantError bool
	}{
		{
			name:      "Valid expression",
			expr:      `item.Title.contains("Go")`,
			wantError: false,
		},
		{
			name:      "Invalid expression (syntax error)",
			expr:      `item.Title.contains("Go"`, // Missing closing parenthesis
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
			name:      "Invalid expression (wrong return type)",
			expr:      `"just a string"`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cel.Compile(env, tt.expr)
			if (err != nil) != tt.wantError {
				t.Errorf("Compile() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	now := time.Now()
	oneHourAgo := now.Add(-1 * time.Hour)

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
		{
			name: "Timestamp check (recent)",
			expr: `now - item.Published < duration("2h")`,
			item: cel.Item{
				Published: oneHourAgo,
			},
			want: true,
		},
		{
			name: "Timestamp check (old)",
			expr: `now - item.Published < duration("30m")`,
			item: cel.Item{
				Published: oneHourAgo,
			},
			want: false,
		},
		{
			name: "Content match",
			expr: `item.Content.contains("ipsum")`,
			item: cel.Item{
				Content: &LoremIpsum,
			},
			want: true,
		},
		{
			name: "Content no match",
			expr: `!item.Content.contains("dolor")`,
			item: cel.Item{
				Content: &LoremIpsum,
			},
			want: false,
		},
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
