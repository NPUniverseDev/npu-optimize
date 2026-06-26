# NPU Optimize — Agent Instructions

## Project
Go CLI that detects hardware, queries HuggingFace for GGUF models,
calculates optimal inference configuration for llama.cpp.

## Build commands
go build ./cmd/npu-optimize

## Tests
go test ./... -v -count=1
go test -tags=integration ./...
golangci-lint run

## ADRs
Architecture documentation is in docs/ADR-*.md.
Read the README.md and relevant source code before making significant changes.

## Conventions
- Go 1.26+
- No CGO
- testify for test assertions
- backend.Interface for abstracting inference engines
- Versioned JSON output with $schema
