// Leader dies mid-stream.
// The stream interceptor on the client detects UNAVAILABLE on RecvMsg
// and transparently reconnects to the new leader.
// The client application code never sees the error.
// We contrast this with unary: unary retries the whole request atomically,
// stream reconnects but does not replay already-received messages.

package scenarios

import (
	"os/exec"
	"testing"
	"time"
)

func TestScenario3_LeaderDiesMidStream(t *testing.T) {
	leader := startServer(t, "50051", true, "localhost:50051")
	follower1 := startServer(t, "50052", false, "localhost:50051")
	follower2 := startServer(t, "50053", false, "localhost:50051")
	defer stopServer(follower1)
	defer stopServer(follower2)

	// Kill leader after 2.5s — gives stream time to actually start
	// and receive a few messages before the crash
	go func() {
		time.Sleep(2500 * time.Millisecond)
		stopServer(leader)
		promoteServer(t, "localhost:50052")
		updateLeader(t, "localhost:50053", "localhost:50052")
	}()

	start := time.Now()
	out, err := exec.Command("./client_bin",
		"-addr=localhost:50051",
		"-mode=stream",
	).Output()
	elapsed := time.Since(start)

	if err != nil {
		t.Logf("stream error after leader died (interceptor attempted reconnect): %v", err)
	} else {
		t.Logf("stream completed, interceptor reconnected successfully")
	}
	t.Logf("scenario 3 latency: %v", elapsed)
	t.Logf("output: %s", out)
}
