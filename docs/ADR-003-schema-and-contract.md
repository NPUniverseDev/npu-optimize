# ADR-003: Output Schema and Integration Contract

**Date:** 2026-06-14
**Status:** Obsoleto — el contrato vivo está en README.md, schema.go y docs/schemas/
**Repo:** `github.com/Ericson246/npu-optimize`

## Context

`npu-optimize` produces an inference configuration recommendation that can be consumed by any external tool: scripts, launchers, inference servers, GUIs, assistants, etc.

For predictable and robust integration, a **formal contract** is defined covering:

1. Output format (JSON schema)
2. Output channels (stdout vs stderr)
3. Exit codes
4. Schema versioning

This contract is **consumer-agnostic**: it doesn't assume what tool consumes the output, what programming language is used, or what is done with the result.

## Decision

### Output Channels

| Channel | Content | Consumer usage |
|:--------|:--------|:---------------|
| **stdout** | Success JSON only (exit 0 or 2). Schema defined in ADR-001 | Parse to get the recommendation |
| **stderr** | Human logs (controlled by `--verbose`) + **ErrorOutput JSON** on failure (exit 1, 3, 4) | Show to user or parse `ErrorOutput` JSON line for programmatic handling |

This allows: `npu-optimize detect > output.json` (stderr goes to terminal) or `npu-optimize detect > output.json 2>error.log`
without JSON being polluted by log messages.

**ErrorOutput JSON** is written as a single JSON line to stderr, distinguishable from human logs by starting with `{`. Consumers can scan stderr for a line matching the error schema to handle failures programmatically.

### Exit Codes

| Code | Meaning | What the consumer should do |
|:----:|:--------|:---------------------------|
| `0` | Successful execution, `viable: true` | Read stdout, extract `inference_params`, proceed |
| `1` | Internal error (bug, network, IO, timeout) | Show stderr to user, don't retry without changes |
| `2` | Successful execution, `viable: false` | stdout contains JSON with `viable: false` and `fallbacks[]`. Consumer can choose a fallback or inform the user |
| `3` | Unsupported hardware | Inform the user their hardware doesn't meet minimum requirements |
| `4` | Authentication required (gated model, HF token not configured) | Ask user for token and retry |

Rules:
- Code `0` or `2` → stdout **always** contains valid JSON matching the main schema. Nothing is written to stderr except optional logs.
- Code `1`, `3` or `4` → stdout is **empty**. The error information is written to **stderr** as:
  1. A human-readable log line via `slog.Error` (e.g. `time=... level=ERROR msg="..."`)
  2. A JSON `ErrorOutput` line via `output.EncodeError` (single-line JSON starting with `{`)
- The consumer **must** inspect the exit code before parsing stdout. If code != 0 and code != 2, read stderr for the error.
- Programmatic consumers can scan stderr for a JSON line matching the error schema (`"error": true`).

#### Error Schema (exit codes 1, 3, 4)

```jsonc
{
  "$schema": "https://Ericson246.github.io/npu-optimize/schemas/error-v1.json",
  "version": 1,
  "error": true,
  "error_code": 1,
  "error_type": "internal_error",
  "message": "Could not connect to HuggingFace API: timeout after 30s",
  "details": {
    "endpoint": "GET /api/models",
    "status_code": 0,
    "retry_after": null
  }
}
```

| Field | Type | Required | Description |
|:------|:-----|:---------|:------------|
| `$schema` | string | Yes | Error schema URL |
| `version` | int | Yes | Error schema version (current: 1) |
| `error` | bool | Yes | Always `true` (distinguishable from main schema) |
| `error_code` | int | Yes | Exit code (1, 3, or 4) |
| `error_type` | string | Yes | Machine-readable type: `internal_error`, `hardware_unsupported`, `auth_required` |
| `message` | string | Yes | Human-readable message |
| `details` | object | No | Additional context data (endpoint, status_code, etc.) |

error_type ↔ exit code mapping:

| exit_code | error_type | When it occurs |
|:---------:|:-----------|:---------------|
| 1 | `internal_error` | Bug, network issue, IO, timeout, unsupported schema version |
| 3 | `hardware_unsupported` | No GPU detected, insufficient VRAM |
| 4 | `auth_required` | Gated model without token, invalid/expired HF token |

### Output Contract (JSON Schema)

The schema is defined in ADR-001 (section "Output Contract") and versioned independently of the tool. The `$schema` field in the JSON indicates the exact version used:

