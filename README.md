# feedcel 🙄

A proxy for filtering RSS, Atom, and JSON feeds using [CEL expressions](https://cel.dev/) and a package for including that filtering in other applications (typically reader clients).

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

### Fields

Available fields on the `item` variable. See [`pkg/cel/env.go`](pkg/cel/env.go) for the canonical definition.

- `URL` (string)
- `Title` (string)
- `Author` (string)
- `Tags` (string)
- `Content` (string)
- `Published` (timestamp)
- `Updated` (timestamp)

Global variables:
- `now` (timestamp)

### String Extensions

Includes functions from [`cel-go/ext#Strings`](https://pkg.go.dev/github.com/google/cel-go/ext#Strings):

- `charAt`, `indexOf`, `lastIndexOf`
- `lowerAscii`, `upperAscii`
- `replace`, `substring`, `trim`

### Examples

See [`pkg/cel/env_test.go`](pkg/cel/env_test.go) and [`cmd/proxy/main_test.go`](cmd/proxy/main_test.go) for the test suite.

```cel
// Simple match
item.Title.contains("Go")

// Case-insensitive match
item.Title.lowerAscii().contains("rust")

// Composite logic
(item.Title.contains("release") || item.Tags.contains("v1")) &&
now - item.Published < duration("24h")
```
