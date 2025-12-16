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

### Test distributed ML training
Once the cluster is running, submit **one parent job** that automatically splits into 3 sub-jobs:
```bash
# Submit a parent job - it will automatically create 3 sub-jobs
curl -X POST http://localhost:8000/submit \
  -H "Content-Type: application/json" \
  -d '{"id":"job-1","type":"mnist_train","status":"PENDING"}'
```

**What happens automatically:**
- The FSM splits `job-1` into three sub-jobs: `job-1-node-1`, `job-1-node-2`, `job-1-node-3`
- Each sub-job is assigned to its respective `worker_id` (node-1, node-2, node-3)
- Node 1 picks up `job-1-node-1` and trains on samples 0-20,000
- Node 2 picks up `job-1-node-2` and trains on samples 20,000-40,000
- Node 3 picks up `job-1-node-3` and trains on samples 40,000-60,000
- Watch logs (`make logs`) to see parallel training across all 3 shards

Verify all sub-jobs completed and results are replicated:
```bash
curl 'http://localhost:8000/job?id=job-1-node-1'  # Check node-1's sub-job
curl 'http://localhost:8001/job?id=job-1-node-2'  # Check node-2's sub-job  
curl 'http://localhost:8002/job?id=job-1-node-3'  # Check node-3's sub-job

# All nodes have replicated state, so you can query any node for any job:
curl 'http://localhost:8000/job?id=job-1-node-2'  # Query leader for node-2's job
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

