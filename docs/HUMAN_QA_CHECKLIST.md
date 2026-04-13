# Human QA Checklist

Use this during the final stabilization pass. For each item, mark it `pass`, `fail`, or `polish`.

## 1. Test Matrix

Run:

```bash
go test ./cmd/frames
go test ./internal/media
go test ./internal/config
go test -tags=integration ./internal/media
```

Check:

- all tests pass on the current branch
- no newly flaky tests appear
- total runtime feels reasonable
- any failures are actionable and specific

## 2. Doctor

Run:

```bash
framescli doctor
framescli doctor --json
```

Check:

- dependency status is obvious in under 5 seconds
- recovery guidance is concrete
- JSON shape feels stable and readable
- transcript and backend hints are understandable
- output does not feel noisy

## 3. Preview

Run:

```bash
framescli preview <short-video>
framescli preview <long-video> --preset laptop-safe
framescli preview <long-video> --preset high-fidelity --json
```

Check:

- workload estimate is immediately understandable
- warnings feel justified, not dramatic
- expensive-workload guardrails are visible before extraction
- preset effect is obvious
- JSON contains the fields an agent would actually need
- output does not imply that audio definitely exists

## 4. Extract

Run:

```bash
framescli extract <short-video> --voice
framescli extract <long-video> --preset laptop-safe --voice
framescli extract <long-video> --preset high-fidelity --voice
```

Check:

- progress feels calm and trustworthy
- output path is easy to find
- warnings and errors are specific
- no-audio failure is early and clear
- guardrail blocking behavior is understandable
- `--allow-expensive` feels like an informed override, not a hack

## 5. Transcribe Resume

Run:

```bash
framescli transcribe-run <runDir> --chunk-duration 600
framescli transcribe-run <same-runDir>
```

Check:

- manifest and resume behavior is obvious
- completed chunks are not redone
- final transcript paths are easy to understand
- resumed run messaging is clear
- chunked behavior does not feel overcomplicated for normal users

## 6. Artifact Retrieval

Run:

```bash
framescli artifacts latest
framescli artifacts --recent 5
framescli artifacts <run-name> --json
framescli open-last --artifact transcript
framescli copy-last --artifact sheet
```

Check:

- latest vs specific-run behavior is intuitive
- artifact labels match user expectations
- output is compact and useful
- JSON and MCP-oriented retrieval feel stable
- index staleness limitation is acceptable and documented

## 7. MCP Smoke

Run:

```bash
framescli mcp
```

Check:

- startup is clean
- tool surface matches docs
- nothing obviously odd appears in handshake or tool naming
- retrieval and transcribe-related tools are present and coherent

## 8. Setup Experience

Review the setup flow in the TUI.

Check:

- first screen explains the product quickly
- performance-mode choices are understandable without extra research
- `laptop-safe`, `balanced`, and `high-fidelity` feel good in UI language
- dependency guidance is clear
- output and input directory choices are easy to trust
- copy is concise
- spacing and hierarchy feel intentional
- no screen feels crowded or amateurish

## 9. Docs

Read:

- `README.md`
- `docs/AGENT_RECIPES.md`
- `CONTRIBUTING.md`

Check:

- command examples match actual behavior
- repeated explanations are removed or minimized
- retrieval story is clear
- long-input story is clear
- setup and install story are clear
- docs feel curated, not layered-on

## 10. Aesthetic

Review the overall feel of:

- setup TUI
- README hero area
- terminology across commands and docs

Check:

- naming is consistent
- tone feels deliberate
- visual presentation feels sharper than a generic dev tool
- the project has one identity, not several

## Decision Rule

After the pass, sort findings into:

- `fix now`
- `fix before next release`
- `document as current limitation`
