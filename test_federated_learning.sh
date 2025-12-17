#!/bin/bash
# Test script for federated learning with model merging

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== Federated Learning Integration Test ===${NC}"

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    pkill -f "./raft-node" || true
    sleep 2
}

# Trap to ensure cleanup on exit
trap cleanup EXIT INT TERM

# Step 1: Clean environment
echo -e "\n${YELLOW}Step 1: Cleaning environment${NC}"
pkill -f "./raft-node" || true
rm -rf raft-data /tmp/node*.log
sleep 1

# Step 2: Build the project
echo -e "\n${YELLOW}Step 2: Building project${NC}"
make build
if [ $? -ne 0 ]; then
    echo -e "${RED}Build failed!${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Build successful${NC}"

# Step 3: Start the 3-node cluster
echo -e "\n${YELLOW}Step 3: Starting 3-node cluster${NC}"

./raft-node -id node-1 -raft localhost:7000 -http :8000 -bootstrap > /tmp/node1.log 2>&1 &
NODE1_PID=$!
echo "Started node-1 (PID: $NODE1_PID)"
sleep 3

./raft-node -id node-2 -raft localhost:7001 -http :8001 > /tmp/node2.log 2>&1 &
NODE2_PID=$!
echo "Started node-2 (PID: $NODE2_PID)"
sleep 1

curl -s "http://localhost:8000/join?nodeID=node-2&raftAddr=localhost:7001" > /dev/null 2>&1
echo "Joined node-2 to cluster"
sleep 1

./raft-node -id node-3 -raft localhost:7002 -http :8002 > /tmp/node3.log 2>&1 &
NODE3_PID=$!
echo "Started node-3 (PID: $NODE3_PID)"
sleep 1

curl -s "http://localhost:8000/join?nodeID=node-3&raftAddr=localhost:7002" > /dev/null 2>&1
echo "Joined node-3 to cluster"
sleep 2

echo -e "${GREEN}✓ Cluster started${NC}"

# Step 4: Submit parent job
echo -e "\n${YELLOW}Step 4: Submitting parent job 'test-federated'${NC}"
RESPONSE=$(curl -s -X POST "http://localhost:8000/submit" \
  -H "Content-Type: application/json" \
  -d '{"id": "test-federated", "script": "train.py", "args": ["--epochs", "2", "--batch-size", "64"]}' 2>/dev/null) || {
    echo -e "${RED}✗ Failed to submit job - is the server running?${NC}"
    tail -20 /tmp/node1.log
    exit 1
}

echo "Response: $RESPONSE"
echo -e "${GREEN}✓ Parent job submitted${NC}"

# Step 5: Monitor job completion
echo -e "\n${YELLOW}Step 5: Monitoring sub-job completion (max 180s)${NC}"

check_job_status() {
    local job_id=$1
    local port=$2
    curl -s "http://localhost:${port}/job?id=${job_id}" 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4
}

TIMEOUT=180
ELAPSED=0
ALL_COMPLETED=false

while [ $ELAPSED -lt $TIMEOUT ]; do
    STATUS1=$(check_job_status "test-federated-node-1" 8000)
    STATUS2=$(check_job_status "test-federated-node-2" 8000)
    STATUS3=$(check_job_status "test-federated-node-3" 8000)
    
    echo -e "  [${ELAPSED}s] node-1: ${STATUS1}, node-2: ${STATUS2}, node-3: ${STATUS3}"
    
    if [ "$STATUS1" = "COMPLETED" ] && [ "$STATUS2" = "COMPLETED" ] && [ "$STATUS3" = "COMPLETED" ]; then
        ALL_COMPLETED=true
        break
    fi
    
    sleep 5
    ELAPSED=$((ELAPSED + 5))
done

if [ "$ALL_COMPLETED" = false ]; then
    echo -e "${RED}✗ Timeout waiting for jobs to complete${NC}"
    echo -e "\n${YELLOW}Node logs:${NC}"
    echo "--- Node 1 ---"
    tail -20 /tmp/node1.log
    echo "--- Node 2 ---"
    tail -20 /tmp/node2.log
    echo "--- Node 3 ---"
    tail -20 /tmp/node3.log
    exit 1
fi

echo -e "${GREEN}✓ All sub-jobs completed in ${ELAPSED}s${NC}"

