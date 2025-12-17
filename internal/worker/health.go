package worker

import (
	"log"
	"time"

	"github.com/hashicorp/raft"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/consensus"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
)

const (
	// JobTimeoutSeconds - jobs running longer than this are considered stuck
	JobTimeoutSeconds = 15 // 15 seconds for testing (increase to 120+ for production)
	// MaxRetries - maximum retry attempts before marking as permanently failed
	MaxRetries = 2
	// HealthCheckInterval - how often to check for stuck jobs
	HealthCheckInterval = 5 * time.Second
)

// RunHealthMonitor periodically checks for stuck jobs and handles them
func RunHealthMonitor(state *store.State, rNode *consensus.RaftNode, clusterSize int) {
	log.Printf("🏥 HEALTH MONITOR STARTED (timeout: %ds, check interval: %v)", JobTimeoutSeconds, HealthCheckInterval)

	// Get available worker IDs (assumes node-1, node-2, node-3)
	workerIDs := []string{}
	for i := 1; i <= clusterSize; i++ {
		workerIDs = append(workerIDs, consensus.NodeIDFromIndex(i))
	}

	ticker := time.NewTicker(HealthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Only leader should perform health checks
		if rNode.Raft.State() != raft.Leader {
			continue
		}

		stuckJobs := state.GetStuckJobs(JobTimeoutSeconds)
		if len(stuckJobs) == 0 {
			continue
		}

		log.Printf("🚨 Found %d stuck job(s)", len(stuckJobs))

		for _, job := range stuckJobs {
			HandleStuckJob(rNode, job, workerIDs)
		}
	}
}

// HandleStuckJob decides whether to retry or mark as failed
func HandleStuckJob(rNode *consensus.RaftNode, job *store.Job, workerIDs []string) {
	log.Printf("⚠️ Handling stuck job: %s (worker: %s, retries: %d, running for: %ds)",
		job.ID, job.WorkerID, job.RetryCount, time.Now().Unix()-job.StartedAt)

	if job.RetryCount >= MaxRetries {
		// Exceeded retry limit - mark as permanently failed
		log.Printf("❌ Job %s exceeded retry limit (%d). Marking as FAILED.", job.ID, MaxRetries)
		job.Status = store.StatusFailed
		job.UpdatedAt = time.Now().Unix()
		applyJobUpdate(rNode, job)
		return
	}

	// Attempt reassignment to a different worker
	newWorkerID := findAlternativeWorker(job.WorkerID, workerIDs)
	if newWorkerID == "" {
		// No alternative found - mark as failed
		log.Printf("❌ No alternative worker for job %s. Marking as FAILED.", job.ID)
		job.Status = store.StatusFailed
		job.UpdatedAt = time.Now().Unix()
		applyJobUpdate(rNode, job)
		return
	}

	// Reassign to a different worker
	log.Printf("🔄 Reassigning job %s: %s -> %s (retry %d/%d)",
		job.ID, job.WorkerID, newWorkerID, job.RetryCount+1, MaxRetries)

	job.Status = store.StatusPending
	job.WorkerID = newWorkerID
	job.RetryCount++
	job.StartedAt = 0 // Reset start time
	job.UpdatedAt = time.Now().Unix()

	applyJobUpdate(rNode, job)
}

// findAlternativeWorker selects a different worker (simple round-robin for now)
func findAlternativeWorker(currentWorker string, allWorkers []string) string {
	// Find next worker in the list
	for i, w := range allWorkers {
		if w == currentWorker {
			// Return next worker (wrap around)
			nextIdx := (i + 1) % len(allWorkers)
			return allWorkers[nextIdx]
		}
	}
	// Fallback: return first worker
	if len(allWorkers) > 0 {
		return allWorkers[0]
	}
	return ""
}

// applyJobUpdate sends a job update through RAFT
func applyJobUpdate(rNode *consensus.RaftNode, job *store.Job) {
	event := consensus.LogEvent{
		Type: consensus.CmdSetJob,
		Job:  job,
	}

	data := consensus.MustMarshalEvent(event)
	future := rNode.Raft.Apply(data, 5*time.Second)
	if err := future.Error(); err != nil {
		log.Printf("❌ Failed to apply job update via RAFT: %v", err)
	}
}
