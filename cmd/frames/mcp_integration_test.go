package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	appconfig "github.com/wraelen/framescli/internal/config"
	"github.com/wraelen/framescli/internal/media"
)

// This harness drives the MCP server through a real stdio subprocess while
// replacing ffmpeg/ffprobe/whisper with deterministic fixtures for CI.
func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("FRAMESCLI_MCP_TEST") != "1" {
		return
	}

	root := os.Getenv("FRAMESCLI_MCP_TEST_ROOT")
	if root == "" {
		fmt.Fprintln(os.Stderr, "missing FRAMESCLI_MCP_TEST_ROOT")
		os.Exit(2)
	}
	cfg := appconfig.Default()
	cfg.FramesRoot = root
	cfg.AgentOutputRoot = filepath.Join(root, "agent-output")
	cfg.AgentInputDirs = []string{root}
	cfg.TranscribeBackend = "whisper"
	if whisperBin := strings.TrimSpace(os.Getenv("WHISPER_BIN")); whisperBin != "" {
		cfg.WhisperBin = whisperBin
	}
	appCfg = cfg
	appconfig.ApplyEnvDefaults(appCfg)
	if err := os.MkdirAll(cfg.AgentOutputRoot, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	state := newMCPServerState(os.Stdout)
	if ms, err := strconv.Atoi(os.Getenv("FRAMESCLI_MCP_TEST_HEARTBEAT_MS")); err == nil && ms > 0 {
		state.heartbeat = time.Duration(ms) * time.Millisecond
	}
	if err := runMCPServerWithState(os.Stdin, state); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func TestMCPServerHandshake(t *testing.T) {
	srv := startMCPTestServer(t, 0, 0)
	defer srv.Close(t)

	initPayload := srv.initializeSession(t)
	if initPayload.ProtocolVersion != "2024-11-05" {
		t.Fatalf("unexpected protocol version: %q", initPayload.ProtocolVersion)
	}
	if initPayload.ServerInfo.Name != "framescli" {
		t.Fatalf("unexpected server name: %q", initPayload.ServerInfo.Name)
	}

	srv.send(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	toolsResp := srv.waitForID(t, "2", time.Second)
	if toolsResp.Error != nil {
		t.Fatalf("tools/list returned error: %+v", toolsResp.Error)
	}
	var toolsPayload struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	decodeJSON(t, toolsResp.Result, &toolsPayload)
	seen := map[string]bool{}
	for _, tool := range toolsPayload.Tools {
		seen[tool.Name] = true
	}
	for _, name := range []string{"doctor", "preview", "transcribe_run"} {
		if !seen[name] {
			t.Fatalf("tools/list missing %q", name)
		}
	}
}

func TestMCPServerDoctorAndPreview(t *testing.T) {
	srv := startMCPTestServer(t, 0, 0)
	defer srv.Close(t)
	srv.initializeSession(t)

	srv.send(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      10,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "doctor",
			"arguments": map[string]any{},
		},
	})
	doctorResp := srv.waitForID(t, "10", time.Second)
	if doctorResp.Error != nil {
		t.Fatalf("doctor returned error: %+v", doctorResp.Error)
	}
	doctorEnv := decodeToolEnvelope(t, doctorResp)
	if doctorEnv.Status != "success" || doctorEnv.Command != "doctor" {
		t.Fatalf("unexpected doctor envelope: %+v", doctorEnv)
	}

	videoPath := filepath.Join(srv.root, "sample.mp4")
	if err := os.WriteFile(videoPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv.send(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      11,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "preview",
			"arguments": map[string]any{
				"input":  videoPath,
				"fps":    2,
				"format": "png",
				"mode":   "both",
			},
		},
	})
	previewResp := srv.waitForID(t, "11", time.Second)
	if previewResp.Error != nil {
		t.Fatalf("preview returned error: %+v", previewResp.Error)
	}
	previewEnv := decodeToolEnvelope(t, previewResp)
	if previewEnv.Status != "success" || previewEnv.Command != "preview" {
		t.Fatalf("unexpected preview envelope: %+v", previewEnv)
	}
	data, ok := previewEnv.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected preview data object, got %T", previewEnv.Data)
	}
	if got := data["resolved"]; got != videoPath {
		t.Fatalf("expected resolved path %q, got %#v", videoPath, got)
	}
	if got := data["target_fps"]; got != float64(2) {
		t.Fatalf("expected target_fps=2, got %#v", got)
	}
}

