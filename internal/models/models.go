package models

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TrafficData holds the parsed HAR traffic grouped into logical endpoints.
type TrafficData struct {
	Endpoints     []Endpoint `json:"endpoints"`
	TotalRequests int        `json:"totalRequests,omitempty"`
}

// Endpoint represents a unique HTTP endpoint observed in the traffic.
type Endpoint struct {
	ID       string    `json:"id"`
	Method   string    `json:"method"`
	Path     string    `json:"path"`
	Name     string    `json:"name,omitempty"`
	Examples []Example `json:"examples,omitempty"`
}

// Request captures the salient parts of an observed HTTP request.
type Request struct {
	URL     string            `json:"url,omitempty"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Query   map[string]string `json:"query,omitempty"`
	Body    interface{}       `json:"body,omitempty"`
}

// Response captures the salient parts of an observed HTTP response.
type Response struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    interface{}       `json:"body,omitempty"`
}

// Example pairs a request with its corresponding response.
type Example struct {
	Request  Request  `json:"request"`
	Response Response `json:"response"`
}

// GenerateRequest is the JSON payload expected by the /api/generate endpoint.
type GenerateRequest struct {
	Traffic *TrafficData `json:"traffic"`
	Type    string       `json:"type"` // "client", "mock", or "test"
	Lang    string       `json:"lang,omitempty"`
}

// Artifact is returned by generation routines and the /api/generate handler.
type Artifact struct {
	Content  string `json:"content"`
	Type     string `json:"type"`
	Language string `json:"language,omitempty"`
	Filename string `json:"filename"`
}

// AddExample records a live request/response pair into the traffic data.
// It is used by the built-in MITM proxy to accumulate captured traffic
// without requiring a full HAR round-trip.
func (td *TrafficData) AddExample(method, url, path string, reqHeaders, reqQuery map[string]string, reqBody any, status int, respHeaders map[string]string, respBody any) {
	if td == nil || method == "" {
		return
	}
	if path == "" {
		path = "/"
	}
	key := method + ":" + path

	for i := range td.Endpoints {
		if td.Endpoints[i].ID == key {
			td.addToEndpoint(i, url, reqHeaders, reqQuery, reqBody, status, respHeaders, respBody)
			return
		}
	}

	td.Endpoints = append(td.Endpoints, Endpoint{
		ID:       key,
		Method:   method,
		Path:     path,
		Name:     "",
		Examples: []Example{},
	})
	td.addToEndpoint(len(td.Endpoints)-1, url, reqHeaders, reqQuery, reqBody, status, respHeaders, respBody)
}

func (td *TrafficData) addToEndpoint(idx int, url string, reqHeaders, reqQuery map[string]string, reqBody any, status int, respHeaders map[string]string, respBody any) {
	ex := Example{
		Request: Request{
			URL:     url,
			Method:  td.Endpoints[idx].Method,
			Path:    td.Endpoints[idx].Path,
			Headers: reqHeaders,
			Query:   reqQuery,
			Body:    reqBody,
		},
		Response: Response{
			Status:  status,
			Headers: respHeaders,
			Body:    respBody,
		},
	}
	td.Endpoints[idx].Examples = append(td.Endpoints[idx].Examples, ex)
	if len(td.Endpoints[idx].Examples) > 5 {
		td.Endpoints[idx].Examples = td.Endpoints[idx].Examples[:5]
	}
	td.TotalRequests++
}

// InferSchema returns a coarse type descriptor derived from the first
// observed response body. This is a foundation for richer inference
// (JSON Schema, OpenAPI, etc.) that can be driven by the LLM later.
func (e *Endpoint) InferSchema() string {
	if len(e.Examples) == 0 || e.Examples[0].Response.Body == nil {
		return "any"
	}

	switch v := e.Examples[0].Response.Body.(type) {
	case map[string]interface{}, []interface{}, map[string]any, []any:
		return "object"
	case string:
		if strings.HasPrefix(v, "http") || strings.Contains(v, "://") {
			return "url"
		}
		return "string"
	case float64, int, int64, json.Number:
		return "number"
	case bool:
		return "boolean"
	default:
		return "any"
	}
}

// ToOpenAPI returns a minimal but valid OpenAPI 3.1 document describing
// the observed endpoints. The spec can be enriched by the LLM or by
// calling InferSchema on each endpoint.
func (td *TrafficData) ToOpenAPI() (string, error) {
	if td == nil || len(td.Endpoints) == 0 {
		return "", fmt.Errorf("traffic data must contain at least one endpoint")
	}

	var b strings.Builder
	b.WriteString(`openapi: 3.1.0
info:
  title: NetScribe Generated API
  description: Observed from real traffic via local HAR + LLM
  version: 0.1.0
servers:
  - url: https://api.example.com
paths:
`)

	for _, ep := range td.Endpoints {
		opID := ep.Name
		if opID == "" {
			opID = "op" + strings.ToUpper(ep.Method) + strings.ReplaceAll(strings.Trim(ep.Path, "/"), "/", "_")
		}
		b.WriteString(fmt.Sprintf(`  %s:
    %s:
      operationId: %s
      summary: %s
      responses:
        '%d':
          description: Successful response
          content:
            application/json:
              schema:
                type: object
`, ep.Path, strings.ToLower(ep.Method), opID, ep.Method+" "+ep.Path, 200))
	}

	b.WriteString("\ncomponents:\n  schemas: {}\n")
	return b.String(), nil
}
