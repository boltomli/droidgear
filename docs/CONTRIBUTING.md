# Contributing Guidelines

Thank you for your interest in contributing!

## Quick Start

### Prerequisites

- [Node.js](https://nodejs.org/) (v20+)
- [Go](https://go.dev/dl/) (1.27+) and the Wails v3 CLI
  (`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0`)
- Familiarity with React, TypeScript, and Go

### Setup

```bash
git clone https://github.com/YOUR_USERNAME/YOUR_REPO.git
cd YOUR_REPO
npm install
npm run dev
npm run check:all
```

## How to Contribute

### Issues

- **Bug Reports**: Use the bug report template
- **Feature Requests**: Use the feature request template
- **Security Issues**: See [SECURITY.md](SECURITY.md)

### Pull Requests

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/amazing-feature`
3. Make changes following the guidelines below
4. Ensure checks pass: `npm run check:all`
5. Commit using conventional commits
6. Push and open a Pull Request

## Code Guidelines

### TypeScript/React

- Use TypeScript for all new code
- Follow existing component patterns
- See `docs/developer/` for architecture patterns

### Go (Wails)

- Use `gofmt`, and run `go vet ./... && go test ./...` inside `wails/`
- Return plain Go `error`s; the command bridge surfaces them as strings
- See `docs/agents/wails-port.md`

## Quality Gates

All PRs must pass:

- TypeScript type checking
- ESLint and Prettier
- Go formatting, vet and tests (`cd wails && go vet ./... && go test ./...`)
- Tests

Run locally: `npm run check:all`

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```bash
feat: add user authentication
fix(ui): resolve sidebar toggle issue
docs: update installation instructions
refactor(store): simplify state management
test: add preferences tests
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

## Code Review

- Keep PRs focused and reasonably sized
- Write clear PR descriptions
- Respond to feedback promptly
- Update documentation as needed

## Legal

By contributing, you agree that your contributions will be licensed under the same license as the project.
