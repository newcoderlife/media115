# Contributing to media115

## Development Setup

- Go 1.25 or later
- Clone and build:
  ```bash
  git clone https://github.com/newcoderlife/media115.git
  cd media115
  make build
  ```

## Code Quality

Run these before submitting a PR:

```bash
make lint       # golangci-lint (includes gofmt, govet, staticcheck, etc.)
make test       # go test with -race + coverage report
```

All CI checks (lint + test) must pass before merging.

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add new scraper provider
fix: correct category filter for scan
ci: update golangci-lint to v2
refactor: extract cache logic
test: add coverage for organizer
docs: update CLI reference
chore: bump Go version
```

All commits must be signed (`git commit -S`).

## Pull Request Workflow

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-change`
3. Make changes and add tests
4. Run `make lint test`
5. Commit with a signed, conventional commit message
6. Push and open a PR against `master`
7. Wait for CI to pass and request a review

## Guidelines

- Keep changes focused — one concern per PR
- Add tests for new functionality
- Do not lower existing test coverage
- Do not call external APIs (115, TMDB, Bangumi, etc.) directly — use the existing provider abstractions
- Do not modify `go.mod` unless adding/removing a dependency is the purpose of the PR
