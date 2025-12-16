package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vigneshSrinivasan2005/DistRAFT/internal/consensus"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
)

func TestNewRaftNodeCreatesResources(t *testing.T) {
	tempDir := t.TempDir()
	raftDir := filepath.Join(tempDir, "raft")
	if err := os.MkdirAll(raftDir, 0o700); err != nil {
		t.Fatalf("failed to create raft dir: %v", err)
	}

	state := store.NewState()
	node := createRaftNodeWithTimeout(t, func() (*consensus.RaftNode, error) {
		return consensus.NewRaftNode("node-test", "127.0.0.1:0", raftDir, state)
	})
	t.Cleanup(func() {
		done := make(chan struct{})
		go func() {
			_ = node.Raft.Shutdown().Error()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Fatalf("shutdown timeout")
		}
	})

	if node.Raft == nil {
		t.Fatalf("raft instance should not be nil")
	}
	if node.FSM == nil {
		t.Fatalf("fsm should not be nil")
	}
}

// createRaftNodeWithTimeout guards against hangs when constructing Raft.
func createRaftNodeWithTimeout(t *testing.T, ctor func() (*consensus.RaftNode, error)) *consensus.RaftNode {
	t.Helper()
	done := make(chan struct{})
	errCh := make(chan error, 1)
	var node *consensus.RaftNode
	go func() {
		defer close(done)
		var err error
		node, err = ctor()
		if err != nil {
			errCh <- err
		}
	}()
	select {
	case <-done:
	case err := <-errCh:
		t.Fatalf("NewRaftNode returned error: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatalf("NewRaftNode timed out")
	}
	return node
}
