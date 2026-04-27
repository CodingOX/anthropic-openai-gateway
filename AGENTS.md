# Repository Guidelines

## Project Structure & Module Organization

This Go gateway converts Anthropic Messages API requests into OpenAI Chat Completions calls, then converts responses back to Anthropic shape.

- `cmd/gateway/main.go`: application entry point.
- `internal/config`: configuration loading from JSON and environment overrides.
- `internal/client`: OpenAI HTTP client.
- `internal/handler`: HTTP handlers such as `/v1/messages` and token counting.
- `internal/transformer`: request, response, and SSE stream conversion logic.
- `pkg/types`: shared Anthropic and OpenAI API DTOs.
- `configs/config.example.json`: local config template. Copy it to `configs/config.json`; do not commit real keys.
- `oc-go-cc/`: separate nested Go project; avoid changing it unless the task explicitly targets it.

## Build, Test, and Development Commands

- `go run cmd/gateway/main.go`: run the gateway locally, defaulting to `127.0.0.1:3456`.
- `go test ./...`: run all unit tests in this module.
- `go test ./internal/transformer -run TestTransformRequest`: run focused transformer tests.
- `go build -o gateway cmd/gateway/main.go`: build the local binary.
- `gofmt -w <files>`: format edited Go files before submitting.

Use `CONFIG_FILE`, `LISTEN_HOST`, `LISTEN_PORT`, `OPENAI_BASE_URL`, `OPENAI_API_KEY`, and `OPENAI_TIMEOUT_MS` to override JSON config at runtime.

## Coding Style & Naming Conventions

Follow standard Go style: tabs via `gofmt`, short package names, exported identifiers in `PascalCase`, and unexported helpers in `camelCase`. Keep transformer code explicit and easy to audit; prefer simple mappings over broad abstractions. Add concise Simplified Chinese comments for core conversion paths, edge cases, or protocol quirks that are not obvious from the code.

## Testing Guidelines

Tests use Go's standard `testing` package and live next to implementation files as `*_test.go`. Name tests by behavior, for example `TestTransformRequestSplitsMultipleToolResultsAndKeepsUserText`. Add or update tests when changing request conversion, response conversion, stream events, or handler output. For background runs, prefer `timeout 60s go test ./...`.

## Commit & Pull Request Guidelines

This checkout does not expose Git history at the repository root, so no existing commit convention could be verified. Use clear, imperative messages such as `fix: preserve tool result ordering` or `test: cover count_tokens handler`. Pull requests should include the behavioral change, affected endpoints or conversion paths, verification commands and results, and any configuration impact. Include sample JSON or SSE snippets when changing API contracts.

## Security & Configuration Tips

Never commit `configs/config.json`, API keys, or local shell exports. Keep examples in `configs/config.example.json` placeholder-only. Treat request and response logs as sensitive because prompts, tool payloads, and model outputs may contain private data.
