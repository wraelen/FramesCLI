# FramesCLI JSON Schemas

This directory contains JSON Schema definitions for all MCP tools and CLI JSON outputs.

## MCP Tool Schemas

Each tool has input and output schemas:

- `mcp-tool-*.input.json` - Input argument schema for MCP tool calls
- `mcp-tool-*.output.json` - Output schema for MCP tool responses

## CLI JSON Schema

- `cli-response-envelope.json` - Common CLI `--json` response structure

## Usage

Agents can reference these schemas to understand FramesCLI's API contract.

**Example (TypeScript):**
```typescript
import doctorInput from './mcp-tool-doctor.input.json';
import doctorOutput from './mcp-tool-doctor.output.json';

// Validate tool call
const args: typeof doctorInput = {};
const result: typeof doctorOutput = await callTool('doctor', args);
```

**Example (Python with jsonschema):**
```python
import json
import jsonschema

with open('mcp-tool-preview.input.json') as f:
    schema = json.load(f)

# Validate input
args = {"input": "recent", "fps": 4}
jsonschema.validate(args, schema)
```

## Tool List

| Tool | Description |
|------|-------------|
| `doctor` | Check local toolchain readiness |
| `preview` | Estimate extraction cost before running |
| `extract` | Extract frames and optional transcript from video |
| `extract_batch` | Process multiple videos |
| `transcribe_run` | Resume/add transcription to existing run |
| `open_last` | Get path to specific artifact from latest run |
| `get_latest_artifacts` | Get all artifact paths from latest run |
| `get_run_artifacts` | Query specific or recent runs |
| `prefs_get` | Get agent path configuration |
| `prefs_set` | Set agent path configuration |
