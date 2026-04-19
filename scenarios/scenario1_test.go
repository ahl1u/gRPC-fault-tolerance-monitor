// Leader dies before responding; 
// the client retries, gets redirected to a new server, and rebinds. 
// Clean failure, nothing was executed yet.

// note that we only simulate leader death and the rebind that we retry
// is with the two follower servers. in an ideal system, we would have automatic
// leader election where we hit the dead server and get rerouted, but our current implementation
// requires us to test the rebind/retry latency manually instead of getting automatic reelection.

package scenarios

import (
    "os/exec"
    "testing"
    "time"
)

func TestScenario1_LeaderDiesBeforeExecution(t *testing.T) {
	leader := startServer(t, "50051", true, "localhost:50051")
	follower1 := startServer(t, "50052", false, "localhost:50051")
	follower2 := startServer(t, "50053", false, "localhost:50051")
	defer stopServer(follower1)
	defer stopServer(follower2)

	stopServer(leader)

	promoteServer(t, "localhost:50052")

	updateLeader(t, "localhost:50053", "localhost:50052")

	start := time.Now()
    out, err := exec.Command("go", "run", "../client/main.go",
        "-addr=localhost:50053",
        "-mode=unary",
    ).Output()
    elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("client failed: %v", err)
	}
	t.Logf("scenario 1 latency: %v", elapsed)
	t.Logf("output: %s", out)
}