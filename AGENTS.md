# media115 Agent Instructions

**media115** is a media library management tool for 115 cloud drive. For CLI usage see [README.md](README.md). For operator workflows see [skills/](skills/).

## Rules

- Use `make` for all build and test operations; do NOT run `go build`, `go test`, `go vet` or other Go toolchain commands directly
- Run `make lint test` before committing
- Follow [CONTRIBUTING.md](CONTRIBUTING.md) for commit messages, PR workflow, and code quality rules
- Keep changes focused — one concern per commit
- Ensure incremental test coverage reaches 90% before committing; `make test` generates `coverage.html` for visual review

## Conventions

- Use `github.com/bytedance/gg` as the general-purpose Go utility library
- Use `github.com/bytedance/mockey` for mocking in unit tests
- Use constructor + DI pattern for cobra commands: `newXxxCmd()` returns `*cobra.Command`, RunE delegates to `xxxRun(opts)` with injected dependencies
- Test `xxxRun()` directly with mock dependencies, not the RunE handler itself
