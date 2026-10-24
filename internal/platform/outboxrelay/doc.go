// Package outboxrelay provides shared, generic transactional Outbox Relay polling logic
// instantiated per-module via each module's adapter/outbox/relay.go
// per 06_architecture.md §6.1, 10_backend_blueprint.md §4, and 21_project_structure.md §5.1.
//
// This package contains zero domain logic and is shared across all modules.
package outboxrelay
