# gRPC Fault-Tolerant Service
Andrew Liu, Hannah Wang, Christopher Kim

## Overview

This project implements a fault-tolerant gRPC service in Go that explores how
distributed systems handle partial failure. The core problem we investigate is:
what happens when a leader server dies mid-request? We build a leader-follower
cluster and implement two layers of fault tolerance: automatic client-side retry
with leader redirect, and server-side exactly-once semantics via a reply cache,
then measure the latency cost of each recovery path across five scenarios.

## Architecture

The system follows a leader-follower model. One server is designated the leader
and is the only server that processes requests. Follower servers exist to redirect
clients to the leader. When a client hits a follower, the follower returns an
UNAVAILABLE error with the leader's address embedded in the response metadata.
The client interceptor reads this metadata and transparently replays the request
to the correct server.

There is no automatic leader election in this implementation. Failover is
simulated manually in tests by killing the leader process, promoting a follower
via the Promote RPC, and updating other followers via the UpdateLeader RPC. This
is an intentional simplification — the interesting behavior we are studying is
the client recovery path, not the election mechanism itself. A production system
would use a consensus protocol like Raft to handle this automatically.

The proto contract defines three core RPCs: Execute (unary), Stream
(server-streaming), and Promote (promotes a server to leader), plus UpdateLeader
for dynamically updating a follower's redirect target.

## Client Interceptors

The client has two interceptors that handle fault tolerance transparently — the
application code that calls Execute or Stream never needs to know about failures
or retries.

The UnaryClientInterceptor wraps every Execute call. On an UNAVAILABLE error it
checks the response metadata for a leader-addr field. If present, it opens a new
connection to that address and replays the request. If not present, it retries
the same server. This loop runs up to three times, resetting the header on each
attempt to avoid using stale redirect addresses.

The StreamClientInterceptor wraps every Stream call by returning a retryStream
struct instead of the raw gRPC stream. The retryStream overrides RecvMsg — when
the application calls Recv(), gRPC internally calls RecvMsg(). If RecvMsg gets
an UNAVAILABLE error, the interceptor silently opens a new stream and swaps it
in, returning nil instead of the error. The application never sees the failure
and continues calling Recv() normally.

## Server Interceptors

The server has one interceptor: a reply cache that implements exactly-once
semantics. Every incoming unary request passes through this interceptor before
reaching the handler. The interceptor checks the request ID against an in-memory
map. On a cache hit, it returns the stored response immediately without calling
the handler. On a cache miss, it calls the handler, stores the result, and
returns it.

This prevents double execution when a client retries: if the leader executed a
request and then crashed before responding, the client will retry with the same
request ID and get the cached result without the operation running again.

The cache includes a background garbage collection goroutine that runs every 30
seconds and evicts entries older than 5 minutes. This bounds memory usage in
long-running servers while still covering any realistic retry window.

The limitation of this implementation is that the cache is in-memory per server.
It does not survive a crash or transfer to a new leader. In a production system
you would replicate the cache across all nodes using a consensus protocol so the
guarantee holds across leader failover. Our implementation demonstrates the
deduplication mechanism works correctly; the replication layer is out of scope.

## Failure Scenarios

We test five scenarios. All latency measurements use a precompiled client binary
to avoid Go compilation overhead skewing the numbers.

Scenario 4 is the unary baseline: client connects directly to the leader with no
failures. This gives us a floor of 40ms for a local gRPC round trip.

Scenario 5 is the stream baseline: client streams all 10 messages from the leader
with no failures. Takes 5.07 seconds: 10 messages at 500ms intervals.

Scenario 1 tests leader death before execution. The leader is killed before the
client sends its request. The client hits a follower, gets redirected to the new
leader, and completes successfully. Recovery takes 486ms: significantly higher
than baseline due to TCP connection timeout overhead from the killed server's port
not being immediately released by the OS. In production with heartbeat-based
failure detection this would be much closer to baseline.

Scenario 2 tests exactly-once semantics. The same request ID is sent to the same
server twice. The first call executes and caches the result: server logs show
"received: mr kim". The second call hits the cache: server logs show "cache hit
for id=1, skipping execution". The second call takes 20ms, slightly faster than
baseline because the cache returns before the handler is ever invoked.

Scenario 3 tests leader death mid-stream. The leader is killed 2.5 seconds into
a 5-second stream. The stream interceptor catches UNAVAILABLE on RecvMsg, opens
a new stream, and swaps it in. The application code never sees an error. Total
time is 5.09 seconds: only 20ms more than the stream baseline, showing the
reconnect overhead is essentially negligible.

## Results Summary

| Scenario | Time | Overhead vs Baseline |
|---|---|---|
| 4 — unary baseline | 40ms | 0ms |
| 2 — cache hit | 25ms | faster (shorter code path) |
| 1 — redirect + retry | 486ms | +446ms |
| 5 — stream baseline | 5.07s | 0ms |
| 3 — stream reconnect | 5.09s | +20ms |

## Limitations and Future Work

The primary limitation is the in-memory reply cache. Replicating it across nodes
using Raft would enable true exactly-once guarantees across leader failover.
Additionally, leader election is manual in this implementation — adding automatic
heartbeat-based failure detection and election would make the system production-ready.
Stream reconnect does not replay already-received messages, which means a
reconnected stream starts fresh rather than resuming. Finally, all request IDs
are hardcoded to "1" in this implementation; a real system would generate unique
UUIDs per request.

## Running the Tests

Build the client binary first:
```bash
go build -o scenarios/client_bin ./client/
```

Then run each scenario with port cleanup between tests:
```bash
go test ./scenarios/ -run TestScenario1_LeaderDiesBeforeExecution -v -count=1
lsof -i :50051 -i :50052 -i :50053 | grep LISTEN | awk '{print $2}' | xargs kill -9 2>/dev/null
go test ./scenarios/ -run TestScenario2_ReplyCache -v -count=1
lsof -i :50051 -i :50052 -i :50053 | grep LISTEN | awk '{print $2}' | xargs kill -9 2>/dev/null
go test ./scenarios/ -run TestScenario3_LeaderDiesMidStream -v -count=1
lsof -i :50051 -i :50052 -i :50053 | grep LISTEN | awk '{print $2}' | xargs kill -9 2>/dev/null
go test ./scenarios/ -run TestScenario4_NoFailures -v -count=1
lsof -i :50051 -i :50052 -i :50053 | grep LISTEN | awk '{print $2}' | xargs kill -9 2>/dev/null
go test ./scenarios/ -run TestScenario5_NoFailuresStream -v -count=1
lsof -i :50051 -i :50052 -i :50053 | grep LISTEN | awk '{print $2}' | xargs kill -9 2>/dev/null
```
