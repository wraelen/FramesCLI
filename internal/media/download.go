package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// VideoMetadata contains information about a downloaded video
type VideoMetadata struct {
	Title       string  `json:"title"`
	Uploader    string  `json:"uploader"`
	Duration    float64 `json:"duration"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	OriginalURL string  `json:"original_url"`
	Timestamp   int64   `json:"timestamp"`
	Format      string  `json:"format"`
	Filesize    int64   `json:"filesize"`
}

// DownloadResult contains the result of a video download
type DownloadResult struct {
	LocalPath  string
	Metadata   VideoMetadata
	WasCached  bool
	DownloadMS int64
}

// DownloadOptions configures video download behavior
type DownloadOptions struct {
	CacheDir       string
	UseCache       bool
	ProgressWriter io.Writer
	Context        context.Context
}

// NormalizeURL creates a canonical form of a URL for caching purposes
func NormalizeURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Lowercase scheme and host
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)

	// Sort query parameters for consistent cache keys
	if parsed.RawQuery != "" {
		query := parsed.Query()
		keys := make([]string, 0, len(query))
		for k := range query {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		normalizedQuery := url.Values{}
		for _, k := range keys {
			normalizedQuery[k] = query[k]
		}
		parsed.RawQuery = normalizedQuery.Encode()
	}

	return parsed.String(), nil
}

// URLHash generates a SHA256 hash for a URL to use as a cache key
func URLHash(rawURL string) (string, error) {
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:]), nil
}

// CheckYtDlp verifies that yt-dlp is installed and accessible
func CheckYtDlp() (string, error) {
	cmd := exec.Command("yt-dlp", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp not found - install with: pip install yt-dlp OR brew install yt-dlp")
	}

	version := strings.TrimSpace(string(output))
	return version, nil
}

// DownloadVideo downloads a video from a URL using yt-dlp
// Returns the local path, metadata, and whether it was cached
func DownloadVideo(rawURL string, opts DownloadOptions) (*DownloadResult, error) {
	if opts.Context == nil {
		opts.Context = context.Background()
	}

	// Check if yt-dlp is available
	if _, err := CheckYtDlp(); err != nil {
		return nil, err
	}

	// Set default cache dir if not provided
	if opts.CacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		opts.CacheDir = filepath.Join(homeDir, ".cache", "framescli", "videos")
	}

	// Create cache directory if it doesn't exist (0700 = user-only access for privacy)
	if err := os.MkdirAll(opts.CacheDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Generate cache key
	urlHash, err := URLHash(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to hash URL: %w", err)
	}

	// Check if video is already cached
	cachedPath := filepath.Join(opts.CacheDir, urlHash+".mp4")
	metadataPath := filepath.Join(opts.CacheDir, urlHash+".json")

	if opts.UseCache {
		if _, err := os.Stat(cachedPath); err == nil {
			// Cache hit - load metadata
			var metadata VideoMetadata
			if metadataBytes, err := os.ReadFile(metadataPath); err == nil {
				if err := json.Unmarshal(metadataBytes, &metadata); err == nil {
					return &DownloadResult{
						LocalPath:  cachedPath,
						Metadata:   metadata,
						WasCached:  true,
						DownloadMS: 0,
					}, nil
				}
			}

			// Metadata missing or corrupt, but video exists - return path only
			return &DownloadResult{
				LocalPath:  cachedPath,
				Metadata:   VideoMetadata{OriginalURL: rawURL},
				WasCached:  true,
				DownloadMS: 0,
			}, nil
		}
	}

	// Cache miss - download video
	if opts.ProgressWriter != nil {
		fmt.Fprintf(opts.ProgressWriter, "Downloading video from %s...\n", rawURL)
	}

	// First, get metadata with -J (don't download yet)
	metaCmd := exec.CommandContext(opts.Context, "yt-dlp", "-J", rawURL)
	metaOutput, err := metaCmd.Output()
	if err != nil {
		return nil, parseYtDlpError(err)
	}

	var metadata VideoMetadata
	var rawMeta map[string]interface{}
	if err := json.Unmarshal(metaOutput, &rawMeta); err != nil {
		return nil, fmt.Errorf("failed to parse video metadata: %w", err)
	}

	// Extract relevant fields from yt-dlp JSON output
	metadata.Title, _ = rawMeta["title"].(string)
	metadata.Uploader, _ = rawMeta["uploader"].(string)
	metadata.Duration, _ = rawMeta["duration"].(float64)
	metadata.OriginalURL = rawURL
	metadata.Format, _ = rawMeta["format"].(string)

	// yt-dlp returns numbers as float64, need to convert to int
	if width, ok := rawMeta["width"].(float64); ok {
		metadata.Width = int(width)
	}
	if height, ok := rawMeta["height"].(float64); ok {
		metadata.Height = int(height)
	}
	if timestamp, ok := rawMeta["timestamp"].(float64); ok {
		metadata.Timestamp = int64(timestamp)
	}

	// Filesize might be in different fields
	if filesize, ok := rawMeta["filesize"].(float64); ok {
		metadata.Filesize = int64(filesize)
	} else if filesize, ok := rawMeta["filesize_approx"].(float64); ok {
		metadata.Filesize = int64(filesize)
	}

	// Download video with progress
	args := []string{
		"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
		"--merge-output-format", "mp4",
		"-o", cachedPath,
		rawURL,
	}

	// Add progress output if writer provided
	if opts.ProgressWriter != nil {
		args = append([]string{"--newline"}, args...)
	}

	downloadCmd := exec.CommandContext(opts.Context, "yt-dlp", args...)

	if opts.ProgressWriter != nil {
		downloadCmd.Stdout = opts.ProgressWriter
		downloadCmd.Stderr = opts.ProgressWriter
	}

	if err := downloadCmd.Run(); err != nil {
		return nil, parseYtDlpError(err)
	}

	// Save metadata
	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err == nil {
		_ = os.WriteFile(metadataPath, metadataBytes, 0644)
	}

	return &DownloadResult{
		LocalPath:  cachedPath,
		Metadata:   metadata,
		WasCached:  false,
		DownloadMS: 0,
	}, nil
}

// parseYtDlpError converts yt-dlp errors into user-friendly messages
func parseYtDlpError(err error) error {
	if err == nil {
		return nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderr := string(exitErr.Stderr)

		// Check for common error patterns
		if strings.Contains(stderr, "Video unavailable") {
			return fmt.Errorf("video unavailable (deleted or private)")
		}
		if strings.Contains(stderr, "not available in your") {
			return fmt.Errorf("video not available in your region (use VPN or proxy)")
		}
		if strings.Contains(stderr, "Sign in") || strings.Contains(stderr, "requires authentication") {
			return fmt.Errorf("video requires authentication (use yt-dlp cookies)")
		}
		if strings.Contains(stderr, "Unable to download") {
			return fmt.Errorf("download failed (check connection, retry with --no-cache)")
		}
	}

	return fmt.Errorf("yt-dlp error: %w", err)
}

// CleanCache removes all cached videos
func CleanCache(cacheDir string) error {
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		cacheDir = filepath.Join(homeDir, ".cache", "framescli", "videos")
	}

	return os.RemoveAll(cacheDir)
}
