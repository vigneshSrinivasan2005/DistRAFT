package worker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
)

// Helper struct to match the Python JSON output
type PythonResult struct {
	JobID     string  `json:"job_id"`
	Status    string  `json:"status"`
	Accuracy  float64 `json:"accuracy"`
	Loss      float64 `json:"loss"`
	ModelPath string  `json:"model_path"`
}

func RunWorker(state *store.State, httpAddr string) {
	log.Println("👷 WORKER STARTED: Listening for jobs...")

	for {
		time.Sleep(2 * time.Second)

		// 1. Find a Pending Job
		var jobToRun *store.Job
		state.RLock()
		for _, job := range state.Jobs {
			if job.Status == store.StatusPending {
				jobToRun = job
				break
			}
		}
		state.RUnlock()

		if jobToRun == nil {
			continue
		}

		// 2. Run the Job
		log.Printf("🚀 Found Pending Job: %s. Starting Python...", jobToRun.ID)
		result, err := RunPythonScript(jobToRun.ID)

		if err != nil {
			log.Printf("❌ Job %s failed: %v", jobToRun.ID, err)
			continue
		}

		// 3. Report Success to Raft (Close the Loop!)
		log.Printf("📬 Reporting completion for %s to Cluster...", jobToRun.ID)
		if err := ReportSuccess(httpAddr, result); err != nil {
			log.Printf("❌ Failed to report success: %v", err)
		} else {
			log.Printf("✅ Job %s cycle complete.", jobToRun.ID)
		}
	}
}

func RunPythonScript(jobID string) (*PythonResult, error) {
	cmd := exec.Command("python3", "ml-code/train.py", jobID)
	cmd.Stderr = os.Stderr
	stdout, _ := cmd.StdoutPipe()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start python: %v", err)
	}

	var lastLine string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("[Worker - %s] %s", jobID, line)
		// Keep track of the last line, which contains the JSON result
		if strings.TrimSpace(line) != "" {
			lastLine = line
		}
	}

	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("python script crashed: %v", err)
	}

	// Parse the final JSON line
	var result PythonResult
	if err := json.Unmarshal([]byte(lastLine), &result); err != nil {
		return nil, fmt.Errorf("failed to parse result JSON: %v", err)
	}

	return &result, nil
}

// ReportSuccess sends the result back to the Leader via HTTP
func ReportSuccess(leaderAddr string, result *PythonResult) error {
	// Construct the payload for the API
	// Note: We are reusing the existing 'Job' struct structure
	payload := map[string]interface{}{
		"id":         result.JobID,
		"status":     "COMPLETED",
		"result_url": result.ModelPath,
	}

	data, _ := json.Marshal(payload)
	resp, err := http.Post("http://localhost"+leaderAddr+"/submit", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}
