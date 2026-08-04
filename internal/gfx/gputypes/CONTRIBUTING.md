# Contributing to gputypes

Thank you for your interest in contributing!

## Requirements

- **Go 1.25+**
- **golangci-lint** for code quality

## Quick Start

```bash
git clone https://github.com/gogpu/gputypes
cd gputypes

go build ./...
golangci-lint run
```

## Adding New Types

1. Determine the correct file based on domain (texture.go, buffer.go, etc.)
2. Follow WebGPU spec naming exactly
3. Add documentation with spec references
4. Ensure values match wgpu-types

## Pull Request Guidelines

- [ ] Code builds (`go build ./...`)
- [ ] Linter passes (`golangci-lint run`)
- [ ] Documentation updated
- [ ] CHANGELOG.md updated

## Commit Messages

```
feat: add QueryType enum
fix: correct BlendFactor values
docs: update texture format table
```

## Questions?

Open a [GitHub Issue](https://github.com/gogpu/gputypes/issues)
