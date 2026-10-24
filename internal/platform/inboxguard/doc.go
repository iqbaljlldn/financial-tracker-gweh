// Package inboxguard provides shared idempotency-guard utilities for Asynq task handlers
// ensuring exactly-once processing semantics atop at-least-once message delivery
// per 06_architecture.md §6.2, 10_backend_blueprint.md §8.2, and 21_project_structure.md §5.1.
//
// This package contains zero domain logic and is shared across all modules.
package inboxguard
