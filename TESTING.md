# Testing & Running

Shared unit tests live in `tests/` and target the exported APIs of `internal/` packages.

## Unit Tests

### Quick commands
- `make test` — run the shared test suite in `tests/` (fast loop).
- `make test-all` — run `go test ./...` across the module (includes any future package-local tests).

### Prerequisites
- Go toolchain installed (module targets Go 1.25.5 per `go.mod`).

### Notes
- Tests use temporary directories and ephemeral TCP ports; they leave no artifacts.
- If you regenerate protobufs or change Raft wiring, rerun `make test-all` to catch regressions.

## Local Cluster

### Spin up a fully-connected 3-node cluster
```bash
make cluster
```

This will automatically:
1. Build the server binary (`raft-node`)
2. Clean up any previous state (kill old nodes, remove `raft-data/`)
3. Start 3 nodes in background with logs in `/tmp/node*.log`:
   - **Node 1**: Bootstrap leader, Raft on `localhost:7000`, HTTP API on `localhost:8000`
   - **Node 2**: Follower, Raft on `localhost:7001`, HTTP API on `localhost:8001`
   - **Node 3**: Follower, Raft on `localhost:7002`, HTTP API on `localhost:8002`
4. **Automatically join nodes 2 and 3 to the cluster** (no manual curl needed)

Expect output like:
```
--- ✅ Cluster Ready! ---
Logs: tail -f /tmp/node*.log
API:  curl -X POST localhost:8000/submit -d '{"id": "job-1", "status": "PENDING"}'
```

### Test replication
Once the cluster is running, submit a job to node 1 (leader):
```bash
curl -X POST http://localhost:8000/submit \
  -H "Content-Type: application/json" \
  -d '{"id":"job-1","type":"mnist_train","status":"PENDING","worker_id":"","result_url":""}'
```

Verify the job immediately propagated to followers:
```bash
curl 'http://localhost:8000/job?id=job-1'  # Leader
curl 'http://localhost:8001/job?id=job-1'  # Follower 2 - should return same data
curl 'http://localhost:8002/job?id=job-1'  # Follower 3 - should return same data
```

### View logs
Watch all 3 nodes at once:
```bash
make logs
```

Or individually:
```bash
tail -f /tmp/node1.log
tail -f /tmp/node2.log
tail -f /tmp/node3.log
```

### Stop and clean up
```bash
make clean
```

This stops all nodes and removes the `raft-data/` directory.

