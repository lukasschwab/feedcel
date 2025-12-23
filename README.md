# feedcel 🙄

A proxy for filtering RSS, Atom, and JSON feeds using [CEL](https://github.com/google/cel-go) expressions and a package for including that filtering in other applications (typically reader clients).

## Usage

### CLI

Inspect feeds directly:

```bash
go run ./cmd/cli -feed <url_or_path> -expr "item.Title.contains('Go')"
```

Run without `-expr` for interactive input and live validation.

### Proxy

Start the server:

```bash
go run ./cmd/proxy -port 8080
```

Send a request with a `url` and a CEL `expression`.

**GET:**
```bash
curl "http://localhost:8080/filter?url=https://news.ycombinator.com/rss&expression=item.Title.contains('Go')"
```

**POST:**
```bash
curl -X POST -d 
  "{
    \"url\": \"https://news.ycombinator.com/rss\",
    \"expression\": \"item.Title.contains(\"Go\")\"
  }" http://localhost:8080/filter
```

#### Options

- `format`: Output format. Supports `json` (default), `rss`, or `atom`.
- `expression`: CEL expression. The `item` variable exposes fields like `Title`, `URL`, `Author`, `Tags`, and `Content`.

For more details on available fields and expression syntax, see `pkg/cel/env.go` and the test suite in `cmd/proxy/main_test.go`.

## CEL environment

Syntax extended with https://pkg.go.dev/github.com/google/cel-go/ext#Strings
