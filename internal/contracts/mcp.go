package contracts

const AutomationSchemaVersion = "framescli.v1"

const (
	MCPToolDoctor             = "doctor"
	MCPToolPreview            = "preview"
	MCPToolExtract            = "extract"
	MCPToolExtractBatch       = "extract_batch"
	MCPToolOpenLast           = "open_last"
	MCPToolGetLatestArtifacts = "get_latest_artifacts"
	MCPToolGetRunArtifacts    = "get_run_artifacts"
	MCPToolTranscribeRun      = "transcribe_run"
	MCPToolPrefsGet           = "prefs_get"
	MCPToolPrefsSet           = "prefs_set"
)

const (
	EnvelopeCommandDoctor             = "doctor"
	EnvelopeCommandPreview            = "preview"
	EnvelopeCommandExtract            = "extract"
	EnvelopeCommandExtractBatch       = "extract-batch"
	EnvelopeCommandOpenLast           = "open-last"
	EnvelopeCommandGetLatestArtifacts = "get_latest_artifacts"
	EnvelopeCommandGetRunArtifacts    = "get_run_artifacts"
	EnvelopeCommandTranscribeRun      = "transcribe_run"
	EnvelopeCommandPrefsGet           = "prefs_get"
	EnvelopeCommandPrefsSet           = "prefs_set"
)

type MCPTool struct {
	Name            string
	Description     string
	EnvelopeCommand string
	Args            any
}

type MCPToolDescriptor struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func MCPTools() []MCPTool {
	return append([]MCPTool(nil), mcpTools...)
}

func MCPToolDescriptors() []MCPToolDescriptor {
	tools := make([]MCPToolDescriptor, 0, len(mcpTools))
	for _, tool := range mcpTools {
		tools = append(tools, MCPToolDescriptor{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema(),
		})
	}
	return tools
}

func (t MCPTool) InputSchema() map[string]any {
	return inputSchemaFromArgs(t.Args)
}

var mcpTools = []MCPTool{
	{
		Name:            MCPToolDoctor,
		Description:     "Check local toolchain readiness for FramesCLI",
		EnvelopeCommand: EnvelopeCommandDoctor,
		Args:            struct{}{},
	},
	{
		Name:            MCPToolPreview,
		Description:     "Dry-run output estimate for a video input (defaults to recent when omitted)",
		EnvelopeCommand: EnvelopeCommandPreview,
		Args:            PreviewArgs{},
	},
	{
		Name:            MCPToolExtract,
		Description:     "Run single-video extraction workflow (input defaults to recent, or use url for remote videos)",
		EnvelopeCommand: EnvelopeCommandExtract,
		Args:            ExtractArgs{},
	},
	{
		Name:            MCPToolExtractBatch,
		Description:     "Run extraction for multiple inputs/globs (falls back to configured agent_input_dirs)",
		EnvelopeCommand: EnvelopeCommandExtractBatch,
		Args:            ExtractBatchArgs{},
	},
	{
		Name:            MCPToolOpenLast,
		Description:     "Resolve path for artifact from most recent run",
		EnvelopeCommand: EnvelopeCommandOpenLast,
		Args:            OpenLastArgs{},
	},
	{
		Name:            MCPToolGetLatestArtifacts,
		Description:     "Return paths for available artifacts from the most recent run",
		EnvelopeCommand: EnvelopeCommandGetLatestArtifacts,
		Args:            GetLatestArtifactsArgs{},
	},
	{
		Name:            MCPToolGetRunArtifacts,
		Description:     "Return indexed artifacts for the latest, a specific run, or a recent run list",
		EnvelopeCommand: EnvelopeCommandGetRunArtifacts,
		Args:            GetRunArtifactsArgs{},
	},
	{
		Name:            MCPToolTranscribeRun,
		Description:     "Resume transcription for an existing run directory",
		EnvelopeCommand: EnvelopeCommandTranscribeRun,
		Args:            TranscribeRunArgs{},
	},
	{
		Name:            MCPToolPrefsGet,
		Description:     "Get agent path preferences for input and output roots",
		EnvelopeCommand: EnvelopeCommandPrefsGet,
		Args:            struct{}{},
	},
	{
		Name:            MCPToolPrefsSet,
		Description:     "Persist agent path preferences for input and output roots",
		EnvelopeCommand: EnvelopeCommandPrefsSet,
		Args:            PrefsSetArgs{},
	},
}
