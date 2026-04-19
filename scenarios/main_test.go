package scenarios

import (
    "context"
    "os"
    "os/exec"
    "testing"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    pb "github.com/ahl1u/gRPC-fault-tolerance-monitor/proto"
)

func startServer(t *testing.T, port string, leader bool, leaderAddr string) *exec.Cmd {
    args := []string{"run", "../server/main.go", "-port=" + port, "-leader-addr=" + leaderAddr}
    if leader {
        args = append(args, "-leader=true")
    }
    cmd := exec.Command("go", args...)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Start(); err != nil {
        t.Fatalf("failed to start server: %v", err)
    }
    time.Sleep(500 * time.Millisecond)
    return cmd
}

func stopServer(cmd *exec.Cmd) {
    cmd.Process.Kill()
}

func promoteServer(t *testing.T, addr string) {
    conn, _ := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
    client := pb.NewFaultTolerantClient(conn)
    client.Promote(context.Background(), &pb.PromoteRequest{})
    conn.Close()
}

func updateLeader(t *testing.T, addr string, newLeaderAddr string) {
    conn, _ := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
    client := pb.NewFaultTolerantClient(conn)
    client.UpdateLeader(context.Background(), &pb.UpdateLeaderRequest{NewLeaderAddr: newLeaderAddr})
    conn.Close()
}