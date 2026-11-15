package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/netscribe/netscribe/internal/handlers"
	"github.com/netscribe/netscribe/internal/llm"
	"github.com/netscribe/netscribe/internal/models"
	"github.com/netscribe/netscribe/internal/parser"
	"github.com/netscribe/netscribe/internal/server"
	"github.com/netscribe/netscribe/internal/service"
	"github.com/spf13/cobra"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "netscribe",
		Short: "NetScribe - Turns network traffic into typed API clients, mocks, and tests using local LLMs",
		Long: `NetScribe is a local-first tool that converts HAR files into production-ready
API clients, dynamic mocks, and tests. All inference runs on your machine via Ollama.

Privacy-first. Zero telemetry.`,
		Version: "0.1.0",
	}

	var (
		port       int
		ollamaHost string
		file       string
		artifactType string
		lang       string
		out        string
	)

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the web dashboard",
		Run: func(cmd *cobra.Command, args []string) {
			if ollamaHost != "" {
				if err := os.Setenv("OLLAMA_HOST", ollamaHost); err != nil {
					log.Printf("warning: failed to set OLLAMA_HOST: %v", err)
				}
			}
			srv := server.New(port)
			log.Printf("NetScribe server listening on http://localhost:%d", port)
			log.Printf("Ollama host: %s", ollamaHost)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server failed: %v", err)
			}
		},
	}

	serveCmd.Flags().IntVarP(&port, "port", "p", 8080, "port for the web server (default 8080)")
	serveCmd.Flags().StringVar(&ollamaHost, "ollama-host", "http://localhost:11434", "Ollama API endpoint")

	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate client, mock, or test artifacts from a HAR file",
		Long: `Generate artifacts directly from the CLI.
Example: netscribe generate --file api.har --type client --lang go --out client.go`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("--file is required: provide a path to a valid HAR file")
			}
			if artifactType != "client" && artifactType != "mock" && artifactType != "test" {
				return fmt.Errorf("--type must be one of: client, mock, test (got %s)", artifactType)
			}
			if lang != "" {
				switch lang {
				case "typescript", "go", "python":
					// valid
				default:
					return fmt.Errorf("--lang must be one of: typescript, go, python (got %s)", lang)
				}
			}

			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("failed to read HAR file %s: %w", file, err)
			}

			traffic, err := parser.ParseHAR(data)
			if err != nil {
				return fmt.Errorf("failed to parse HAR: %w", err)
			}

			llmClient := llm.NewClient()
			svc := service.New(llmClient)

			req := models.GenerateRequest{
				Traffic: traffic,
				Type:    artifactType,
				Lang:    lang,
			}

			artifact, err := svc.Generate(req)
			if err != nil {
				return fmt.Errorf("generation failed: %w", err)
			}

			output := artifact.Content
			if out == "" || out == "-" {
				fmt.Println(output)
				return nil
			}

			if err := os.WriteFile(out, []byte(output), 0644); err != nil {
				return fmt.Errorf("failed to write output to %s: %w", out, err)
			}

			fmt.Printf("Successfully wrote %s artifact (%d bytes) to %s\n", artifactType, len(output), out)
			return nil
		},
	}

	generateCmd.Flags().StringVarP(&file, "file", "f", "", "Path to input HAR file (required)")
	generateCmd.Flags().StringVar(&artifactType, "type", "client", "Artifact type: client, mock, test")
	generateCmd.Flags().StringVar(&lang, "lang", "typescript", "Output language: typescript, go, python")
	generateCmd.Flags().StringVarP(&out, "out", "o", "", "Output file path (default: stdout)")

	rootCmd.AddCommand(serveCmd, generateCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
