package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ollama/ollama/api"
)

// Client provides access to a local Ollama instance. It implements the
// LLM interface used by the service layer for endpoint naming and
// optional schema enrichment. All processing stays on the local machine.
type Client struct{}

// NewClient returns a ready-to-use Ollama client. The endpoint is read
// from the OLLAMA_HOST environment variable (default: http://localhost:11434).
func NewClient() *Client {
	return &Client{}
}

// Complete runs a prompt against the requested model and returns the
// trimmed response. It streams results internally, applies 3 retries
// with exponential backoff on transient failures, and cleans common
// LLM output artifacts (quotes, markdown, extra labels).
func (c *Client) Complete(prompt, model string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	if model == "" {
		model = "llama3"
	}

	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://localhost:11434"
	}

	cli, err := api.ClientFromEnvironment()
	if err != nil {
		// fallback construction using explicit host
		cli, err = api.NewClient(api.WithHost(host))
		if err != nil {
			return "", fmt.Errorf("failed to create ollama client: %w", err)
		}
	}

	req := &api.GenerateRequest{
		Model:   model,
		Prompt:  prompt,
		Stream:  true,
		Options: map[string]any{"temperature": 0.1, "top_p": 0.9},
	}

	const maxRetries = 3
	var response strings.Builder

	for attempt := 0; attempt < maxRetries; attempt++ {
		response.Reset()

		err = cli.Generate(context.Background(), req, func(gr api.GenerateResponse) error {
			response.WriteString(gr.Response)
			return nil
		})

		if err == nil {
			break
		}

		if attempt < maxRetries-1 {
			backoff := time.Duration(1<<uint(attempt)) * 350 * time.Millisecond
			time.Sleep(backoff)
			continue
		}
	}

	if err != nil {
		return "", fmt.Errorf("ollama request failed after %d attempts: %w", maxRetries, err)
	}

	result := strings.TrimSpace(response.String())
	// strip common LLM wrappers
	result = strings.Trim(result, "`\"'\n ")
	if idx := strings.LastIndexAny(result, "\n:"); idx != -1 && idx < len(result)-1 {
		result = strings.TrimSpace(result[idx+1:])
	}

	if result == "" {
		return "", fmt.Errorf("empty response from model")
	}

	return result, nil
}