func TestMCPServerTranscribeRunHeartbeatAndSuccess(t *testing.T) {
	srv := startMCPTestServer(t, 25, 120)
	defer srv.Close(t)
	srv.initializeSession(t)

	runDir := srv.makeRunDir(t)

	srv.send(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      20,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "transcribe_run",
			"arguments": map[string]any{
				"run_dir": runDir,
			},
		},
	})

	var heartbeatSeen bool
	var resp mcpEnvelope
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		msg := srv.nextMessage(t, 2*time.Second)
		if msg.Method == "notifications/message" {
			heartbeatSeen = true
			continue
		}
		if string(msg.ID) == "20" {
			resp = msg
			break
		}
	}
	if !heartbeatSeen {
		t.Fatalf("expected heartbeat notification before transcribe_run completion")
	}
	if resp.Error != nil {
		t.Fatalf("transcribe_run returned error: %+v", resp.Error)
	}
	env := decodeToolEnvelope(t, resp)
	if env.Status != "success" || env.Command != "transcribe_run" {
		t.Fatalf("unexpected transcribe_run envelope: %+v", env)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected transcribe_run data object, got %T", env.Data)
	}
	if data["transcript_json"] == "" || data["transcript_txt"] == "" {
		t.Fatalf("expected transcript artifacts, got %+v", data)
	}
}

func TestMCPServerTranscribeRunCancellationAndTimeout(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		srv := startMCPTestServer(t, 25, 500)
		defer srv.Close(t)
		srv.initializeSession(t)

		runDir := srv.makeRunDir(t)

		srv.send(t, map[string]any{
			"jsonrpc": "2.0",
			"id":      30,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "transcribe_run",
				"arguments": map[string]any{
					"run_dir": runDir,
				},
			},
		})

		for {
			msg := srv.nextMessage(t, 2*time.Second)
			if msg.Method != "notifications/message" {
				continue
			}
			srv.send(t, map[string]any{
				"jsonrpc": "2.0",
				"method":  "notifications/cancelled",
				"params": map[string]any{
					"requestId": 30,
				},
			})
			break
		}

		resp := srv.waitForID(t, "30", 2*time.Second)
		if resp.Error == nil {
			t.Fatalf("expected cancellation error response")
		}
		if resp.Error.Code != -32800 || resp.Error.Message != "tool call cancelled" {
			t.Fatalf("unexpected cancellation error: %+v", resp.Error)
		}
		if resp.Error.Data["class"] != "interrupted" {
			t.Fatalf("expected interruption metadata, got %+v", resp.Error.Data)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		srv := startMCPTestServer(t, 25, 500)
		defer srv.Close(t)
		srv.initializeSession(t)

		runDir := srv.makeRunDir(t)

		srv.send(t, map[string]any{
			"jsonrpc": "2.0",
			"id":      31,
			"method":  "tools/call",
			"params": map[string]any{
				"name":       "transcribe_run",
				"timeout_ms": 80,
				"arguments": map[string]any{
					"run_dir": runDir,
				},
			},
		})
		resp := srv.waitForID(t, "31", 2*time.Second)
		if resp.Error == nil {
			t.Fatalf("expected timeout error response")
		}
		if resp.Error.Code != -32001 || resp.Error.Message != "tool call timed out" {
			t.Fatalf("unexpected timeout error: %+v", resp.Error)
		}
	})
}

func TestMCPServerErrorDataForInvalidPrefsSet(t *testing.T) {
	srv := startMCPTestServer(t, 0, 0)
	defer srv.Close(t)
	srv.initializeSession(t)

	srv.send(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      40,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "prefs_set",
			"arguments": map[string]any{
				"output_root": "https://example.com/not-local",
			},
		},
	})

	resp := srv.waitForID(t, "40", time.Second)
	if resp.Error == nil {
		t.Fatal("expected prefs_set error")
	}
	if resp.Error.Code != -32000 {
		t.Fatalf("unexpected prefs_set error code: %+v", resp.Error)
	}
	if resp.Error.Data["class"] != "path_not_allowed" {
		t.Fatalf("unexpected prefs_set error data: %+v", resp.Error.Data)
	}
	if resp.Error.Data["recovery"] == "" {
		t.Fatalf("expected recovery guidance in prefs_set error data: %+v", resp.Error.Data)
	}
}

