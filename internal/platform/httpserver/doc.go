// Package httpserver provides chi router setup, server lifecycle management,
// and shared HTTP middleware (panic recovery, request logging, tracing, security headers)
// per 06_architecture.md §11, 10_backend_blueprint.md §8, and 21_project_structure.md §5.1.
//
// This package contains zero domain logic and is shared across all modules.
package httpserver
