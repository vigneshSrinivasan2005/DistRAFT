package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/hashicorp/raft"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/consensus"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
	"github.com/vigneshSrinivasan2005/DistRAFT/internal/worker"
)

func main() {
	// 1. Parse Command Line Arguments
	// We need these to run multiple nodes on one laptop (different ports)
	nodeID := flag.String("id", "node-1", "Unique ID for this node")
	raftAddr := flag.String("raft", "localhost:7000", "Address for Raft transport")
	httpAddr := flag.String("http", ":8000", "Address for HTTP API")
	bootstrap := flag.Bool("bootstrap", false, "Bootstrap the cluster (only for the first node)")
	flag.Parse()

	// 2. Setup Data Directory
	// This is where Raft stores its logs. We create a folder named after the Node ID.
	raftDir := fmt.Sprintf("raft-data/%s", *nodeID)
	os.MkdirAll(raftDir, 0700)

	// 3. Initialize the State (The Brain)
	fsmStore := store.NewState()

	// 4. Initialize Raft (The Engine)
	rNode, err := consensus.NewRaftNode(*nodeID, *raftAddr, raftDir, fsmStore)
	if err != nil {
		log.Fatalf("Failed to create raft node: %v", err)
	}

	// 5. Handle Bootstrap
	// The first node needs to say "I am the leader" to start the cluster.
	if *bootstrap {
		cfg := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raft.ServerID(*nodeID),
					Address: raft.ServerAddress(*raftAddr),
				},
			},
		}
		f := rNode.Raft.BootstrapCluster(cfg)
		if err := f.Error(); err != nil {
			log.Printf("Bootstrap error (might already be initialized): %v", err)
		}
	}

	// 6. Define cluster size (for now, hardcoded to 3 nodes)
	clusterSize := 3

	// 7. Define HTTP API Handlers
	// These allow us to talk to the cluster using curl or Postman.

	// Handler: Submit a new Job
	http.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var job store.Job
		if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Prepare the command for Raft
		// Use SUBMIT_PARENT_JOB to automatically split into sub-jobs
		event := consensus.LogEvent{
			Type:        consensus.CmdSubmitParentJob,
			JobID:       job.ID,
			Data:        &job,
			ClusterSize: clusterSize,
		}
		eventBytes, _ := json.Marshal(event)

		// Apply to Raft (This is the magic moment!)
		// We give it a 5-second timeout to get a consensus.
		applyFuture := rNode.Raft.Apply(eventBytes, 5*time.Second)
		if err := applyFuture.Error(); err != nil {
			http.Error(w, "Raft error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte(fmt.Sprintf("Parent job %s split into %d sub-jobs successfully", job.ID, clusterSize)))
	})

	// Handler: Join Cluster (Add a new node)
	http.HandleFunc("/join", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		nodeID := query.Get("nodeID")
		raftAddr := query.Get("raftAddr")

		if nodeID == "" || raftAddr == "" {
			http.Error(w, "Missing nodeID or raftAddr", http.StatusBadRequest)
			return
		}

		log.Printf("Received join request from %s at %s", nodeID, raftAddr)

		// Tell Raft to add this new guy to the voting pool
		future := rNode.Raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(raftAddr), 0, 0)
		if err := future.Error(); err != nil {
			http.Error(w, "Failed to join: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte("Node joined successfully"))
	})

	// Handler: Get Job Status (Read from local memory)
	http.HandleFunc("/job", func(w http.ResponseWriter, r *http.Request) {
		jobID := r.URL.Query().Get("id")
		job, ok := fsmStore.GetJob(jobID)
		if !ok {
			http.Error(w, "Job not found", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(job)
	})

	// Handler: Update Job Status (used by workers to report completion)
	http.HandleFunc("/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var update store.Job
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Get existing job to preserve all fields
		existingJob, ok := fsmStore.GetJob(update.ID)
		if !ok {
			http.Error(w, "Job not found", http.StatusNotFound)
			return
		}

		// Merge update fields with existing job
		if update.Status != "" {
			existingJob.Status = update.Status
		}
		if update.ResultURL != "" {
			existingJob.ResultURL = update.ResultURL
		}

		// Use CmdSetJob for direct updates (no splitting)
		event := consensus.LogEvent{
			Type:  consensus.CmdSetJob,
			JobID: existingJob.ID,
			Data:  existingJob,
		}
		eventBytes, _ := json.Marshal(event)

		applyFuture := rNode.Raft.Apply(eventBytes, 5*time.Second)
		if err := applyFuture.Error(); err != nil {
			http.Error(w, "Raft error: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte("Job updated successfully"))
	})
	
	// 8. Start the worker goroutine
	go worker.RunWorker(fsmStore, *httpAddr, *nodeID, clusterSize)

	log.Printf("Server started on HTTP %s (Raft %s)", *httpAddr, *raftAddr)
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
}
