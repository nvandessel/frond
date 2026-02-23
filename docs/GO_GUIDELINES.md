# Go Development Guidelines

## Code Style

### Formatting
- Always use `gofmt` or `goimports`
- Run `gofmt -l .` before committing

### Naming
- `CamelCase` for exported, `camelCase` for unexported
- Short but descriptive (`ctx` for context, `b` for branch in loops)
- Package names: short, lowercase, singular

### Error Handling
- Return errors as the last return value
- Wrap with context: `fmt.Errorf("doing X: %w", err)`
- Never ignore errors, never panic except for unrecoverable init errors

```go
result, err := doSomething()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}
```

## Testing

- All packages must have `*_test.go` files
- Table-driven tests for multiple input cases
- Test both success and error paths

```go
func TestFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   Type
        want    Type
        wantErr bool
    }{
        {"valid case", input, expected, false},
        {"error case", badInput, nil, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Function(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Interfaces

- Define where they're used, not where implemented
- Keep small (1-3 methods)
- Use for testability

## Context

- Pass `context.Context` as first parameter
- Use for cancellation/timeouts, not data passing
- Never store in structs

## JSON Struct Tags

- All serializable structs must have explicit tags
- Use `omitempty` for optional fields

## Documentation

- Comments on all exported functions and types
- Complete sentences starting with the identifier name

## CLI Patterns (Cobra)

- All commands use `RunE` for error propagation
- All commands support `--json` for agent consumption
- Non-interactive always: no TTY, no prompts, no pagers
