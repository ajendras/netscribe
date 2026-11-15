package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/netscribe/netscribe/internal/llm"
	"github.com/netscribe/netscribe/internal/models"
	"github.com/netscribe/netscribe/internal/parser"
	"github.com/netscribe/netscribe/internal/service"
)

type apiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, payload interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, `{"success":false,"error":"failed to encode JSON response"}`, http.StatusInternalServerError)
	}
}

// respondError is a convenience wrapper for error responses.
func respondError(w http.ResponseWriter, message string, status int) {
	respondJSON(w, apiResponse{
		Success: false,
		Error:   message,
	}, status)
}

// UploadHAR handles multipart/form-data uploads of HAR files.
// It validates the upload, parses the HAR using the dedicated parser,
// and returns structured traffic data for the frontend dashboard.
func UploadHAR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MiB limit
		respondError(w, fmt.Sprintf("failed to parse multipart form: %v", err), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		respondError(w, "missing 'file' field in form data (must be a valid .har file)", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		respondError(w, fmt.Sprintf("failed to read uploaded file: %v", err), http.StatusInternalServerError)
		return
	}

	if len(data) == 0 {
		respondError(w, "uploaded HAR file is empty", http.StatusBadRequest)
		return
	}

	traffic, err := parser.ParseHAR(data)
	if err != nil {
		respondError(w, fmt.Sprintf("failed to parse HAR: %v", err), http.StatusUnprocessableEntity)
		return
	}

	respondJSON(w, apiResponse{
		Success: true,
		Data: map[string]interface{}{
			"traffic": traffic,
			"count":   len(traffic.Endpoints),
		},
	}, http.StatusOK)
}

// Generate handles POST requests to produce client, mock, or test artifacts.
// The request body must be valid JSON matching models.GenerateRequest (including
// the full TrafficData). It returns the generated source code wrapped in JSON.
func Generate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, fmt.Sprintf("invalid request JSON: %v", err), http.StatusBadRequest)
		return
	}

	if req.Traffic == nil {
		respondError(w, "traffic data is required", http.StatusBadRequest)
		return
	}

	if req.Type == "" {
		req.Type = "client"
	}
	if !isValidType(req.Type) {
		respondError(w, "type must be one of: client, mock, test", http.StatusBadRequest)
		return
	}
	if req.Type != "mock" && req.Lang != "" && !isValidLang(req.Lang) {
		respondError(w, "lang must be one of: typescript, go, python", http.StatusBadRequest)
		return
	}
	if req.Type == "client" && req.Lang == "" {
		req.Lang = "typescript"
	}
	if req.Type == "test" && req.Lang == "" {
		req.Lang = "go"
	}

	svc := service.New(llm.NewClient())
	artifact, err := svc.Generate(req)
	if err != nil {
		respondError(w, fmt.Sprintf("generation failed: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, apiResponse{
		Success: true,
		Data:    artifact,
	}, http.StatusOK)
}

func isValidType(t string) bool {
	return t == "client" || t == "mock" || t == "test"
}

func isValidLang(l string) bool {
	return l == "typescript" || l == "go" || l == "python"
}
