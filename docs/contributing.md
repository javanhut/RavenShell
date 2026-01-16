# Contributing to RavenShell

Thank you for your interest in contributing to RavenShell! This document provides guidelines for contributing to the project.

## Getting Started

### Development Environment

**Prerequisites:**
- Go 1.21 or later
- Git

**Setup:**

```bash
# Clone the repository
git clone https://github.com/yourusername/ravenshell.git
cd ravenshell

# Verify the build
go build -o ravenshell

# Run tests
go test ./...
```

### Project Overview

RavenShell is organized into these main packages:

| Package | Purpose |
|---------|---------|
| `token` | Token type definitions |
| `lexer` | Tokenization |
| `ast` | Abstract Syntax Tree nodes |
| `parser` | Pratt parser implementation |
| `evaluator` | AST execution |
| `readline` | Interactive line editing |

See [architecture.md](architecture.md) for detailed technical documentation.

## Development Workflow

### Creating a Branch

```bash
# Create a feature branch
git checkout -b feature/your-feature-name

# Or for bug fixes
git checkout -b fix/bug-description
```

### Making Changes

1. Write your code following the style guidelines below
2. Add tests for new functionality
3. Update documentation if needed
4. Run tests to ensure nothing is broken

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test ./parser/...
```

### Building

```bash
go build -o ravenshell
```

## Code Style

### Go Conventions

- Run `gofmt` on all code before committing
- Follow [Effective Go](https://golang.org/doc/effective_go) guidelines
- Use descriptive variable and function names

```bash
# Format all code
gofmt -w .
```

### Project Conventions

**Error Messages:**
```go
return "", fmt.Errorf("command: description of error")
```

**AST Node Naming:**
- Statements end with `Statement` (e.g., `ForStatement`)
- Expressions end with `Expression` or `Literal` (e.g., `InfixExpression`, `IntegerLiteral`)

**Test Organization:**
- Use table-driven tests
- Name test functions `TestXxx`
- Group related tests in subtests

```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {"case 1", "input1", "expected1"},
        {"case 2", "input2", "expected2"},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test logic
        })
    }
}
```

## Pull Request Process

### Before Submitting

- [ ] All tests pass (`go test ./...`)
- [ ] Code is formatted (`gofmt -w .`)
- [ ] Documentation is updated (if needed)
- [ ] Commit messages are clear and descriptive

### PR Description

Include in your PR description:

1. **What** changes were made
2. **Why** the changes are needed
3. **How** to test the changes

Example:

```markdown
## Summary
Add `history` command to display command history

## Changes
- Added HISTORY token type
- Implemented history command in evaluator
- Added tests for history functionality

## Testing
1. Start RavenShell
2. Run a few commands
3. Type `history` to see command history
```

### Review Process

- PRs require at least one review before merging
- Address review feedback promptly
- Keep PRs focused on a single change when possible

## Types of Contributions

### Bug Reports

When reporting bugs, include:

- RavenShell version (or commit hash)
- Operating system
- Steps to reproduce
- Expected behavior
- Actual behavior
- Any error messages

**Template:**

```markdown
**Environment:**
- OS: [e.g., Ubuntu 22.04]
- Go version: [e.g., 1.21]
- RavenShell version: [e.g., commit abc123]

**Steps to reproduce:**
1. Run `ravenshell`
2. Enter command `...`
3. See error

**Expected behavior:**
[What should happen]

**Actual behavior:**
[What actually happens]

**Error message (if any):**
```
[paste error here]
```
```

### Feature Requests

For feature requests:

1. Check existing issues to avoid duplicates
2. Describe the feature and its use case
3. Provide examples of how it would work

### Code Contributions

**Good first contributions:**
- Bug fixes
- Documentation improvements
- Adding tests
- Small feature additions

**Larger contributions:**
- Discuss in an issue first
- Break into smaller PRs when possible

## Adding New Features

### New Commands

1. Add token in `token/token.go`
2. Add to `TokenMap`
3. Add `CommandType` in `ast/ast.go`
4. Register in parser
5. Implement in evaluator
6. Add to readline command list
7. Add tests
8. Document in `docs/commands.md`

See [architecture.md](architecture.md) for detailed steps.

### New Language Features

1. Open an issue to discuss the design
2. Get feedback before implementing
3. Update `docs/language-reference.md`
4. Add comprehensive tests
5. Add examples to `docs/examples.md`

## Testing Guidelines

### Test Coverage

- All new code should have tests
- Parser tests verify syntax recognition
- Evaluator tests verify execution behavior

### What to Test

| Component | Test Focus |
|-----------|-----------|
| Lexer | Token recognition, edge cases |
| Parser | AST structure, operator precedence |
| Evaluator | Command behavior, error handling |

### Test Style

```go
func TestParseCommand(t *testing.T) {
    input := "ls ~/Documents"

    l := lexer.New(input)
    p := parser.New(l)
    program := p.ParseProgram()

    checkParserErrors(t, p)

    if len(program.Statements) != 1 {
        t.Fatalf("expected 1 statement, got %d", len(program.Statements))
    }

    // Additional assertions...
}
```

## Documentation

### When to Update

- Adding new features
- Changing existing behavior
- Adding new examples

### Documentation Files

| File | Content |
|------|---------|
| `README.md` | Project overview, quick start |
| `docs/user-guide.md` | End-user documentation |
| `docs/language-reference.md` | Syntax reference |
| `docs/commands.md` | Command reference |
| `docs/examples.md` | Examples and tutorials |
| `docs/architecture.md` | Developer documentation |
| `docs/contributing.md` | This file |

## Questions?

- Open an issue for questions
- Tag it with `question` label
- Check existing issues first

## Code of Conduct

- Be respectful and constructive
- Welcome newcomers
- Focus on the code, not the person
- Accept constructive criticism gracefully
