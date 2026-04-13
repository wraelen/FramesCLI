# Contributing

## Prerequisites

- Go 1.22+
- `ffmpeg` and `ffprobe` on `PATH`
- optional: `whisper` for transcription workflows

## Local Workflow

```bash
make tidy
make fmt
make test
make build
```

## Integration Tests

Integration tests are opt-in and require `ffmpeg`.
The media regression corpus is generated at test time under temporary directories; prefer adding short synthetic fixtures over committing binary assets.

```bash
go test -tags=integration ./internal/media
```

## Release Verification

For release work, verify packaged artifacts in addition to source tests:

```bash
goreleaser release --snapshot --clean
./scripts/release-verify.sh --source dist --dist-dir ./dist
```

After a GitHub release is published, the `release-verify` workflow re-runs the same checks against the uploaded artifacts.

## Pull Request Expectations

- Keep changes focused and small enough to review.
- Add tests for new behavior.
- Update docs and command inventory when flags, artifact selectors, or MCP/CLI output contracts change.
- Ensure `go test ./...` and `go build ./...` pass before opening PR.
