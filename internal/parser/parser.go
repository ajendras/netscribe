package parser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/netscribe/netscribe/internal/models"
)

// harFile is the root object of a standard HAR 1.2 file.
type harFile struct {
	Log harLog `json:"log"`
}

type harLog struct {
	Entries []harEntry `json:"entries"`
}

type harEntry struct {
	Request  harRequest  `json:"request"`
	Response harResponse `json:"response"`
}

type harRequest struct {
	Method      string         `json:"method"`
	URL         string         `json:"url"`
	Headers     []nameValue    `json:"headers"`
	QueryString []nameValue    `json:"queryString"`
	PostData    *harPostData   `json:"postData,omitempty"`
}

type harResponse struct {
	Status  int          `json:"status"`
	Headers []nameValue  `json:"headers"`
	Content *harContent  `json:"content,omitempty"`
}

type nameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type harPostData struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

type harContent struct {
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

// ParseHAR converts a HAR JSON document into a TrafficData value.
// Entries are grouped by method+pathname. Headers, query params and
// request/response bodies (with JSON parsing when possible) are extracted.
// The resulting structure is ready for LLM augmentation and code generation.
func ParseHAR(b []byte) (*models.TrafficData, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("HAR file is empty")
	}

	var f harFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("invalid HAR JSON: %w", err)
	}
	if len(f.Log.Entries) == 0 {
		return nil, fmt.Errorf("HAR contains no entries")
	}

	groups := make(map[string]*models.Endpoint)

	for _, entry := range f.Log.Entries {
		if entry.Request.URL == "" || entry.Request.Method == "" {
			continue
		}

		u, err := url.Parse(entry.Request.URL)
		if err != nil {
			continue
		}
		path := u.EscapedPath()
		if path == "" {
			path = "/"
		}

		key := entry.Request.Method + ":" + path
		if _, ok := groups[key]; !ok {
			groups[key] = &models.Endpoint{
				ID:     key,
				Method: entry.Request.Method,
				Path:   path,
				Name:   "", // populated later by LLM
			}
		}

		ep := groups[key]

		req := models.Request{
			URL:     entry.Request.URL,
			Method:  entry.Request.Method,
			Path:    path,
			Headers: extractNameValues(entry.Request.Headers),
			Query:   extractNameValues(entry.Request.QueryString),
			Body:    extractBody(entry.Request.PostData),
		}

		resp := models.Response{
			Status:  entry.Response.Status,
			Headers: extractNameValues(entry.Response.Headers),
			Body:    extractBody(entry.Response.Content),
		}

		ep.Examples = append(ep.Examples, models.Example{
			Request:  req,
			Response: resp,
		})

		// prevent unbounded memory growth from very chatty HARs
		if len(ep.Examples) > 5 {
			ep.Examples = ep.Examples[:5]
		}
	}

	endpoints := make([]models.Endpoint, 0, len(groups))
	for _, ep := range groups {
		endpoints = append(endpoints, *ep)
	}

	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Path != endpoints[j].Path {
			return endpoints[i].Path < endpoints[j].Path
		}
		return endpoints[i].Method < endpoints[j].Method
	})

	return &models.TrafficData{
		Endpoints:     endpoints,
		TotalRequests: len(f.Log.Entries),
	}, nil
}

func extractNameValues(nvs []nameValue) map[string]string {
	m := make(map[string]string, len(nvs))
	for _, nv := range nvs {
		if nv.Name != "" {
			m[nv.Name] = nv.Value
		}
	}
	return m
}

// extractBody accepts either a *harPostData or *harContent and returns
// a native Go value (parsed JSON object/array when appropriate).
func extractBody(v interface{}) interface{} {
	switch x := v.(type) {
	case *harPostData:
		if x == nil || x.Text == "" {
			return nil
		}
		return parsePossibleJSON(x.Text, x.MimeType)
	case *harContent:
		if x == nil || x.Text == "" {
			return nil
		}
		return parsePossibleJSON(x.Text, x.MimeType)
	}
	return nil
}

func parsePossibleJSON(text, mimeType string) interface{} {
	if text == "" {
		return nil
	}
	if strings.Contains(strings.ToLower(mimeType), "application/json") {
		var parsed interface{}
		if json.Unmarshal([]byte(text), &parsed) == nil {
			return parsed
		}
	}
	return text
}

// ToHAR builds a valid HAR 1.2 document from a TrafficData value.
// Used by the built-in proxy to let users download captures without
// leaving the capture session.
func ToHAR(td *models.TrafficData) ([]byte, error) {
	if td == nil {
		return nil, fmt.Errorf("traffic data is required")
	}

	var entries []harEntry
	for _, ep := range td.Endpoints {
		for _, ex := range ep.Examples {
			entries = append(entries, harEntry{
				Request: harRequest{
					Method:      ex.Request.Method,
					URL:         ex.Request.URL,
					Headers:     mapToNameValues(ex.Request.Headers),
					QueryString: mapToNameValues(ex.Request.Query),
					PostData:    bodyToPostData(ex.Request.Body),
				},
				Response: harResponse{
					Status:  ex.Response.Status,
					Headers: mapToNameValues(ex.Response.Headers),
					Content: bodyToContent(ex.Response.Body),
				},
			})
		}
	}

	f := harFile{
		Log: harLog{
			Entries: entries,
		},
	}
	return json.MarshalIndent(f, "", "  ")
}

func mapToNameValues(m map[string]string) []nameValue {
	if m == nil {
		return nil
	}
	nvs := make([]nameValue, 0, len(m))
	for k, v := range m {
		nvs = append(nvs, nameValue{Name: k, Value: v})
	}
	return nvs
}

func bodyToPostData(body any) *harPostData {
	if body == nil {
		return nil
	}
	text := ""
	mime := "text/plain"
	switch v := body.(type) {
	case map[string]any, []any, map[string]interface{}, []interface{}:
		if b, err := json.Marshal(v); err == nil {
			text = string(b)
			mime = "application/json"
		}
	case string:
		text = v
	default:
		if b, err := json.Marshal(v); err == nil {
			text = string(b)
			mime = "application/json"
		}
	}
	if text == "" {
		return nil
	}
	return &harPostData{
		MimeType: mime,
		Text:     text,
	}
}

func bodyToContent(body any) *harContent {
	if body == nil {
		return nil
	}
	text := ""
	mime := "text/plain"
	switch v := body.(type) {
	case map[string]any, []any, map[string]interface{}, []interface{}:
		if b, err := json.Marshal(v); err == nil {
			text = string(b)
			mime = "application/json"
		}
	case string:
		text = v
	default:
		if b, err := json.Marshal(v); err == nil {
			text = string(b)
			mime = "application/json"
		}
	}
	if text == "" {
		return nil
	}
	return &harContent{
		MimeType: mime,
		Text:     text,
	}
}
