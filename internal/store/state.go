package store

import (
	"encoding/json"
	"sync"
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
	ID        string    `json:"id"`
	Type      string    `json:"type"` // e.g., "mnist_train"
	Status    JobStatus `json:"status"`
	WorkerID  string    `json:"worker_id"` // Which node is doing the work?
	ResultURL string    `json:"result_url"`
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
