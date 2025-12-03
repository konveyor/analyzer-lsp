# Protobuf Regeneration Required

The `library.proto` file has been updated to:
1. Rename `PrepareProgressEvent` → `ProgressEvent`
2. Add `ProgressEventType` enum for future extensibility

## Action Required

The generated Go files (`library.pb.go` and `library_grpc.pb.go`) need to be regenerated using:

```bash
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       provider/internal/grpc/library.proto
```

This requires:
- `protoc` (Protocol Buffer Compiler) - see installation instructions in `README.md`
- `protoc-gen-go` plugin
- `protoc-gen-go-grpc` plugin

After regeneration, this file can be deleted.

## Changes Made to .proto

1. Added `ProgressEventType` enum with `PREPARE = 0` (future-proof for other event types)
2. Renamed `message PrepareProgressEvent` to `message ProgressEvent`
3. Added `ProgressEventType type = 1;` field to the ProgressEvent message
4. Renumbered existing fields to accommodate the new type field
5. Updated RPC `StreamPrepareProgress` to return `stream ProgressEvent`

## Background

This change addresses Shawn's feedback to:
- Standardize naming (ProgressEvent instead of PrepareProgressEvent)
- Add type enum for future progress event types (EVALUATE, DEPENDENCY_ANALYSIS, etc.)
