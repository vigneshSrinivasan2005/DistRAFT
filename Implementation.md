### Implementation Plan - Distributed ML System with RAFT
## Goal Description
The system will use the RAFT consensus algorithm to manage distributed state (storage and job coordination) and execute distributed Machine Learning training workloads.

## General Design 
Each Peer in the System will have 2 concurrent routines running at once both a RAFT node, sending its heart beat as per RAFT, and a worker which actually executes the ML tasks.

## ML WorkFlow Mgmt 
Use the RAFT logs to send which job each peer is working on
Use GRPC to send the ML models and gradients from leader to peer.

## Tech Stack:

Core System (Consensus & Networking): Go (Golang)
ML & Application Layer: Python (PyTorch/TensorFlow)
Communication: gRPC (Protobuf)
Deployment: Docker & Kubernetes

RAFT Implementation: We will use the industry-standard hashicorp/raft library for the consensus module to ensure stability. Used by Consul and Nomad.

Compute Model: The Go backend will act as a "Parameter Server" and "Job Coordinator". Python scripts will theoretically run on the same physical nodes (in Docker containers) but communicate with the Go agent via localhost gRPC.

## Verification Plan

#### Automated Tests
Go Unit Tests: Test the FSM logic (Does a log entry actually update the map?).
Integration Test: Spin up 3 Go processes locally. Kill the leader. Ensure a new leader is elected. Write data to the new leader. Read from a follower (stale read) or leader (strong read).
#### Manual Verification
Cluster Formation: Run 3 terminals. verify they join the same RAFT cluster.
Data Replication: put key=foo val=bar on Node 1. get key=foo on Node 3.
ML Job: Submit a job. Watch 3 Python windows start training. Watch loss decrease.

