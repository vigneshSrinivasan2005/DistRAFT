# Distributed Training Example

This example demonstrates how the FSM automatically splits a single parent job into 3 sub-jobs for distributed training.

## How It Works

### Before (Manual Submission)
Previously, you had to manually submit 3 separate jobs:
```bash
curl -X POST http://localhost:8000/submit -H "Content-Type: application/json" \
  -d '{"id":"job-1-node-1","type":"mnist_train","status":"PENDING","worker_id":"node-1"}'
  
curl -X POST http://localhost:8000/submit -H "Content-Type: application/json" \
  -d '{"id":"job-1-node-2","type":"mnist_train","status":"PENDING","worker_id":"node-2"}'
  
curl -X POST http://localhost:8000/submit -H "Content-Type: application/json" \
  -d '{"id":"job-1-node-3","type":"mnist_train","status":"PENDING","worker_id":"node-3"}'
```

### After (Automatic Splitting)
Now you submit **one parent job** and the FSM handles the rest:
```bash
curl -X POST http://localhost:8000/submit -H "Content-Type: application/json" \
  -d '{"id":"job-1","type":"mnist_train","status":"PENDING"}'
```

**Response:**
```
Parent job job-1 split into 3 sub-jobs successfully
```

### What Happens Behind the Scenes

1. **Leader receives parent job** with ID `job-1`
2. **FSM applies `SUBMIT_PARENT_JOB` command** which:
   - Creates `job-1-node-1` with `worker_id: "node-1"`
   - Creates `job-1-node-2` with `worker_id: "node-2"`
   - Creates `job-1-node-3` with `worker_id: "node-3"`
3. **Each sub-job is replicated** via Raft to all nodes
4. **Each worker picks up its assigned job**:
   - Node-1 finds `job-1-node-1` (WorkerID matches)
   - Node-2 finds `job-1-node-2` (WorkerID matches)
   - Node-3 finds `job-1-node-3` (WorkerID matches)
5. **Python training starts in parallel** on all 3 nodes:
   - Node-1: `train.py job-1-node-1 --shard_index node-1 --total_shards 3` (samples 0-20K)
   - Node-2: `train.py job-1-node-2 --shard_index node-2 --total_shards 3` (samples 20K-40K)
   - Node-3: `train.py job-1-node-3 --shard_index node-3 --total_shards 3` (samples 40K-60K)

## Verify Sub-Jobs Were Created

Query any node (state is replicated) to see all 3 sub-jobs:
```bash
# Query leader
curl 'http://localhost:8000/job?id=job-1-node-1'
curl 'http://localhost:8000/job?id=job-1-node-2'
curl 'http://localhost:8000/job?id=job-1-node-3'

# Or query followers (same result)
curl 'http://localhost:8001/job?id=job-1-node-1'
curl 'http://localhost:8002/job?id=job-1-node-1'
```

## Watch Training Progress

Use `make logs` to tail all node logs and watch parallel training:
```bash
make logs
```

You'll see output like:
```
==> raft-data/node-1/server.log <==
2025/12/16 10:30:15 Worker node-1 picked up job: job-1-node-1
2025/12/16 10:30:15 Running: python3 ml-code/train.py job-1-node-1 --shard_index node-1 --total_shards 3

==> raft-data/node-2/server.log <==
2025/12/16 10:30:16 Worker node-2 picked up job: job-1-node-2
2025/12/16 10:30:16 Running: python3 ml-code/train.py job-1-node-2 --shard_index node-2 --total_shards 3

==> raft-data/node-3/server.log <==
2025/12/16 10:30:17 Worker node-3 picked up job: job-1-node-3
2025/12/16 10:30:17 Running: python3 ml-code/train.py job-1-node-3 --shard_index node-3 --total_shards 3
```

## Implementation Details

### FSM Code (`internal/consensus/fsm.go`)
```go
case CmdSubmitParentJob:
    // Split parent job into sub-jobs for each node
    for i := 1; i <= event.ClusterSize; i++ {
        nodeID := fmt.Sprintf("node-%d", i)
        subJobID := fmt.Sprintf("%s-%s", event.JobID, nodeID)
        subJob := &store.Job{
            ID:        subJobID,
            Type:      event.Data.Type,
            Status:    store.StatusPending,
            WorkerID:  nodeID,
            ResultURL: "",
        }
        f.state.Apply(subJobID, subJob)
    }
```

### Worker Filtering (`internal/worker/runner.go`)
```go
// Only pick up jobs assigned to this worker
if job.Status == store.StatusPending && job.WorkerID == nodeID {
    log.Printf("Worker %s picked up job: %s", nodeID, jobID)
    // Execute training...
}
```

## Benefits

✅ **Simpler API**: Submit one job instead of three  
✅ **Consistent naming**: Automatic `{parent-id}-{node-id}` format  
✅ **Atomic operation**: All sub-jobs created in one Raft log entry  
✅ **Fault tolerant**: If leader crashes during split, new leader has full state  
✅ **Easy to extend**: Change `clusterSize` to scale to more nodes
