// No failures — baseline stream performance.
// Client connects directly to leader, streams all 10 messages.
// No reconnects, no failures.
// Compare against scenario 3 to see the overhead of mid-stream reconnect.

package scenarios

import (
	"os/exec"
	"testing"
	"time"
)

func TestScenario5_NoFailuresStream(t *testing.T) {
	leader := startServer(t, "50051", true, "localhost:50051")
	defer stopServer(leader)

	start := time.Now()
	out, err := exec.Command("./client_bin",
		"-addr=localhost:50051",
		"-mode=stream",
	).Output()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("stream failed: %v\noutput: %s", err, out)
	}
	t.Logf("scenario 5 stream latency (no failures, baseline): %v", elapsed)
	t.Logf("output: %s", out)
}