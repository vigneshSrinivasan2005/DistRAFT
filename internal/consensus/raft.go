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
	Raft *raft.Raft
	FSM  *FSM
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
		return nil, err
	}

	return &RaftNode{Raft: r, FSM: fsm}, nil
}
