package core

// Regenerate the protobuf Go bindings from snapshot.proto and index.proto.
// Run `go generate ./internal/core/...` after editing either .proto file.
//
// The --go_opt=paths=source_relative flag is REQUIRED: without it protoc
// creates a nested internal/core/github.com/Alei-001/drift/internal/core/
// directory and the generated raw descriptor encodes a stale go_package,
// which panics at init time (slice bounds out of range [-1:]).
//
// See AGENTS.md "Protobuf codegen" for background.
//go:generate protoc --proto_path=. --go_out=. --go_opt=paths=source_relative snapshot.proto
//go:generate protoc --proto_path=. --go_out=. --go_opt=paths=source_relative index.proto