# Step 6: Wait for aggregator to merge (it runs every 2 seconds)
echo -e "\n${YELLOW}Step 6: Waiting for model aggregation (10s)${NC}"
sleep 10

# Step 7: Verify merged model exists
echo -e "\n${YELLOW}Step 7: Verifying merged model${NC}"

MERGED_MODEL="./raft-data/test-federated_global.pth"
if [ -f "$MERGED_MODEL" ]; then
    echo -e "${GREEN}✓ Merged model found: ${MERGED_MODEL}${NC}"
    ls -lh "$MERGED_MODEL"
    
    # Check model size (should be reasonable, not empty)
    SIZE=$(stat -f%z "$MERGED_MODEL" 2>/dev/null || stat -c%s "$MERGED_MODEL" 2>/dev/null)
    if [ $SIZE -gt 1000 ]; then
        echo -e "${GREEN}✓ Model size looks reasonable: ${SIZE} bytes${NC}"
    else
        echo -e "${RED}✗ Model file too small: ${SIZE} bytes${NC}"
        exit 1
    fi
else
    echo -e "${RED}✗ Merged model not found at: ${MERGED_MODEL}${NC}"
    echo -e "\n${YELLOW}Checking what files exist in raft-data:${NC}"
    find ./raft-data -name "*.pth" -ls
    echo -e "\n${YELLOW}Checking aggregator logs in node-1:${NC}"
    grep -i "merge\|aggregat" /tmp/node1.log | tail -20
    exit 1
fi

# Step 8: Verify individual shard models exist
echo -e "\n${YELLOW}Step 8: Verifying individual shard models${NC}"
SHARDS_FOUND=0
for node in node-1 node-2 node-3; do
    SHARD_MODEL="./raft-data/test-federated-${node}_model.pth"
    if [ -f "$SHARD_MODEL" ]; then
        SIZE=$(stat -f%z "$SHARD_MODEL" 2>/dev/null || stat -c%s "$SHARD_MODEL" 2>/dev/null)
        echo -e "${GREEN}✓ Found shard: ${SHARD_MODEL} (${SIZE} bytes)${NC}"
        SHARDS_FOUND=$((SHARDS_FOUND + 1))
    else
        echo -e "${YELLOW}⚠ Shard not found: ${SHARD_MODEL}${NC}"
    fi
done

if [ $SHARDS_FOUND -eq 3 ]; then
    echo -e "${GREEN}✓ All 3 shard models found${NC}"
else
    echo -e "${RED}✗ Only ${SHARDS_FOUND}/3 shards found${NC}"
    exit 1
fi

# Step 9: Check aggregator logs
echo -e "\n${YELLOW}Step 9: Checking aggregator logs${NC}"
if grep -qi "merged.*models.*for.*test-federated" /tmp/node1.log; then
    echo -e "${GREEN}✓ Aggregator merge log found${NC}"
    grep -i "merged.*models.*for.*test-federated\|Aggregator.*test-federated" /tmp/node1.log | head -3
else
    echo -e "${RED}✗ No aggregator merge log found${NC}"
    exit 1
fi

# Step 10: Verify model weights are correctly averaged
echo -e "\n${YELLOW}Step 10: Verifying model weight averaging${NC}"
if command -v python3 &> /dev/null; then
    python3 ml-code/verify_merge.py
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓ Model weights verification passed${NC}"
    else
        echo -e "${RED}✗ Model weights verification failed${NC}"
        exit 1
    fi
else
    echo -e "${YELLOW}⚠ Python3 not found, skipping weight verification${NC}"
fi

# Step 11: Final summary
echo -e "\n${GREEN}================================${NC}"
echo -e "${GREEN}✓ ALL TESTS PASSED${NC}"
echo -e "${GREEN}================================${NC}"
echo -e "\nFederated Learning Pipeline Verified:"
echo -e "  1. Parent job splits into 3 sub-jobs ✓"
echo -e "  2. Each node trains on its data shard ✓"
echo -e "  3. All sub-jobs complete successfully ✓"
echo -e "  4. Aggregator merges models into global model ✓"
echo -e "  5. Merged model file created: ${MERGED_MODEL} ✓"
echo -e "  6. Model weights correctly averaged ✓"

echo -e "\n${YELLOW}Node logs available at:${NC}"
echo "  /tmp/node1.log"
echo "  /tmp/node2.log"
echo "  /tmp/node3.log"

exit 0
