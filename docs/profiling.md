# Profiling Guide

## Overview

Pprof profiling enables runtime introspection of the live server to debug performance issues.

## Purpose

**Benchmarks** measure system capabilities offline. **Profiling** debugs live issues during incidents.

## Security

**Production-Ready Security Features:**

1. **Localhost Only**: Server binds to `127.0.0.1` only - not accessible from network
2. **Admin Authentication**: Bearer token required for all endpoints 
3. **Configurable Port**: Set `PPROF_PORT` environment variable (default: 6060)
4. **Graceful Shutdown**: Server shuts down cleanly with main process
5. **Isolated State**: Uses dedicated ServeMux, no global DefaultServeMux pollution

**Production Access**: Use SSH tunnel to access profiling endpoints securely:
```bash
ssh -L 6060:127.0.0.1:6060 user@production-server
# Then access http://127.0.0.1:6060 locally
```

## Available Endpoints

Server automatically exposes profiling endpoints on `127.0.0.1:6060` (localhost only):

- `/debug/pprof/` - Index page with all available profiles
- `/debug/pprof/profile` - CPU profile (default 30 seconds)
- `/debug/pprof/heap` - Current heap memory usage
- `/debug/pprof/goroutine` - All goroutine stack traces
- `/debug/pprof/block` - Goroutines blocked on synchronization
- `/debug/pprof/mutex` - Mutex contention hotspots

## Quick Commands

```bash
# Set admin key (required for all profiling commands)
export ADMIN_KEY=your-admin-key-here

# CPU profiling (30 seconds)
make pprof-cpu

# Heap memory analysis
make pprof-heap

# Goroutine inspection
make pprof-goroutines

# Custom duration CPU profile
curl -H "Authorization: Bearer $ADMIN_KEY" -o /tmp/cpu.prof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=60'
go tool pprof /tmp/cpu.prof

## Configuration

Set environment variables to customize profiling:

```bash
export PPROF_PORT=6060      # Default profiling port
export ADMIN_KEY=your-key   # Required for authentication
```

## When to Use Profiling

| Problem | Profile Type | Investigation |
|---|---|---|
| High CPU usage | CPU profile | Which functions consume most CPU time |
| Memory leaks | Heap profile | What objects are accumulating in memory |
| High latency | CPU + goroutine | Blocking operations and slow functions |
| Deadlocks | Goroutine profile | Stack traces of all goroutines |
| Resource contention | Mutex/block profiles | Synchronization bottlenecks |

## Zero Overhead

Profiling has **zero runtime cost** until endpoints are accessed. The pprof HTTP server runs separately from the main gRPC server on port 6060.

## Interactive Analysis

After capturing a profile, pprof opens an interactive shell:

```
(pprof) top10          # Show top 10 functions by CPU time
(pprof) list main.Chat # Show source code with timing annotations
(pprof) web           # Generate visual graph (requires graphviz)
(pprof) peek main.Chat # Show callers/callees of function
```