type mcpTestServer struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	messages chan mcpEnvelope
	readErrs chan error
	root     string
	stderr   bytes.Buffer
}

type mcpEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpInitializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name string `json:"name"`
	} `json:"serverInfo"`
}

func startMCPTestServer(t *testing.T, heartbeatMS int, transcribeDelayMS int) *mcpTestServer {
	t.Helper()

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeMCPTools(t, binDir)

	cmd := exec.Command(os.Args[0], "-test.run=TestMCPHelperProcess")
	cmd.Env = append(os.Environ(),
		"FRAMESCLI_MCP_TEST=1",
		"FRAMESCLI_MCP_TEST_ROOT="+root,
		fmt.Sprintf("FRAMESCLI_MCP_TEST_HEARTBEAT_MS=%d", heartbeatMS),
		fmt.Sprintf("FRAMESCLI_MCP_FAKE_TRANSCRIBE_DELAY_MS=%d", transcribeDelayMS),
		"HOME="+root,
		"XDG_CONFIG_HOME="+filepath.Join(root, ".config"),
		"FRAMES_CONFIG="+filepath.Join(root, ".config", "framescli", "config.json"),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"WHISPER_BIN="+filepath.Join(binDir, "whisper"),
		"TRANSCRIBE_BACKEND=whisper",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	srv := &mcpTestServer{
		cmd:      cmd,
		stdin:    stdin,
		messages: make(chan mcpEnvelope, 32),
		readErrs: make(chan error, 1),
		root:     root,
	}
	cmd.Stderr = &srv.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go srv.readLoop(stdout)
	return srv
}

func (s *mcpTestServer) Close(t *testing.T) {
	t.Helper()
	closeErr := s.stdin.Close()
	waitErr := s.cmd.Wait()
	if waitErr != nil {
		if closeErr != nil {
			t.Fatalf("mcp helper exited unexpectedly: %v (stdin close: %v)\nstderr:\n%s", waitErr, closeErr, s.stderr.String())
		}
		t.Fatalf("mcp helper exited unexpectedly: %v\nstderr:\n%s", waitErr, s.stderr.String())
	}
	if closeErr != nil {
		t.Fatalf("failed closing helper stdin: %v\nstderr:\n%s", closeErr, s.stderr.String())
	}
}

func (s *mcpTestServer) initializeSession(t *testing.T) mcpInitializeResult {
	t.Helper()
	s.send(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{},
	})
	initResp := s.waitForID(t, "1", time.Second)
	if initResp.Error != nil {
		t.Fatalf("initialize returned error: %+v", initResp.Error)
	}
	var initPayload mcpInitializeResult
	decodeJSON(t, initResp.Result, &initPayload)

	s.send(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})
	s.expectNoMessage(t, 150*time.Millisecond)
	return initPayload
}

