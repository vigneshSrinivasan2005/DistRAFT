package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/hashicorp/raft"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/consensus"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
)

func TestFSMApplySetJob(t *testing.T) {
	state := store.NewState()
	fsm := consensus.NewFSM(state)

	event := consensus.LogEvent{
		Type:  consensus.CmdSetJob,
		JobID: "job-1",
		Data:  &store.Job{ID: "job-1", Type: "mnist_train", Status: store.StatusPending, WorkerID: "worker-a"},
	}
	data, _ := json.Marshal(event)
	logEntry := &raft.Log{Data: data}

	if got := fsm.Apply(logEntry); got != nil {
		t.Fatalf("expected nil apply result, got %v", got)
	}

	job, ok := state.GetJob("job-1")
	if !ok {
		t.Fatalf("job not stored in state")
	}
	if job.Status != store.StatusPending || job.WorkerID != "worker-a" {
		t.Fatalf("job fields incorrect after apply: %+v", job)
	}
}

func TestFSMApplyUnknownCommand(t *testing.T) {
	state := store.NewState()
	fsm := consensus.NewFSM(state)

	event := consensus.LogEvent{Type: consensus.CommandType("UNKNOWN"), JobID: "job-2", Data: &store.Job{ID: "job-2"}}
	data, _ := json.Marshal(event)
	logEntry := &raft.Log{Data: data}

	if got := fsm.Apply(logEntry); got == nil {
		t.Fatalf("expected error for unknown command, got nil")
	}
}

func TestFSMApplySubmitParentJob(t *testing.T) {
	state := store.NewState()
	fsm := consensus.NewFSM(state)

	// Submit a parent job that should be split into 3 sub-jobs
	event := consensus.LogEvent{
		Type:        consensus.CmdSubmitParentJob,
		JobID:       "job-1",
		Data:        &store.Job{ID: "job-1", Type: "mnist_train", Status: store.StatusPending},
		ClusterSize: 3,
	}
	data, _ := json.Marshal(event)
	logEntry := &raft.Log{Data: data}

	if got := fsm.Apply(logEntry); got != nil {
		t.Fatalf("expected nil apply result, got %v", got)
	}

	// Verify 3 sub-jobs were created
	expectedSubJobs := []string{"job-1-node-1", "job-1-node-2", "job-1-node-3"}
	expectedWorkerIDs := []string{"node-1", "node-2", "node-3"}

	for i, subJobID := range expectedSubJobs {
		job, ok := state.GetJob(subJobID)
		if !ok {
			t.Fatalf("sub-job %s not found in state", subJobID)
		}
		if job.Type != "mnist_train" {
			t.Fatalf("sub-job %s has wrong type: %s", subJobID, job.Type)
		}
		if job.Status != store.StatusPending {
			t.Fatalf("sub-job %s has wrong status: %s", subJobID, job.Status)
		}
		if job.WorkerID != expectedWorkerIDs[i] {
			t.Fatalf("sub-job %s has wrong worker_id: expected %s, got %s",
				subJobID, expectedWorkerIDs[i], job.WorkerID)
		}
	}
}

func TestFSMSnapshotAndRestore(t *testing.T) {
	state := store.NewState()
	state.Apply("job-1", &store.Job{ID: "job-1", Type: "mnist_train", Status: store.StatusRunning, WorkerID: "worker-a"})
	fsm := consensus.NewFSM(state)

	snap, err := fsm.Snapshot()
	if err != nil {
		t.Fatalf("snapshot error: %v", err)
	}

	sink := &testSnapshotSink{id: "snap-1"}
	if err := snap.Persist(sink); err != nil {
		t.Fatalf("persist error: %v", err)
	}
	if !sink.closed {
		t.Fatalf("expected sink to be closed")
	}

	var restoredData map[string]*store.Job
	if err := json.Unmarshal(sink.buf.Bytes(), &restoredData); err != nil {
		t.Fatalf("failed to decode snapshot bytes: %v", err)
	}
	job, ok := restoredData["job-1"]
	if !ok || job.Status != store.StatusRunning {
		t.Fatalf("snapshot missing job data: %+v", job)
	}

	restoreState := store.NewState()
	restoreFSM := consensus.NewFSM(restoreState)
	reader := io.NopCloser(bytes.NewReader(sink.buf.Bytes()))
	if err := restoreFSM.Restore(reader); err != nil {
		t.Fatalf("restore error: %v", err)
	}

	restoredJob, ok := restoreState.GetJob("job-1")
	if !ok || restoredJob.WorkerID != "worker-a" {
		t.Fatalf("restored job incorrect: %+v", restoredJob)
	}
}

// testSnapshotSink captures snapshot bytes for verification.
type testSnapshotSink struct {
	buf      bytes.Buffer
	closed   bool
	canceled bool
	id       string
}

func (s *testSnapshotSink) ID() string                  { return s.id }
func (s *testSnapshotSink) Cancel() error               { s.canceled = true; return nil }
func (s *testSnapshotSink) Write(p []byte) (int, error) { return s.buf.Write(p) }
func (s *testSnapshotSink) Close() error                { s.closed = true; return nil }
