// Package observability provides cross-cutting OpenTelemetry distributed tracing,
// Prometheus metric registration, and structured slog logging composition-root wiring
// per 06_architecture.md §10, 17_observability.md, and 21_project_structure.md §5.1.
//
// This package contains zero domain logic and is shared across all modules.
package observability