func (s *mcpTestServer) send(t *testing.T, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(s.stdin, "Content-Length: %d\r\n\r\n", len(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.stdin.Write(raw); err != nil {
		t.Fatal(err)
	}
}

func (s *mcpTestServer) nextMessage(t *testing.T, timeout time.Duration) mcpEnvelope {
	t.Helper()
	select {
	case msg := <-s.messages:
		return msg
	case err := <-s.readErrs:
		t.Fatalf("mcp server read failed: %v\nstderr:\n%s", err, s.stderr.String())
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for mcp message\nstderr:\n%s", s.stderr.String())
	}
	return mcpEnvelope{}
}

func (s *mcpTestServer) waitForID(t *testing.T, want string, timeout time.Duration) mcpEnvelope {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		msg := s.nextMessage(t, time.Until(deadline))
		if string(msg.ID) == want {
			return msg
		}
	}
	t.Fatalf("timed out waiting for response id=%s\nstderr:\n%s", want, s.stderr.String())
	return mcpEnvelope{}
}

func (s *mcpTestServer) expectNoMessage(t *testing.T, duration time.Duration) {
	t.Helper()
	select {
	case msg := <-s.messages:
		t.Fatalf("expected no message, got %+v", msg)
	case err := <-s.readErrs:
		if err != io.EOF {
			t.Fatalf("unexpected read error: %v\nstderr:\n%s", err, s.stderr.String())
		}
	case <-time.After(duration):
	}
}

func (s *mcpTestServer) makeRunDir(t *testing.T) string {
	t.Helper()
	runDir := filepath.Join(s.root, "Run_Integration")
	if err := os.MkdirAll(filepath.Join(runDir, "voice"), 0o755); err != nil {
		t.Fatal(err)
	}
	audioPath := filepath.Join(runDir, "voice", "voice.wav")
	if err := os.WriteFile(audioPath, []byte("fake audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := media.RunMetadata{
		VideoPath:      filepath.Join(s.root, "sample.mp4"),
		FPS:            2,
		FrameFormat:    "png",
		ExtractedAudio: true,
		AudioPath:      audioPath,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	raw, err := json.Marshal(md)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(media.MetadataPathForRun(runDir), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return runDir
}

func (s *mcpTestServer) readLoop(r io.Reader) {
	br := bufio.NewReader(r)
	for {
		msg, err := readMCPEnvelope(br)
		if err != nil {
			s.readErrs <- err
			return
		}
		s.messages <- msg
	}
}

func readMCPEnvelope(br *bufio.Reader) (mcpEnvelope, error) {
	headers := map[string]string{}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return mcpEnvelope{}, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		headers[strings.ToLower(strings.TrimSpace(parts[0]))] = strings.TrimSpace(parts[1])
	}
	n, err := strconv.Atoi(headers["content-length"])
	if err != nil || n <= 0 {
		return mcpEnvelope{}, fmt.Errorf("invalid content-length: %q", headers["content-length"])
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(br, body); err != nil {
		return mcpEnvelope{}, err
	}
	var msg mcpEnvelope
	if err := json.Unmarshal(body, &msg); err != nil {
		return mcpEnvelope{}, err
	}
	return msg, nil
}

func decodeToolEnvelope(t *testing.T, resp mcpEnvelope) automationEnvelope {
	t.Helper()
	var wrapper struct {
		StructuredContent automationEnvelope `json:"structuredContent"`
	}
	decodeJSON(t, resp.Result, &wrapper)
	return wrapper.StructuredContent
}

func decodeJSON(t *testing.T, raw json.RawMessage, target any) {
	t.Helper()
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("failed decoding json %s: %v", string(raw), err)
	}
}

func writeFakeMCPTools(t *testing.T, dir string) {
	t.Helper()
	writeFakeTool(t, filepath.Join(dir, "ffmpeg"), `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "-version" ]]; then
  printf 'ffmpeg version test-build\n'
  exit 0
fi
if [[ "${1:-}" == "-hide_banner" && "${2:-}" == "-hwaccels" ]]; then
  cat <<'EOF'
Hardware acceleration methods:
cuda
vaapi
EOF
  exit 0
fi
echo "unexpected ffmpeg args: $*" >&2
exit 1
`)
	writeFakeTool(t, filepath.Join(dir, "ffprobe"), `#!/usr/bin/env bash
set -euo pipefail
cat <<'EOF'
{"format":{"duration":"12.5"},"streams":[{"codec_type":"video","width":1280,"height":720,"r_frame_rate":"30/1"},{"codec_type":"audio"}]}
EOF
`)
	writeFakeTool(t, filepath.Join(dir, "whisper"), `#!/usr/bin/env bash
set -euo pipefail
audio="${1:?audio path required}"
shift || true
out_dir=""
while [[ "$#" -gt 0 ]]; do
  if [[ "$1" == "--output_dir" && "$#" -ge 2 ]]; then
    out_dir="$2"
    shift 2
    continue
  fi
  shift
done
if [[ -z "$out_dir" ]]; then
  echo "missing --output_dir" >&2
  exit 1
fi
delay_ms="${FRAMESCLI_MCP_FAKE_TRANSCRIBE_DELAY_MS:-0}"
if [[ "$delay_ms" -gt 0 ]]; then
  sleep "$(awk "BEGIN { printf \"%.3f\", $delay_ms / 1000 }")"
fi
base="$(basename "$audio")"
base="${base%.*}"
cat > "$out_dir/$base.json" <<'JSON'
{"text":"integration transcript","segments":[{"start":0.0,"end":0.5,"text":"integration transcript"}]}
JSON
`)
}

func writeFakeTool(t *testing.T, path string, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}
