package contracts

func CLIResponseEnvelopeSchema() map[string]any {
	return map[string]any{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"$id":         "https://github.com/wraelen/framescli/schemas/cli-response-envelope.json",
		"title":       "FramesCLI JSON Response Envelope",
		"description": "Common automation envelope used by most --json CLI commands and MCP tool payloads",
		"type":        "object",
		"required":    []string{"schema_version", "command", "status", "started_at", "ended_at", "duration_ms"},
		"properties": map[string]any{
			"schema_version": map[string]any{
				"type":        "string",
				"const":       AutomationSchemaVersion,
				"description": "Schema version identifier",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "CLI command or MCP tool name that generated this response",
				"$comment":    "Intentionally unconstrained: some emitters use CLI-style command names while MCP payloads may expose tool-oriented names. Keep this field as a free-form string and rely on generated tool inventory docs for the current surface.",
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        []string{"success", "partial", "error"},
				"description": "Overall command status",
			},
			"started_at": map[string]any{
				"type":        "string",
				"format":      "date-time",
				"description": "Command start timestamp (RFC3339)",
			},
			"ended_at": map[string]any{
				"type":        "string",
				"format":      "date-time",
				"description": "Command completion timestamp (RFC3339)",
			},
			"duration_ms": map[string]any{
				"type":        "integer",
				"minimum":     0,
				"description": "Elapsed command time in milliseconds",
			},
			"data": map[string]any{
				"type":        "object",
				"description": "Command-specific result payload (structure varies by command)",
			},
			"error": map[string]any{
				"type":        "object",
				"description": "Present when status is 'partial' or 'error'",
				"required":    []string{"code", "message"},
				"properties": map[string]any{
					"code": map[string]any{
						"type":        "string",
						"description": "Stable error code (always 'command_failed')",
					},
					"message": map[string]any{
						"type":        "string",
						"description": "Human-readable error description",
					},
					"class": map[string]any{
						"type":        "string",
						"description": "Additive error category for agent handling",
					},
					"recovery": map[string]any{
						"type":        "string",
						"description": "Suggested recovery action for user/agent",
					},
					"retryable": map[string]any{
						"type":        "boolean",
						"description": "Whether retrying the operation might succeed",
					},
				},
			},
		},
		"$comment": "doctor --json is a separate report shape and does not use this envelope",
	}
}
