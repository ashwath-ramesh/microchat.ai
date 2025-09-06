# Benchmarking Guide

## Overview

Performance benchmarks measure server speed and identify bottlenecks.

## Benchmarking Order (Bottom-Up)

```bash
# 1. Check data structures first (fastest, most isolated)
go test -run=^$ -bench=BenchmarkSessionStore -benchmem -benchtime=1s ./cmd/server/

# 2. Check application logic (if data structures are fast)
go test -run=^$ -bench=BenchmarkChat -benchmem -benchtime=1s ./cmd/server/

# 3. Check full system (if both above are fast)
go run cmd/loadtest/main.go

# Size-specific testing (debug memory performance)
go test -run=^$ -bench=BenchmarkSessionStore_AppendMessage -benchmem -benchtime=1s ./cmd/server/
go test -run=^$ -bench=BenchmarkSessionStore_GetMessages -benchmem -benchtime=1s ./cmd/server/
```

## Programs

| | Category | session_store_bench_test.go | grpc_handlers_bench_test.go | cmd/loadtest |
|---|---|---|---|---|
| **Program** | | cmd/server/session_store_bench_test.go | cmd/server/grpc_handlers_bench_test.go | cmd/loadtest/main.go |
| **Use Case** | | Data structure efficiency in memory | Application logic efficiency in isolation | End-to-end system performance |
| **Focus** | | Data operations (append, get, etc) | Chat request/response processing speed | Network I/O + real Google API calls |
| **Key Question** | | How expensive are session reads/writes? | How fast can we handle message patterns? | How does server behave under load? |
| | | **Depends On** | **Depends On** | **Depends On** |
| **Server CPU Speed** | Hardware | YES | YES | YES |
| **Server Memory Speed** | Hardware | YES | YES | YES |
| **Go Version** | Software | NO | SOME (affects optimization) | SOME (network optimizations) |
| **GOMAXPROCS** | Software | SOME (concurrency tests) | SOME (concurrent sessions) | SOME (concurrent connections) |
| **Server Disk I/O** | Local I/O | NO (no persistence) | NO (everything in memory) | NO |
| **TLS Overhead** | Crypto | NO | NO | SOME (varies by server CPU) |
| **Server Network** | Network | NO (in-memory only) | NO (mock provider) | YES (real API calls) |
| **Client Network** | Network | NO | NO | YES (bandwidth/latency) |

### Justification for "NO" Dependencies

**grpc_handlers_bench_test.go:**

- **Server Disk I/O: NO** - All session data stored in memory, no file operations during chat processing
- **TLS Overhead: NO** - Uses mock LLM provider, bypasses all TLS/network encryption
- **Server Network: NO** - Mock provider eliminates network stack, pure function calls
- **Client Network: NO** - No actual client connections, benchmark runs locally

**session_store_bench_test.go:**

- **Go Version: NO** - Simple data operations (append/read), minimal compiler optimization impact
- **Server Disk I/O: NO** - Pure in-memory data structure, no persistence layer
- **TLS Overhead: NO** - Direct method calls on session store, no crypto operations
- **Server Network: NO** - Direct memory access, no network protocols involved
- **Client Network: NO** - Local function calls only, no network communication

**test/load_test.go:**

- **Server Disk I/O: NO** - Server processes requests in memory, no data persistence during load test

### Primary Bottlenecks (Largest Dependency)

- **session_store_bench_test.go**: Server Memory Speed (memory-bound operations)
- **grpc_handlers_bench_test.go**: Server CPU Speed (compute-bound processing)
- **test/load_test.go**: Client Network (network-bound communication)

## Quick Commands

```bash
# Run all benchmarks (skip unit tests to reduce noise)
go test -run=^$ -bench=. ./cmd/server/

# Run with memory stats
go test -run=^$ -bench=. -benchmem ./cmd/server/

# Run for specific duration (default is 1s)
go test -run=^$ -bench=. -benchtime=5s ./cmd/server/

# Run exact number of iterations
go test -run=^$ -bench=. -benchtime=1000x ./cmd/server/

# Save baseline
go test -run=^$ -bench=. -count=5 ./cmd/server/ > baseline.txt

# Compare performance
go test -run=^$ -bench=. -count=5 ./cmd/server/ > new.txt
benchstat baseline.txt new.txt
```

## Understanding -benchtime

- `-benchtime=1s` (default): Run each benchmark for 1 second
- `-benchtime=10s`: Run for 10 seconds (more reliable, slower)
- `-benchtime=100x`: Run exactly 100 iterations (faster, less reliable)
- `-benchtime=100ms`: Run for 100 milliseconds (quick test)

**Note**: Fast machines may hit session limits with long benchtime values.

## Understanding -benchmem

**BYTES/OP**: Total heap memory allocated per operation

- **Includes**: New objects, string concatenations, slice expansions, temporary allocations
- Higher = more GC pressure and memory usage
- `290 B/op` = efficient, `138KB/op` = memory-heavy

**ALLOCS/OP**: Number of individual heap allocations per operation  

- **Includes**: Each `make()`, `new()`, literal creation that escapes to heap
- Higher = more GC pauses even with small total bytes
- `0 allocs/op` = perfect (stack-only), `1 alloc/op` = efficient, `5+ allocs/op` = investigate

**Best performance**: Low BYTES/OP + Low ALLOCS/OP = GC-friendly code

## Avoiding Log Noise

**Problem**: Without `-run=^$`, you'll see lots of logs before benchmark results:

```
time=2025-09-06T09:03:51.581+02:00 level=INFO msg="received chat request"...
time=2025-09-06T09:03:51.582+02:00 level=WARN msg="invalid session ID"...
```

**Solution**: Always use `-run=^$` to skip unit tests:

```bash
# Clean output (recommended)
go test -run=^$ -bench=BenchmarkSessionStore ./cmd/server/

# Noisy output (avoid)
go test -bench=BenchmarkSessionStore ./cmd/server/
```

## Realistic Session Limits

Benchmarks use realistic conversation lengths based on research:

- **Customer support**: 8-25 messages
- **AI assistants**: 10-100 messages  
- **Technical discussions**: 50-200 messages
- **Extended sessions**: 200-500 messages (rare)

**Benchmark limit**: 200 messages per session (covers 80%+ of real conversations)
