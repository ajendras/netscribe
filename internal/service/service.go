package service

import (
	"fmt"
	"strings"

	"github.com/netscribe/netscribe/internal/generator"
	"github.com/netscribe/netscribe/internal/llm"
	"github.com/netscribe/netscribe/internal/models"
)

// LLM abstracts interactions with a local large language model (implemented
// by internal/llm.Client). It is used to intelligently name endpoints and
// infer types from observed traffic patterns.
type LLM interface {
	// Complete runs a prompt against the specified Ollama model and returns
	// the generated text. It handles retries and streaming internally.
	Complete(prompt, model string) (string, error)
}

// Service coordinates parsing results, LLM augmentation, and dispatches to
// the appropriate generator. All operations stay on-device for privacy.
type Service struct {
	llm LLM
}

// New initializes a Service with an LLM backend. If nil is provided a
// default Ollama client is created.
func New(l LLM) *Service {
	if l == nil {
		l = llm.NewClient()
	}
	return &Service{llm: l}
}

// Generate dispatches the request to the correct generator after optionally
// enriching the TrafficData with LLM-derived names and summaries. Returns
// a complete Artifact containing the generated source code.
func (s *Service) Generate(req models.GenerateRequest) (models.Artifact, error) {
	if req.Traffic == nil {
		return models.Artifact{}, fmt.Errorf("traffic data is required")
	}
	if req.Type == "" {
		req.Type = "client"
	}
	if !isValidType(req.Type) {
		return models.Artifact{}, fmt.Errorf("unsupported type %q: must be client, mock or test", req.Type)
	}
	if (req.Type == "client" || req.Type == "test") && req.Lang == "" {
		if req.Type == "client" {
			req.Lang = "typescript"
		} else {
			req.Lang = "go"
		}
	}
	if req.Type != "mock" && req.Lang != "" && !isValidLang(req.Lang) {
		return models.Artifact{}, fmt.Errorf("unsupported language %q: must be typescript, go or python", req.Lang)
	}

	// Enrich endpoint metadata with LLM before generation (non-fatal).
	if err := s.enrichWithLLM(req.Traffic); err != nil {
		// continue with best-effort data; enrichment is optional
	}

	var (
		content string
		err     error
	)

	switch req.Type {
	case "client":
		content, err = generator.GenerateClient(req.Traffic, req.Lang)
	case "mock":
		content, err = generator.GenerateMock(req.Traffic)
	case "test":
		content, err = generator.GenerateTest(req.Traffic, req.Lang)
	default:
		return models.Artifact{}, fmt.Errorf("unreachable: unknown type %s", req.Type)
	}

	if err != nil {
		return models.Artifact{}, fmt.Errorf("generator failure for %s: %w", req.Type, err)
	}

	filename := deriveFilename(req.Type, req.Lang)

	return models.Artifact{
		Content:  content,
		Type:     req.Type,
		Language: req.Lang,
		Filename: filename,
	}, nil
}

// enrichWithLLM queries the local LLM to derive friendly operation names
// for each endpoint based on HTTP method, path and captured examples.
func (s *Service) enrichWithLLM(td *models.TrafficData) error {
	if s.llm == nil || len(td.Endpoints) == 0 {
		return nil
	}

	const model = "llama3"

	for i := range td.Endpoints {
		ep := &td.Endpoints[i]
		prompt := fmt.Sprintf(`You are an expert API designer.
Given the endpoint below, respond with ONLY a concise camelCase name (e.g. getUserProfile, createPaymentIntent).
Do not include explanation, quotes or markdown.

Method: %s
Path: %s
Observed examples: %d

Name:`, ep.Method, ep.Path, len(ep.Examples))

		name, err := s.llm.Complete(prompt, model)
		if err == nil && name != "" {
			clean := strings.TrimSpace(name)
			clean = strings.Trim(clean, "`\"'")
			if clean != "" {
				ep.Name = clean
			}
		}
		// models.Endpoint.InferSchema() can be called here when implemented
	}
	return nil
}

func isValidType(t string) bool {
	return t == "client" || t == "mock" || t == "test"
}

func isValidLang(l string) bool {
	return l == "typescript" || l == "go" || l == "python"
}

func deriveFilename(typ, lang string) string {
	switch typ {
	case "client":
		switch lang {
		case "typescript":
			return "client.ts"
		case "go":
			return "client.go"
		case "python":
			return "client.py"
		default:
			return "client." + lang
		}
	case "mock":
		return "mock-server.js"
	case "test":
		switch lang {
		case "go":
			return "client_test.go"
		case "typescript":
			return "client.test.ts"
		default:
			return "test." + lang
		}
	}
	return "artifact.txt"
}
