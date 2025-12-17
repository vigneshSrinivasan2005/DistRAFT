package master

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
)

// RunAggregator periodically checks for completed sub-jobs and merges their models.
func RunAggregator(state *store.State, parentPrefix string, pollInterval time.Duration) {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	for {
		time.Sleep(pollInterval)

		state.RLock()
		jobs := make(map[string]*store.Job)
		for id, job := range state.Jobs {
			jobs[id] = job
		}
		state.RUnlock()

		parents := collectParents(jobs, parentPrefix)
		for _, parent := range parents {
			subIDs := []string{fmt.Sprintf("%s-node-1", parent), fmt.Sprintf("%s-node-2", parent), fmt.Sprintf("%s-node-3", parent)}
			models := make([]string, 0, 3)
			allDone := true
			expectedShards := len(subIDs)
			foundShards := 0
			
			for _, sid := range subIDs {
				state.RLock()
				job, ok := state.Jobs[sid]
				state.RUnlock()
				if !ok || job.Status != store.StatusCompleted || job.ResultURL == "" {
					allDone = false
					break
				}
				models = append(models, job.ResultURL)
				foundShards++
			}
			
			// Validate all expected shards are present
			if !allDone {
				continue
			}
			
			if foundShards != expectedShards {
				log.Printf("⚠️ Aggregator: skipping %s - only %d/%d shards completed", parent, foundShards, expectedShards)
				continue
			}

			outPath := filepath.Join("raft-data", fmt.Sprintf("%s_global.pth", parent))
			args := append([]string{"ml-code/merge.py", parent, "--models"}, models...)
			args = append(args, "--out", outPath)
			cmd := exec.Command("python3", args...)
			stdout, _ := cmd.StdoutPipe()
			cmd.Stderr = cmd.Stderr
			if err := cmd.Start(); err != nil {
				log.Printf("Aggregator: failed to start merge.py: %v", err)
				continue
			}

			var lastLine string
			buf := make([]byte, 4096)
			for {
				n, err := stdout.Read(buf)
				if n > 0 {
					for _, line := range strings.Split(string(buf[:n]), "\n") {
						line = strings.TrimSpace(line)
						if line != "" {
							lastLine = line
							log.Printf("[Aggregator - %s] %s", parent, line)
						}
					}
				}
				if err != nil {
					break
				}
			}
			if err := cmd.Wait(); err != nil {
				log.Printf("Aggregator: merge.py crashed: %v", err)
				continue
			}

			var result struct {
				ParentID  string `json:"parent_id"`
				Status    string `json:"status"`
				ModelPath string `json:"model_path"`
				NumModels int    `json:"num_models"`
			}
			if err := json.Unmarshal([]byte(lastLine), &result); err != nil {
				log.Printf("Aggregator: failed to parse merge output: %v", err)
				continue
			}
			log.Printf("Aggregator: merged %d models for %s -> %s", result.NumModels, parent, result.ModelPath)
		}
	}
}

// collectParents finds unique parent IDs from jobs ending in -node-1/2/3.
func collectParents(jobs map[string]*store.Job, parentPrefix string) []string {
	parents := map[string]struct{}{}
	for id := range jobs {
		if parentPrefix != "" && !strings.HasPrefix(id, parentPrefix) {
			continue
		}
		if strings.HasSuffix(id, "-node-1") || strings.HasSuffix(id, "-node-2") || strings.HasSuffix(id, "-node-3") {
			p := id[:len(id)-7]
			parents[p] = struct{}{}
		}
	}
	out := make([]string, 0, len(parents))
	for p := range parents {
		out = append(out, p)
	}
	return out
}
