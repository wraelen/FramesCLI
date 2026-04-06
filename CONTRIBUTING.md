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

```bash
go test -tags=integration ./internal/media
```

## Pull Request Expectations

- Keep changes focused and small enough to review.
- Add tests for new behavior.
- Update docs and command inventory when flags/UX change.
- Ensure `go test ./...` and `go build ./...` pass before opening PR.

