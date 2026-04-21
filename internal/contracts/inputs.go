package contracts

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type FPSSpec struct {
	Value    float64
	Explicit bool
}

func (s *FPSSpec) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*s = FPSSpec{}
		return nil
	}
	s.Explicit = true
	if raw == `"auto"` {
		s.Value = 0
		return nil
	}
	if strings.HasPrefix(raw, `"`) {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		if strings.EqualFold(strings.TrimSpace(str), "auto") {
			s.Value = 0
			return nil
		}
		v, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return fmt.Errorf("invalid fps value. use a positive number, 0, or auto")
		}
		if v < 0 {
			return fmt.Errorf("invalid fps value. use a positive number, 0, or auto")
		}
		s.Value = v
		return nil
	}
	var num float64
	if err := json.Unmarshal(data, &num); err != nil {
		return fmt.Errorf("invalid fps value. use a positive number, 0, or auto")
	}
	if num < 0 {
		return fmt.Errorf("invalid fps value. use a positive number, 0, or auto")
	}
	s.Value = num
	return nil
}

func (s FPSSpec) Resolve(fallback float64) (float64, bool) {
	if !s.Explicit {
		return fallback, false
	}
	return s.Value, true
}

type PreviewArgs struct {
	Input         string  `json:"input"`
	FPS           FPSSpec `json:"fps"`
	Format        string  `json:"format"`
	Mode          string  `json:"mode"`
	Preset        string  `json:"preset"`
	ChunkDuration int     `json:"chunk_duration"`
}

type ExtractArgs struct {
	Input              string  `json:"input"`
	URL                string  `json:"url"`
	NoCache            bool    `json:"no_cache"`
	FPS                FPSSpec `json:"fps"`
	Voice              bool    `json:"voice"`
	Format             string  `json:"format"`
	Out                string  `json:"out"`
	HWAccel            string  `json:"hwaccel"`
	Preset             string  `json:"preset"`
	From               string  `json:"from"`
	To                 string  `json:"to"`
	FrameStart         int     `json:"frame_start"`
	FrameEnd           int     `json:"frame_end"`
	EveryN             int     `json:"every_n"`
	NameTemplate       string  `json:"name_template"`
	ChunkDuration      int     `json:"chunk_duration"`
	AllowExpensive     bool    `json:"allow_expensive"`
	TranscribeBackend  string  `json:"transcribe_backend"`
	TranscribeBin      string  `json:"transcribe_bin"`
	TranscribeLanguage string  `json:"transcribe_language"`
}

type ExtractBatchArgs struct {
	Inputs             []string `json:"inputs"`
	FPS                FPSSpec  `json:"fps"`
	Voice              bool     `json:"voice"`
	Format             string   `json:"format"`
	Preset             string   `json:"preset"`
	ChunkDuration      int      `json:"chunk_duration"`
	AllowExpensive     bool     `json:"allow_expensive"`
	TranscribeBackend  string   `json:"transcribe_backend"`
	TranscribeBin      string   `json:"transcribe_bin"`
	TranscribeLanguage string   `json:"transcribe_language"`
}

type OpenLastArgs struct {
	Root     string `json:"root"`
	Artifact string `json:"artifact"`
}

type GetLatestArtifactsArgs struct {
	Root string `json:"root"`
}

type GetRunArtifactsArgs struct {
	Root   string `json:"root"`
	Run    string `json:"run"`
	Recent int    `json:"recent"`
}

type TranscribeRunArgs struct {
	RunDir        string `json:"run_dir" contract:"required"`
	ChunkDuration int    `json:"chunk_duration"`
}

type PrefsSetArgs struct {
	InputDirs  *[]string `json:"input_dirs"`
	OutputRoot *string   `json:"output_root"`
}
