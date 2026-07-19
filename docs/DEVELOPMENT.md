# Development Guide

How to work on the `code-reviewer` codebase.

## Setup

The repo ships a Nix flake that pins Go, golangci-lint, and git:

```bash
nix develop          # drops you into a shell with all tooling
go build ./...       # build everything
go test ./... -race  # run tests with race detector
```

**Without Nix:** install Go ≥ 1.26 and [golangci-lint](https://golangci-lint.run/) manually, then use the same `go build` / `go test` commands.

> [!TIP]
> The `flake.nix` `devShells.default` includes `go`, `golangci-lint`, and `git`. If you use direnv, add `use flake` to `.envrc`.

---

## Adding a Config Field

Every configuration value flows through three layers (flags > env > yaml > defaults). Follow this checklist when adding a new field:

### 1. Add the field to `Config` struct

In [`internal/config/config.go`](internal/config/config.go#L76-L127), add the field to the `Config` struct with a doc comment:

```go
type Config struct {
    // ...existing fields...
    MyNewField string // Description of what it does.
}
```

### 2. Add to `repoConfig` with yaml tag

In [`internal/config/config.go`](internal/config/config.go#L130-L143), add the corresponding `repoConfig` field so `.code-reviewer.yaml` can set it:

```go
type repoConfig struct {
    // ...existing fields...
    MyNewField string `yaml:"my_new_field"`
}
```

### 3. Add `flag.XxxVar()` in `loadFlags()`

In [`loadFlags()`](internal/config/config.go#L351-L464), register the flag with `flag.NewFlagSet`:

```go
myNewField := fs.String("my-new-field", "", "Description for --help")
```

### 4. Apply the flag value in `loadFlags()`

In the "Apply flags" section of `loadFlags()` (around line 383), add:

```go
if *myNewField != "" {
    c.MyNewField = *myNewField
}
```

### 5. Apply in `applyRepoConfig()`

In [`applyRepoConfig()`](internal/config/config.go#L222-L269), merge from the parsed YAML:

```go
if rc.MyNewField != "" {
    c.MyNewField = rc.MyNewField
}
```

### 6. Apply in `loadEnv()`

If the field should be settable via environment variable, add it in [`loadEnv()`](internal/config/config.go#L271-L348):

```go
if v := os.Getenv("REVIEW_MY_NEW_FIELD"); v != "" {
    c.MyNewField = v
}
```

### 7. Add validation in `validate()` if needed

In [`validate()`](internal/config/config.go#L473-L532), add any constraints:

```go
if c.MyNewField != "" && !isValidValue(c.MyNewField) {
    return fmt.Errorf("invalid my-new-field: %q", c.MyNewField)
}
```

### 8. Set the default in `Load()`

In [`Load()`](internal/config/config.go#L157-L191), set the default value in the `Config` literal if it isn't the zero value.

### 9. Update documentation

- Add to [`.code-reviewer.example.yaml`](.code-reviewer.example.yaml)
- Add to the flags table in [`README.md`](README.md)
- Add to the env vars table in `README.md` if env-sourced

### 10. Add tests in `config_test.go`

In [`internal/config/config_test.go`](internal/config/config_test.go), add tests covering:
- Default value (see `TestLoad_Defaults` pattern at line 76)
- Flag override
- Env override
- YAML override
- Validation (if applicable)

---

## Adding a Provider Method

Both `Provider` (Vertex AI / genai SDK) and `HTTPProvider` (OpenAI-compatible) expose the same capabilities. When adding a new model operation, you need to update both.

### Pattern

1. **Define the interface** in [`internal/reviewer/interfaces.go`](internal/reviewer/interfaces.go) if the new method extends the contract. The existing `ModelReviewer` interface (line 11) has `Review()` and `Close()`. If your method is optional, create a separate interface (checked via type assertion at call sites).

2. **Implement on `Provider`** in [`internal/model/provider.go`](internal/model/provider.go). Follow the `Review()` pattern (line 70):
   - Build a `genai.GenerateContentConfig` with system prompt and temperature
   - Use `retry.Do()` around `p.client.Models.GenerateContent()`
   - Extract text with `extractText()`
   - Parse JSON response with a dedicated `parseXxxJSON()` helper

3. **Implement on `HTTPProvider`** in [`internal/model/http_provider.go`](internal/model/http_provider.go). Follow the `Review()` pattern (line 119):
   - Build a `chatRequest` with system/user messages
   - Use `retry.Do()` around the HTTP call
   - Parse the OpenAI `chatResponse` format
   - Parse your domain JSON from the content

4. **Add JSON parsing helper.** Follow the `parseReviewJSON()` pattern in [`provider.go`](internal/model/provider.go#L190-L227): try direct unmarshal, strip markdown fences, then fallback to brace extraction.

5. **Wire into the reviewer.** Add a method on `Reviewer` in [`internal/reviewer/reviewer.go`](internal/reviewer/reviewer.go) that calls the new provider method. Follow the `Run()` flow (line 66):
   - Get diffs → build prompt → call provider → process results

### Example: Adding a Summarize capability

```
interfaces.go  →  type SummarizeProvider interface { Summarize(...) (*SummaryResult, error) }
provider.go    →  func (p *Provider) Summarize(ctx, prompt string) (*SummaryResult, error) { ... }
http_provider  →  func (p *HTTPProvider) Summarize(ctx, prompt string) (*SummaryResult, error) { ... }
reviewer.go    →  func (r *Reviewer) RunSummary(ctx) error { /* type-assert provider */ }
```

---

## Test Patterns

### Table-driven tests

Every test file uses Go's standard table-driven pattern. See [`config_test.go`](internal/config/config_test.go#L11-L46) for a clean example:

```go
func TestParseSeverity_AllLevels(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name    string
        input   string
        want    Severity
        wantErr bool
    }{
        {name: "low", input: "low", want: SeverityLow},
        {name: "invalid", input: "urgent", want: SeverityLow, wantErr: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            got, err := ParseSeverity(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("ParseSeverity(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("ParseSeverity(%q) = %v, want %v", tt.input, got, tt.want)
            }
        })
    }
}
```

### Mock interfaces

Tests define package-local mock structs that implement the interfaces from [`interfaces.go`](internal/reviewer/interfaces.go). No mock frameworks are used.

**`mockModel`** — implements `ModelReviewer` ([`reviewer_test.go:19-32`](internal/reviewer/reviewer_test.go#L19-L32)):

```go
type mockModel struct {
    result         *model.ReviewResult
    err            error
    calls          int
    lastUserPrompt string
}

func (m *mockModel) Review(ctx context.Context, systemPrompt, userPrompt string) (*model.ReviewResult, error) {
    m.calls++
    m.lastUserPrompt = userPrompt
    return m.result, m.err
}
func (m *mockModel) Close() {}
```

**`mockVCS`** — implements `VCSClient` ([`reviewer_test.go:34-100`](internal/reviewer/reviewer_test.go#L34-L100)):
- Tracks call counts per method (e.g. `postNoteCalls`, `getMRChangesCalls`)
- Returns configurable responses/errors
- Captures arguments for assertion (e.g. `compareFromSHA`)

**`mockDiffSource`** — implements `DiffSource` ([`reviewer_test.go:423-434`](internal/reviewer/reviewer_test.go#L423-L434)):

```go
type mockDiffSource struct {
    diffs []diff.FileDiff
    title string
    desc  string
    err   error
    calls int
}

func (m *mockDiffSource) GetDiffs(ctx context.Context) ([]diff.FileDiff, string, string, error) {
    m.calls++
    return m.diffs, m.title, m.desc, m.err
}
```

**`mockReviewer`** (model package) — uses `atomic.Int32` for thread-safe call counting in concurrent consensus tests ([`multi_test.go:11-25`](internal/model/multi_test.go#L11-L25)).

### How tests avoid network calls

- **No real model calls.** Tests inject `mockModel` via constructor (`New()`, `NewWithDiffSource()`), so `Review()` returns canned data.
- **No real GitLab calls.** Tests inject `mockVCS` — no HTTP, no tokens needed.
- **No real git calls.** `NewWithDiffSource()` injects a `mockDiffSource` that returns pre-built `diff.FileDiff` slices, bypassing `exec.Command("git", ...)`.
- **Config tests** use `t.Setenv()` and `os.Args` manipulation to avoid real flag parsing from the test binary's args.

---

## CI

CI runs in GitHub Actions ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) with three jobs:

| Job | What it does |
|-----|-------------|
| **Build** | `go build -v ./...` + `go vet ./...` |
| **Test** | `go test ./... -v -count=1 -race -coverprofile=coverage.out` |
| **Lint** | `golangci/golangci-lint-action@v7` with `latest` version |

### Running lint locally

```bash
# With nix (golangci-lint is in the devShell):
nix develop -c golangci-lint run ./...

# Without nix:
golangci-lint run ./...
```

### Common linter issues

The project uses golangci-lint's default linter set. Common findings to watch for:

- **`staticcheck` / `QF1001`** — redundant type conversion (e.g., `int(x)` where `x` is already `int`)
- **`unused`** — unexported functions/types/variables that are never referenced
- **`errcheck`** — unchecked error returns
- **`govet`** — `printf`-style format string mismatches, struct field alignment
- **`ineffassign`** — assignments to variables that are never read

### Pre-commit hooks

If the repo has pre-commit hooks configured, always let them run — never use `--no-verify` with `git commit`.

---

## Prompt Changes

The system prompt lives in [`internal/model/prompt.go`](internal/model/prompt.go).

### Structure

1. **`basePrompt`** (line 11) — the core system prompt: persona, objective, constraints, severity guidelines, output format. This is adapted from Google's code-review-commons SKILL.md (Apache 2.0).

2. **`focusOverlays`** (line 73) — a `map[string]string` of focus-area-specific instructions. Current overlays: `bugs`, `security`, `performance`, `style`, `docs`.

3. **`BuildPromptWithCustom()`** (line 128) — assembles the final prompt:
   - Loads custom prompt file or uses `basePrompt`
   - Appends selected focus overlays
   - Appends `extraRules` if set

### How focus overlays work

When `--focus` is `all` (default) or empty, all five overlays are appended in deterministic order. When specific focuses are given (e.g., `--focus bugs,security`), only those overlays are appended.

### Adding a new focus overlay

1. Add the overlay text to the `focusOverlays` map in [`prompt.go`](internal/model/prompt.go#L73-L117):

```go
"testing": `
## FOCUS: Test Coverage
Concentrate on test quality:
- Are edge cases covered?
- ...`,
```

2. Add `"testing"` to the `all` mode's ordered list in [`BuildPromptWithCustom()`](internal/model/prompt.go#L148).

3. Update the `--focus` help text in [`loadFlags()`](internal/config/config.go#L359) and `README.md`.

### Testing prompt text

Prompt functions are pure — they take strings and return strings. Test them directly:

```go
func TestBuildPrompt_CustomFocus(t *testing.T) {
    prompt := model.BuildPrompt([]string{"security"}, "")
    if !strings.Contains(prompt, "FOCUS: Security Review") {
        t.Error("expected security focus overlay")
    }
    if strings.Contains(prompt, "FOCUS: Bug Detection") {
        t.Error("should not contain bug overlay when only security requested")
    }
}
```

Custom prompt file loading uses `os.ReadFile` and falls back to `basePrompt` on error, so test both the happy path (temp file with content) and the fallback (non-existent file).

---

## Project Layout

```
cmd/code-reviewer/     Entry point (main.go)
internal/
├── config/            Config loading (flags, env, yaml)
├── context/           Repo-aware context discovery (tree-sitter)
├── diff/              Diff parsing, filtering, chunking
├── model/             AI provider (Vertex AI, HTTP/OpenAI-compat)
├── retry/             Retry with exponential backoff
├── reviewer/          Review pipeline orchestration
└── vcs/               VCS abstraction (GitLab client, types)
```
