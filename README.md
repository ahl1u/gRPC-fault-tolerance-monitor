# gRPC Fault-Tolerant Service
**Andrew Liu, Hannah Wang, Christopher Kim**

## Overview
This project implements a fault-tolerant gRPC service in Go that explores how
distributed systems handle partial failure. The system uses a leader-follower
architecture for unary requests and demonstrates two recovery mechanisms:
client-side retry with leader redirect, and server-side deduplication through
a reply cache. We evaluate the latency impact of normal execution,
redirect-based recovery, cached retries, and streaming behavior across five
scenarios.

## Architecture
The system follows a leader-follower model. For unary Execute requests, one
server is designated the leader and is the only server that processes the
request. Follower servers do not execute unary requests; instead, they return
an UNAVAILABLE error and include the current leader's address in gRPC response
metadata. The client interceptor reads this metadata and transparently retries
the request against the leader.

There is no automatic leader election in this implementation. Failover is
simulated manually in tests by promoting a follower with the Promote RPC and
updating follower redirect targets with the UpdateLeader RPC. This is an
intentional simplification: the focus of the project is the client recovery
path and deduplication behavior, not consensus or election.

The proto contract defines four RPCs: Execute (unary), Stream
(server-streaming), Promote, and UpdateLeader.

## Client Interceptors
The client includes two interceptors that make retry behavior transparent to
the application.

The **UnaryClientInterceptor** wraps every Execute call. If it receives an
UNAVAILABLE error, it checks the response headers for a leader-addr value. If
that metadata is present, it opens a new connection to the reported leader and
retries the same request. If no redirect hint is present, it waits briefly and
retries the same server. This loop runs up to three times.

The **StreamClientInterceptor** wraps streaming RPCs by returning a custom
retryStream. This wrapper overrides RecvMsg(). If RecvMsg() returns
UNAVAILABLE, the interceptor creates a new stream on the same client connection
and swaps it in so the application can continue calling Recv(). Unlike the
unary interceptor, the streaming interceptor does not inspect leader metadata
or redirect to a different server.

## Server Interceptor
The server includes a unary interceptor that implements a reply cache for
deduplication. For each unary request, the interceptor checks the request ID
in an in-memory map. On a cache hit, it returns the stored response immediately
without calling the handler. On a cache miss, it runs the handler, stores the
response, and returns it.

This prevents duplicate execution when the same unary request is sent twice to
the same running server. It demonstrates deduplication behavior, but it is not
a full exactly-once guarantee across crashes or leader failover, since the cache
is only stored in memory on one server.

A background garbage-collection goroutine runs every 30 seconds and evicts
cache entries older than 5 minutes.

## Important Limitation
The reply cache is per-server and in-memory only. If a server actually crashes,
its cache is lost. Because of that, the current implementation only guarantees
deduplication for repeated requests reaching the same live server process. A
production system would need replicated state, likely through a consensus
protocol such as Raft, to preserve this property across failover.

## Failure Scenarios
We test five scenarios. To reduce measurement noise, the scenario tests invoke
a precompiled client binary.

**Scenario 4** is the unary baseline: the client sends a unary request directly
to the leader with no failure or redirect.

**Scenario 5** is the stream baseline: the client receives 10 stream messages
with 500ms spacing, for a total duration of about 5 seconds.

**Scenario 1** simulates redirect-based unary recovery. The original leader is
stopped, a follower is promoted, and another follower is updated with the new
leader address. The client then contacts that follower, receives a redirect, and
retries successfully against the promoted leader. This scenario measures
follower redirect plus client retry, not direct recovery from initially dialing
a dead leader.

**Scenario 2** demonstrates reply-cache deduplication. The same unary request
ID is sent twice to the same server. The first request executes normally. The
second request hits the cache and returns the stored response without invoking
the handler again.

**Scenario 3** is intended to test stream behavior during a leader failure. The
client begins a stream, and the test attempts to stop the original leader
mid-stream and promote a follower. However, the current stream implementation
does not enforce leader-only execution or redirect via leader metadata. The
streaming path therefore demonstrates reconnect behavior more than true
leader-aware failover semantics.

## Results

| Scenario | Description | Time |
|---|---|---|
| 4 | Unary baseline (no failures) | 40ms |
| 2 | Cache hit (no re-execution) | 20ms |
| 1 | Redirect + retry after leader death | 486ms |
| 5 | Stream baseline (no failures) | 5.07s |
| 3 | Stream reconnect (mid-stream failure) | 5.09s |

The unary baseline and cache-hit scenarios show that the normal unary path is
fast and that cache hits avoid handler execution. The redirect scenario adds
overhead from retrying through a follower before reaching the current leader.
The streaming baseline takes about 5 seconds because the server sends 10
messages at 500ms intervals. The stream recovery scenario shows little
additional latency, but this should be interpreted carefully because the stream
path does not currently implement the same leader-redirect logic as unary
requests.

## Limitations and Future Work
The main limitation is that the reply cache is not replicated, so deduplication
does not survive server crashes or leader failover. Leader election is also
manual; adding heartbeat-based failure detection and automatic election would
make the system substantially more realistic.

The streaming path is another important limitation. Stream does not currently
check whether the server is leader, and the stream interceptor reconnects only
on the same client connection rather than redirecting to a new leader. A
stronger implementation would make streaming follow the same leader-aware
semantics as unary requests and would define whether reconnect should restart
the stream or resume from the last delivered message.

Finally, request IDs are currently hardcoded to "1" in the client. A real
system would generate unique request IDs, such as UUIDs, for each logical
operation.

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
