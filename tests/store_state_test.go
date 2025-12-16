package tests

import (
	"testing"

	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
)

func TestStateApplyAndGet(t *testing.T) {
	state := store.NewState()
	job := &store.Job{ID: "job-1", Type: "mnist_train", Status: store.StatusPending, WorkerID: "worker-a", ResultURL: ""}

	state.Apply(job.ID, job)

	got, ok := state.GetJob(job.ID)
	if !ok {
		t.Fatalf("expected job to exist")
	}
	if got != job {
		t.Fatalf("expected stored job pointer to match original")
	}
}

func TestStateMarshalUnmarshal(t *testing.T) {
	state := store.NewState()
	state.Apply("job-1", &store.Job{ID: "job-1", Type: "mnist_train", Status: store.StatusRunning, WorkerID: "worker-a", ResultURL: "http://result/1"})
	state.Apply("job-2", &store.Job{ID: "job-2", Type: "mnist_eval", Status: store.StatusCompleted, WorkerID: "worker-b", ResultURL: "http://result/2"})

	data, err := state.Marshal()
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	restored := store.NewState()
	if err := restored.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	job, ok := restored.GetJob("job-1")
	if !ok || job.Status != store.StatusRunning || job.WorkerID != "worker-a" {
		t.Fatalf("job-1 did not round-trip correctly: %+v", job)
	}

	job, ok = restored.GetJob("job-2")
	if !ok || job.Status != store.StatusCompleted || job.ResultURL != "http://result/2" {
		t.Fatalf("job-2 did not round-trip correctly: %+v", job)
	}
}
