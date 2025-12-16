package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/worker"
)

// TestReportSuccess verifies that the worker correctly reports training results to the leader.
func TestReportSuccess(t *testing.T) {
	// Setup a mock leader HTTP endpoint
	jobCompleted := false
	receivedPayload := map[string]interface{}{}

	mockLeader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/submit" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		receivedPayload = payload
		jobCompleted = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer mockLeader.Close()

	// Extract port from mock server
	// mockLeader.URL looks like "http://127.0.0.1:PORT"
	// We need to extract just ":PORT"
	mockURL := mockLeader.URL
	colonIdx := len("http://127.0.0.1") // Position of the colon before port
	mockAddr := mockURL[colonIdx:]

	result := &worker.PythonResult{
		JobID:     "test-job-123",
		Status:    "COMPLETED",
		Accuracy:  95.5,
		Loss:      0.15,
		ModelPath: "/tmp/test-model.pth",
	}

	// Report success
	err := worker.ReportSuccess(mockAddr, result)
	if err != nil {
		t.Fatalf("ReportSuccess failed: %v", err)
	}

	// Verify the payload was sent correctly
	if !jobCompleted {
		t.Fatalf("expected leader endpoint to be called")
	}
	if receivedPayload["id"] != result.JobID {
		t.Errorf("expected id=%s, got %v", result.JobID, receivedPayload["id"])
	}
	if receivedPayload["status"] != "COMPLETED" {
		t.Errorf("expected status=COMPLETED, got %v", receivedPayload["status"])
	}
	if receivedPayload["result_url"] != result.ModelPath {
		t.Errorf("expected result_url=%s, got %v", result.ModelPath, receivedPayload["result_url"])
	}
}

// TestPythonResultMarshaling verifies JSON serialization/deserialization.
func TestPythonResultMarshaling(t *testing.T) {
	original := &worker.PythonResult{
		JobID:     "test-job-001",
		Status:    "COMPLETED",
		Accuracy:  92.5,
		Loss:      0.25,
		ModelPath: "/path/to/model.pth",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal back
	restored := &worker.PythonResult{}
	if err := json.Unmarshal(jsonData, restored); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Verify fields match
	if restored.JobID != original.JobID {
		t.Errorf("JobID mismatch: %s != %s", restored.JobID, original.JobID)
	}
	if restored.Status != original.Status {
		t.Errorf("Status mismatch: %s != %s", restored.Status, original.Status)
	}
	if restored.Accuracy != original.Accuracy {
		t.Errorf("Accuracy mismatch: %f != %f", restored.Accuracy, original.Accuracy)
	}
	if restored.Loss != original.Loss {
		t.Errorf("Loss mismatch: %f != %f", restored.Loss, original.Loss)
	}
	if restored.ModelPath != original.ModelPath {
		t.Errorf("ModelPath mismatch: %s != %s", restored.ModelPath, original.ModelPath)
	}
}

// TestWorkerLoopWithMockLeader simulates a complete worker loop: find job -> run -> report.
func TestWorkerLoopWithMockLeader(t *testing.T) {
	// Setup mock leader
	jobsReceived := []map[string]interface{}{}

	mockLeader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/submit" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		jobsReceived = append(jobsReceived, payload)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer mockLeader.Close()

	// Extract port (mockLeader.URL looks like "http://127.0.0.1:PORT")
	// We need to extract just ":PORT"
	mockURL := mockLeader.URL
	colonIdx := len("http://127.0.0.1") // Position of the colon before port
	mockAddr := mockURL[colonIdx:]

	// Setup test state with a pending job
	state := store.NewState()
	testJobID := "integration-test-" + fmt.Sprintf("%d", time.Now().Unix())
	state.Apply(testJobID, &store.Job{
		ID:       testJobID,
		Type:     "mnist_train",
		Status:   store.StatusPending,
		WorkerID: "test-worker",
	})

	// Verify job is in pending state
	job, ok := state.GetJob(testJobID)
	if !ok || job.Status != store.StatusPending {
		t.Fatalf("test job not found or not pending")
	}

	// Simulate one worker iteration (find and run pending job)
	var jobToRun *store.Job
	state.RLock()
	for _, j := range state.Jobs {
		if j.Status == store.StatusPending {
			jobToRun = j
			break
		}
	}
	state.RUnlock()

	if jobToRun == nil {
		t.Fatalf("expected to find pending job")
	}

	// For this test, we'll manually construct a result instead of actually running Python
	// (to avoid dependency on Python in Go tests)
	mockResult := &worker.PythonResult{
		JobID:     jobToRun.ID,
		Status:    "COMPLETED",
		Accuracy:  89.45,
		Loss:      0.3478,
		ModelPath: filepath.Join("raft-data", jobToRun.ID+"_model.pth"),
	}

	// Report the result
	err := worker.ReportSuccess(mockAddr, mockResult)
	if err != nil {
		t.Fatalf("ReportSuccess failed: %v", err)
	}

	// Verify the result was reported
	if len(jobsReceived) != 1 {
		t.Fatalf("expected 1 job report, got %d", len(jobsReceived))
	}

	reported := jobsReceived[0]
	if reported["id"] != testJobID {
		t.Errorf("expected reported job id=%s, got %v", testJobID, reported["id"])
	}
	if reported["status"] != "COMPLETED" {
		t.Errorf("expected reported status=COMPLETED, got %v", reported["status"])
	}
}

// TestWorkerIntegrationWithState tests the worker loop reading from a shared state.
func TestWorkerIntegrationWithState(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test state with multiple jobs
	state := store.NewState()
	jobIDs := []string{
		"job-pending-1",
		"job-running-1",
		"job-completed-1",
	}

	for i, id := range jobIDs {
		statuses := []store.JobStatus{
			store.StatusPending,
			store.StatusRunning,
			store.StatusCompleted,
		}
		state.Apply(id, &store.Job{
			ID:        id,
			Type:      "mnist_train",
			Status:    statuses[i],
			WorkerID:  "worker-1",
			ResultURL: filepath.Join(tempDir, id+"_model.pth"),
		})
	}

	// Verify state contains all jobs
	if len(state.Jobs) != len(jobIDs) {
		t.Fatalf("expected %d jobs, got %d", len(jobIDs), len(state.Jobs))
	}

	// Find pending jobs (simulating worker loop logic)
	state.RLock()
	pendingJobs := 0
	for _, job := range state.Jobs {
		if job.Status == store.StatusPending {
			pendingJobs++
		}
	}
	state.RUnlock()

	if pendingJobs != 1 {
		t.Errorf("expected 1 pending job, got %d", pendingJobs)
	}
}
