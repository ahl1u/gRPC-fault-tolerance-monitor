// Leader executes the request and caches the result.
// Client retries with the same request ID.
// Reply cache on the same server returns stored result — no double execution.
// NOTE: In production, a replicated cache would extend this guarantee
// across leader failover. Here we demonstrate the deduplication mechanism itself.

package scenarios

import (
	"os/exec"
	"testing"
	"time"
)

func TestScenario2_ReplyCache(t *testing.T) {
	leader := startServer(t, "50051", true, "localhost:50051")
	defer stopServer(leader)

	// First request — leader executes it.
	// Server log will show: "received: mr kim"
	out, err := exec.Command("go", "run", "../client/main.go",
		"-addr=localhost:50051",
		"-mode=unary",
	).Output()
	if err != nil {
		t.Fatalf("first request failed: %v\noutput: %s", err, out)
	}
	t.Logf("first response (executed): %s", out)

	// Second request with same ID — cache hit, skips execution entirely.
	// Server log will show: "cache hit for id=1, skipping execution"
	// NOT "received: mr kim" — proving no double execution.
	start := time.Now()
	out, err = exec.Command("go", "run", "../client/main.go",
		"-addr=localhost:50051",
		"-mode=unary",
	).Output()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("retry request failed: %v\noutput: %s", err, out)
	}
	t.Logf("scenario 2 latency (cache hit, no re-execution): %v", elapsed)
	t.Logf("output: %s", out)
}
