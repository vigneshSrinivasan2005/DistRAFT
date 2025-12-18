package consensus

import (
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
)

type RaftNode struct {
	Raft        *raft.Raft
	FSM         *FSM
	logStore    raft.LogStore    // Keep reference to close on shutdown
	stableStore raft.StableStore // Keep reference to close on shutdown
}

func NewRaftNode(nodeID, raftAddr, raftDir string, state *store.State) (*RaftNode, error) {

	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(nodeID)

	// Setup TCP Transport
	addr, err := net.ResolveTCPAddr("tcp", raftAddr)
	if err != nil {
		return nil, err
	}

	transport, err := raft.NewTCPTransport(raftAddr, addr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, err
	}

	// Setup Snapshot Store
	snapshotStore, err := raft.NewFileSnapshotStore(raftDir, 1, os.Stderr)
	if err != nil {
		return nil, err
	}

	// Setup Log Store (BoltDB)
	boltDBFile := filepath.Join(raftDir, "raft.db")
	logStore, err := raftboltdb.NewBoltStore(boltDBFile)
	if err != nil {
		return nil, err
	}

	// Reuse the same Bolt store for stable storage to avoid double-opening the file.
	stableStore := logStore

	fsm := NewFSM(state)

	// Start Raft
	r, err := raft.NewRaft(config, fsm, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		logStore.Close()
		return nil, err
	}

	return &RaftNode{Raft: r, FSM: fsm, logStore: logStore, stableStore: stableStore}, nil
}

// Close gracefully shuts down the Raft node and closes all file handles.
func (n *RaftNode) Close() error {
	// First shutdown Raft
	if err := n.Raft.Shutdown().Error(); err != nil {
		return err
	}
	// Then close the log store (which is also the stable store)
	if closer, ok := n.logStore.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}