```jsonc
{
  "$schema": "https://Ericson246.github.io/npu-optimize/schemas/v1.json",
  "version": 1,
  "backend": "llama.cpp",
  "viable": true,
  "inference_params": { /* ... */ },
  "backend_params": { /* ... */ },
  "recommended": { /* ... */ },
  "hardware": { /* ... */ }
}
```

Main fields for any consumer:

| Field | Purpose |
|:------|:--------|
| `backend` | Recommended inference engine (`llama.cpp`, `vllm`, etc.) |
| `inference_params` | Generic parameters (applicable to any backend) |
| `backend_params` | Engine-specific parameters |
| `recommended.repo` + `recommended.file` | Model to download/use |
| `viable` | Whether a configuration meeting `--min-ts` exists |
| `fallbacks[]` | Alternative viable configurations ordered by quality |
| `hardware` | System metadata |

### Schema Versioning

- The schema is versioned with `semver` (v1, v2, ...) independently of the `npu-optimize` version
- The consumer negotiates the version via `--output-schema-version` (persistent flag)
- Breaking changes (removing fields, changing types) → major version
- Additions (new optional fields) → minor version
- The `$schema` URL always points to the exact version

#### Tool ↔ Schema Compatibility Matrix

| `npu-optimize` | Default output schema | Supported schemas | Schema additions |
|:---------------|:---------------------|:------------------|:-----------------|
| v0.1.0 | v1 | v1 | Initial schema: `detect`, manual VRAM formula |
| v0.2.0 | v2 | v1, v2 | Adds `llama_bench`, `proxy_benchmark`, `extrapolation_method` |
| v0.3.0 | v2 | v1, v2 | Adds `benchmark_results`, `sweep_best`, `sweep_elapsed_seconds` |
| v1.0.0+ | v2+ | v1+ | Stable contract. Breaking changes require major schema version |

`--output-schema-version` flag behavior:
- If the consumer requests a version the tool supports → output in that version
- If they request a **higher** version than the tool produces → the highest available version (best-effort)
- If they request a version the tool **no longer supports** (removed in a major version) → exit code 1 with `error_type: schema_version_unsupported` and stderr indicating available versions
- The `version` field in the JSON output **always** reflects the actual version produced, regardless of what was requested

### Schema Hosting

While obtaining a custom domain, schemas are published on GitHub Pages:

```
https://Ericson246.github.io/npu-optimize/schemas/
  ├── v1.json          # Main schema v1 (v0.1.0)
  ├── v2.json          # Main schema v2 (v0.2.0+)
  └── error-v1.json    # Error schema (all versions)
```

Generated automatically in CI when a release is tagged. Served statically without authentication. Post-v1.0.0, migrating to a custom domain with CDN will be evaluated.

### Example Consumption (non-binding)

This shows a hypothetical consumer; each tool implements its own logic:

```
1. Run: npu-optimize detect --mode auto > output.json 2>error.log
2. Read exit code
3. If exit code == 0:
      parse stdout (output.json) → extract inference_params + recommended
      build inference command: llama-server <params> -m <model>
4. If exit code == 2:
      parse stdout (output.json) → read fallbacks[]
      choose fallback or inform user
5. If exit code == 1, 3, 4:
      stdout is empty
      read stderr (error.log) for:
        - human message (slog line)
        - ErrorOutput JSON line (for programmatic handling)
      act based on exit code (retry, ask for token, abort)
```

### Contract Stability

- **v0.x of the tool**: Schema may change without notice (stabilized in v1.0.0)
- **v1.0.0+**: Breaking changes require a major schema version
- The `$schema` field in the JSON allows the consumer to validate they understand the format

## Alternatives Considered

| Alternative | Discarded because |
|:------------|:------------------|
| gRPC / protocol buffers | Overkill for a unidirectional relationship. JSON + stdin/stdout is universal, debuggable, and dependency-free |
| Socket / named pipe | Unnecessary complexity. stdout is the simplest mechanism and works on all platforms |
| DLL / shared library | Language coupling (C ABI). The consumer could be in Python, Rust, TypeScript, etc. |
| Output only on stderr | JSON would be mixed with logs; the consumer couldn't easily separate them |

## References

- ADR-001: Full output schema (`$schema`, `inference_params`, `backend_params`, `fallbacks`)
- ADR-002: Benchmark, Extrapolation and Optimization (generation of schema values)
