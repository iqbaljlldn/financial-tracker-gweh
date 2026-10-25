package domain

import "errors"

// Sentinel domain errors per 20_coding_convention.md §4.2 & §6.2
var (
	ErrTemplateNotFound          = errors.New("understanding: no active template found for sender")
	ErrTemplateValidationFailed  = errors.New("understanding: template failed sample-set validation harness")
	ErrRequiredFieldMissing      = errors.New("understanding: required field failed extraction")
	ErrExtractionExecutionFailed = errors.New("understanding: extraction execution failed")
	ErrDuplicateActiveVersion    = errors.New("understanding: duplicate active version constraint violation")
)
