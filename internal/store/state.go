package store

import (
	"encoding/json"
	"sync"
	"time"
)

type JobStatus string

const (
	StatusPending   JobStatus = "PENDING"
	StatusRunning   JobStatus = "RUNNING"
	StatusCompleted JobStatus = "COMPLETED"
	StatusFailed    JobStatus = "FAILED"
)

// Job represents a single ML task
type Job struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // e.g., "mnist_train"
	Status     JobStatus `json:"status"`
	WorkerID   string    `json:"worker_id"` // Which node is doing the work?
	ResultURL  string    `json:"result_url"`
	StartedAt  int64     `json:"started_at,omitempty"`  // Unix timestamp when job started
	UpdatedAt  int64     `json:"updated_at,omitempty"`  // Unix timestamp of last update
	RetryCount int       `json:"retry_count,omitempty"` // Number of retry attempts
}

// State is the thread-safe "Database"
type State struct {
	sync.RWMutex // make the jobs map thread-safe
	Jobs         map[string]*Job
}

func NewState() *State {
	return &State{
		Jobs: make(map[string]*Job),
	}
}

// GetJob reads a job safely
func (s *State) GetJob(id string) (*Job, bool) {
	s.RLock()
	defer s.RUnlock()
	j, ok := s.Jobs[id]
	return j, ok
}

// Apply sets a job state (Writing to the DB)
func (s *State) Apply(jobID string, job *Job) {
	s.Lock()
	defer s.Unlock()
	s.Jobs[jobID] = job
}

// Marshal dumps state for snapshots
func (s *State) Marshal() ([]byte, error) {
	s.RLock()
	defer s.RUnlock()
	return json.Marshal(s.Jobs)
}

// Unmarshal restores state from snapshots
func (s *State) Unmarshal(data []byte) error {
	s.Lock()
	defer s.Unlock()
	return json.Unmarshal(data, &s.Jobs)
}

// GetAllJobs returns a snapshot of all jobs
func (s *State) GetAllJobs() map[string]*Job {
	s.RLock()
	defer s.RUnlock()
	snapshot := make(map[string]*Job, len(s.Jobs))
	for k, v := range s.Jobs {
		snapshot[k] = v
	}
	return snapshot
}

// GetStuckJobs returns jobs that have been running longer than timeout
func (s *State) GetStuckJobs(timeoutSeconds int64) []*Job {
	s.RLock()
	defer s.RUnlock()
	
	var stuck []*Job
	now := time.Now().Unix()
	
	for _, job := range s.Jobs {
		if job.Status == StatusRunning && job.StartedAt > 0 {
			elapsed := now - job.StartedAt
			if elapsed > timeoutSeconds {
				stuck = append(stuck, job)
			}
		}
	}
	
	return stuck
}
