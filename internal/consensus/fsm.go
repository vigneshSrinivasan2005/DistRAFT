package consensus

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/raft"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
)

// CommandType helps us know what kind of event happened
type CommandType string

const (
	CmdSetJob          CommandType = "SET_JOB"
	CmdSubmitParentJob CommandType = "SUBMIT_PARENT_JOB"
)

// LogEvent is what we actually write to the Raft log
type LogEvent struct {
	Type        CommandType `json:"type"`
	JobID       string      `json:"job_id"`
	Job         *store.Job  `json:"job,omitempty"`  // Job data for SET_JOB
	Data        *store.Job  `json:"data,omitempty"` // Deprecated: use Job instead
	ClusterSize int         `json:"cluster_size,omitempty"` // For parent job splitting
}

// FSM implementation
type FSM struct {
	state *store.State
}

func NewFSM(state *store.State) *FSM {
	return &FSM{state: state}
}

// Apply is called once a log entry is committed by a majority
func (f *FSM) Apply(l *raft.Log) interface{} {
	var event LogEvent
	if err := json.Unmarshal(l.Data, &event); err != nil {
		panic(fmt.Sprintf("failed to unmarshal command: %s", err.Error()))
	}

	switch event.Type {
	case CmdSetJob:
		// Support both Job and Data fields for backwards compatibility
		job := event.Job
		if job == nil {
			job = event.Data
		}
		f.state.Apply(event.JobID, job)
		return nil
	case CmdSubmitParentJob:
		// Split parent job into sub-jobs for each node
		parentJob := event.Job
		if parentJob == nil {
			parentJob = event.Data
		}
		if parentJob == nil || event.ClusterSize == 0 {
			return fmt.Errorf("invalid parent job: missing data or cluster size")
		}
		for i := 1; i <= event.ClusterSize; i++ {
			nodeID := NodeIDFromIndex(i)
			subJobID := fmt.Sprintf("%s-%s", event.JobID, nodeID)
			subJob := &store.Job{
				ID:        subJobID,
				Type:      parentJob.Type,
				Status:    store.StatusPending,
				WorkerID:  nodeID,
				ResultURL: "",
			}
			f.state.Apply(subJobID, subJob)
		}
		return nil
	default:
		return fmt.Errorf("unknown command type: %s", event.Type)
	}
}

// Snapshot returns a point-in-time snapshot of the system
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return &fsmSnapshot{state: f.state}, nil
}

// Restore restores the system from a snapshot
func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	return f.state.Unmarshal(data)
}

// --- Snapshot Helper ---

type fsmSnapshot struct {
	state *store.State
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	err := func() error {
		data, err := s.state.Marshal()
		if err != nil {
			return err
		}
		if _, err := sink.Write(data); err != nil {
			return err
		}
		return sink.Close()
	}()
	if err != nil {
		sink.Cancel()
	}
	return err
}

func (s *fsmSnapshot) Release() {}

// Helper functions

// NodeIDFromIndex converts node index (1, 2, 3) to node ID ("node-1", "node-2", "node-3")
func NodeIDFromIndex(index int) string {
	return fmt.Sprintf("node-%d", index)
}

// MustMarshalEvent marshals a LogEvent and panics on error (for internal use)
func MustMarshalEvent(event LogEvent) []byte {
	data, err := json.Marshal(event)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal event: %v", err))
	}
	return data
}
