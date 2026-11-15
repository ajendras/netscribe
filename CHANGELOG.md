# Changelog

All notable changes to this project are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]
### Fixed

- add go.mod/go.sum for all external deps; fix unused import, middleware logger, unexported test helper, and missing http import

## [0.1.0] - 2026-11-15

### Added

- HAR Ingestion — Drag-and-drop or upload HAR files for instant structured parsing of requests and responses.
- Local LLM — Queries Ollama to intelligently name endpoints, infer types, and enrich generated code.
- Multi-Language — Outputs TypeScript (fetch/axios), Go, and Python clients from the same traffic.
- Test Generation — Produces ready-to-run test suites with real-data assertions and common edge cases.
- Dynamic Mocks — Creates mock servers and fixtures that vary responses based on observed patterns.
- Neon UI — Synthwave-styled responsive dashboard with live previews and one-click copy.
- CLI + Web — Use via web UI or headless CLI for CI and scriptable generation workflows.
- Privacy First — Zero telemetry; all inference and generation happens on your machine.
