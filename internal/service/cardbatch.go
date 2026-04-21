package service

// CardBatchService lives in the cardbatch subpackage now
// (internal/service/cardbatch) so it can depend on internal/service/cardsheet
// without creating an import cycle: cardsheet imports service, and cardbatch
// needs both, so cardbatch sits below service in the import graph.
//
// This file is intentionally a placeholder — the implementation moved to
// internal/service/cardbatch/cardbatch.go.
