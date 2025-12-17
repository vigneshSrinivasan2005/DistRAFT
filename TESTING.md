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

### Federated Averaging (Phase 3)
When all three shard jobs complete, the aggregator automatically merges their models:

```bash
# Submit a parent job
curl -X POST http://localhost:8000/submit \
  -H "Content-Type: application/json" \
  -d '{"id":"fed-demo","type":"mnist_train","status":"PENDING"}'

# Tail logs to see aggregator messages
make logs

# After completion, verify global model exists
ls -lh raft-data/fed-demo_global.pth
```

Notes:
- Aggregator runs on all nodes; merging is harmlessly redundant for a prototype.
- Global model path format: `raft-data/<parent>_global.pth`.

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

## Integration Tests

### Automated Test Scripts

#### 1. Full Federated Learning Test
Automated end-to-end test that verifies the complete pipeline:
```bash
./test_federated_learning.sh
```

This test:
1. Builds the project
2. Starts a 3-node cluster
3. Submits a parent job that splits into 3 sub-jobs
4. Monitors completion of all sub-jobs
5. Verifies merged global model is created
6. **Validates model weights are correctly averaged** (mathematical verification)
7. Checks aggregator logs

Expected output: `✅ ALL TESTS PASSED` with detailed verification steps.

#### 2. Model Weight Verification
Standalone script to verify merged model correctness:
```bash
python3 ml-code/verify_merge.py
```

Or with custom paths:
```bash
python3 ml-code/verify_merge.py <global_model.pth> <shard1.pth> <shard2.pth> <shard3.pth>
```

This script:
- Loads the global model and all shard models
- Verifies weights are non-zero
- **Mathematically verifies averaging**: Computes expected average from shards and compares with global model
- Shows sample weights from first layer for inspection

Expected output: `✅ VERIFICATION PASSED`

#### 3. Chaos Test - Health Monitor & Job Reassignment
Tests automatic recovery from node failures with health monitoring:
```bash
./test_chaos.sh
```

This test:
1. Starts a 3-node cluster
2. Submits a parent job with longer training (5 epochs)
3. **Kills node-2 during training** (after 3 seconds)
4. **Waits for health monitor to detect stuck job** (15s timeout)
5. Verifies job reassignment to surviving nodes
6. Checks if all jobs complete and global model is created

Expected findings:
- ✓ System continues operating after node failure
- ✓ Surviving nodes complete their jobs
- ✓ Health monitor detects stuck job after 15s
- ✓ Job automatically reassigned from node-2 to node-3
- ✓ All jobs complete successfully after reassignment
- ✓ Global model merged from all 3 shards
- ✓ System fully recovers without manual intervention

**Resilience features verified:**
- Job timeout detection (15s threshold, configurable)
- Health monitoring (5s check interval)
- Automatic job reassignment (up to 2 retries)
- Shard count validation before merging

### Quick Test Summary
```bash
# Full pipeline verification (recommended)
./test_federated_learning.sh

# Just verify model weights
python3 ml-code/verify_merge.py

# Test automatic job recovery and reassignment (chaos test)
./test_chaos.sh

# Manual cleanup if tests fail
pkill -f "./raft-node"; rm -rf raft-data /tmp/node*.log
```

### Configuration

Health monitor settings in `internal/worker/health.go`:
- `JobTimeoutSeconds = 15` - Jobs running longer than this are marked as stuck (increase to 120+ for production)
- `MaxRetries = 2` - Maximum retry attempts before permanent failure
- `HealthCheckInterval = 5s` - How often to check for stuck jobs

