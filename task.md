Failure Type	What Happens?	Detection Mechanism
Node Crash	The entire VM/Pod dies. Raft heartbeats fail.	Timeout. The Leader stops receiving updates for the job.
Worker Crash	The Python script dies, but the Go node is up.	Error RPC. The Go node sees the exit code (non-zero) and sends a JOB_FAILED RPC to the leader.
Network Partition	Node is running, but can't talk to Leader.	Timeout. The Leader stops receiving updates.

How to Implement It
You need to add a "Watchdog" or "Reconciler" to your Leader logic.

Step A: Update your Job Struct
You need to track when the job was last "touched."

type SubJob struct {
    ID            string
    AssignedNode  string
    Status        string // PENDING, RUNNING, COMPLETED, FAILED
    LastUpdated   time.Time // <--- NEW: Track the last heartbeat/update from the worker
    RetryCount    int       // <--- NEW: Avoid infinite retry loops
}

Step B: The Leader's "Reconcile" Loop
The Leader should run a background goroutine that wakes up every few seconds (e.g., every 5s) to check for "stale" jobs.

Logic:

Loop through all RUNNING jobs.

Check: if time.Now() - job.LastUpdated > JOB_TIMEOUT (e.g., 30s):

Assume failure. The node might be down, or the network is cut.

Action: Mark job as PENDING again.

Action: Re-assign to a different node (if available).

Action: Increment RetryCount. If RetryCount > MaxRetries, mark as DEAD (don't retry forever).

Step C: The Worker's Responsibility
The Worker node needs to be a "good citizen."

Active Heartbeats: While the Python script is running, the Go node should send a lightweight "I'm still working" RPC to the leader every 5-10 seconds. This updates the LastUpdated timestamp.

Catching Crashes: If the Python script exits with an error (e.g., exit code 1), the Go node must not stay silent. It must immediately send a ReportFailure RPC to the leader so the leader can reschedule immediately without waiting for the timeout.

Revised Flow Diagram
Here is how the system handles a Node Crash with this logic:

T=0s: Leader assigns Job-A to Node-2.

T=1s: Node-2 starts training.

T=5s: CRASH! Node-2 loses power.

T=10s: Leader's Reconcile Loop runs. Checks Job-A.

LastUpdated was T=0s.

CurrentTime is T=10s.

Diff is 10s. (Assume Timeout is 30s). Wait.

T=35s: Leader's Reconcile Loop runs.

Diff is 35s. Timeout Exceeded!

Leader marks Job-A as PENDING.

Leader looks for available nodes. Finds Node-3.

Leader assigns Job-A to Node-3.

T=40s: Node-3 completes the job. Success.

