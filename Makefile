# Simple test entrypoints for DistRAFT

.PHONY: test test-all build cluster clean-cluster

# Run shared test suite only (fast)
test:
	go test ./tests

# Run full module tests (includes package-specific tests if added later)
test-all:
	go test ./...

# Build the server binary
build:
	go build -o raft-node ./cmd/server/main.go

# Spin up a fully connected 3-node local cluster
cluster: build
	@echo "--- 🧹 Clearing previous state ---"
	@pkill -f "raft-node" || true
	@rm -rf raft-data
	@mkdir -p raft-data
	@sleep 1

	@echo "--- 🚀 Starting Node 1 (Leader/Bootstrap) ---"
	@./raft-node -id=node-1 -raft=localhost:7000 -http=:8000 -bootstrap=true > /tmp/node1.log 2>&1 &
	@sleep 2 # Wait for leader election

	@echo "--- 🚀 Starting Node 2 (Follower) ---"
	@./raft-node -id=node-2 -raft=localhost:7001 -http=:8001 > /tmp/node2.log 2>&1 &

	@echo "--- 🚀 Starting Node 3 (Follower) ---"
	@./raft-node -id=node-3 -raft=localhost:7002 -http=:8002 > /tmp/node3.log 2>&1 &
	@sleep 2 # Wait for followers to boot

	@echo "--- 🤝 Joining Cluster ---"
	@curl "http://localhost:8000/join?nodeID=node-2&raftAddr=localhost:7001"
	@echo ""
	@curl "http://localhost:8000/join?nodeID=node-3&raftAddr=localhost:7002"
	@echo ""

	@echo "--- ✅ Cluster Ready! ---"
	@echo "Logs: tail -f /tmp/node*.log"
	@echo "API:  curl -X POST localhost:8000/submit -d '{\"id\": \"job-1\", \"status\": \"PENDING\"}'"

# Kill all cluster nodes
clean:
	@pkill -f "raft-node" || echo "No running nodes found"
	@rm -rf raft-data
	@echo "Cluster stopped and data cleaned"

# Watch the logs of all 3 nodes at once (Requires 'multitail' or just use tail)
logs:
	tail -f /tmp/node1.log /tmp/node2.log /tmp/node3.log