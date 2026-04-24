// No failures — baseline performance. Unary only
// Client connects directly to leader, executes immediately.
// No redirects, no retries, no cache hits.
// This is the happy path latency baseline to compare against the other scenarios.

package scenarios

import (
	"os/exec"
	"testing"
	"time"
)

func TestScenario4_NoFailures(t *testing.T) {
	leader := startServer(t, "50051", true, "localhost:50051")
	defer stopServer(leader)

	start := time.Now()
	out, err := exec.Command("./client_bin",
		"-addr=localhost:50051",
		"-mode=unary",
	).Output()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("client failed: %v\noutput: %s", err, out)
	}
	t.Logf("scenario 4 latency (no failures, baseline): %v", elapsed)
	t.Logf("output: %s", out)
}

