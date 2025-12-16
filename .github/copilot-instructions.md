# Copilot Project Instructions

- **Purpose**: Distributed ML training orchestrated by RAFT; each peer runs a RAFT node plus a worker that executes ML tasks (see `Implementation.md`).
- **Planned flow**: RAFT log coordinates which job each peer runs; leader streams model shards to workers; workers stream gradients back; Python jobs run alongside the Go agent via localhost gRPC.
- **Code layout**: Proto contract in `internal/api/ml_service.proto` with generated stubs `ml_service.pb.go` and `ml_service_grpc.pb.go`. Placeholder dirs `internal/consensus`, `internal/store`, `internal/worker`, `cmd/server`, `ml-code/` are currently empty and ready for implementation.
- **gRPC contract**: Service `MLWorkerService` has server-streaming `GetModel(ModelRequest)->ModelChunk` (chunked model download) and client-streaming `SendGradients(stream GradientChunk)->Ack` (upload gradients per job/worker). Messages: `ModelChunk{chunk_id,data}`, `GradientChunk{job_id,worker_id,data}`, `Ack{success}`.
- **Package naming**: Update the proto `go_package` to the module path `github.com/vigneshSrinivasan2005/DistRAFT/internal/api` before regenerating to avoid import mismatches (current value is `github.com/yourusername/raft-ml-grid/internal/api`).
- **Regenerate protobufs**: From repo root run `protoc --go_out=. --go-grpc_out=. internal/api/ml_service.proto` (requires `protoc`, `protoc-gen-go`, `protoc-gen-go-grpc` on PATH). Do not edit generated `.pb.go` files by hand.
- **Dependencies**: Go module `github.com/vigneshSrinivasan2005/DistRAFT` targeting Go 1.25.5; uses HashiCorp RAFT and raft-boltdb for consensus/storage plus gRPC/protobuf for RPCs.
- **Expected components**: `internal/consensus` to wrap hashicorp/raft node (FSM, log apply); `internal/store` likely for boltdb-backed stable store/snapshots; `internal/worker` for executing ML jobs and handling gRPC streaming; `cmd/server` for wiring node startup/peer join/worker loop.
- **Concurrency model**: Each peer should run RAFT heartbeats plus worker loop concurrently (per design doc). Keep RAFT apply path synchronous with log entries; perform heavy ML work off the RAFT apply thread.
- **State propagation**: Use RAFT log entries to distribute job assignments and any state that must be replicated. Use gRPC streaming only for model/gradient payloads; keep control-plane decisions consistent via RAFT.
- **Testing goals**: Unit-test FSM/apply logic (state updates on log apply). Integration: multi-process 3-node cluster, kill leader, ensure re-election and continued writes/reads. ML flow: submit job, observe gradient/model streaming across nodes.
- **Build/run hints**: After adding code, default to `go test ./...` and `go build ./...`. Expect to create a `main` in `cmd/server` to run a node (args for peer join addresses, raft/data dirs, ports).
- **Observability**: Dependencies include `github.com/armon/go-metrics` and `github.com/fatih/color`; prefer consistent logging via `go-hclog` (already in deps) and expose node metrics if added.
- **Python side**: `ml-code/` is reserved for training scripts; workers should call back to Go via gRPC over localhost. Keep payloads chunked (`ModelChunk`, `GradientChunk`) to handle large models.
- **Development tips**: Stick to `internal/` packages for server code; keep protobuf types in `internal/api`; avoid circular deps between consensus/store/worker; keep streaming handlers cancel-aware (ctx deadlines) and return `codes.Unimplemented` until implemented.
- **Additions checklist**: When introducing new RPCs or log entry types, update the proto and regen stubs; thread new state through RAFT FSM; add small doc notes to `Implementation.md` or inline comments for non-obvious decisions.

If anything here is unclear or missing for your workflow, let me know and I will refine it.
