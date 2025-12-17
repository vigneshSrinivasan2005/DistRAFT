package tests

import (
    "testing"
    "github.com/vigneshSrinivasan2005/DistRAFT/internal/store"
)

func TestCollectParents(t *testing.T) {
    jobs := map[string]*store.Job{
        "job-a-node-1": {ID: "job-a-node-1"},
        "job-a-node-2": {ID: "job-a-node-2"},
        "job-a-node-3": {ID: "job-a-node-3"},
        "job-b-node-1": {ID: "job-b-node-1"},
    }

    parents := map[string]struct{}{}
    for id := range jobs {
        if len(id) >= 7 && (id[len(id)-7:] == "-node-1" || id[len(id)-7:] == "-node-2" || id[len(id)-7:] == "-node-3") {
            p := id[:len(id)-7]
            parents[p] = struct{}{}
        }
    }

    if len(parents) != 2 {
        t.Fatalf("expected 2 parents, got %d", len(parents))
    }
}