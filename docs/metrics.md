# Prometheus Metrics Guide

## Overview

Server metrics provide real-time insights into performance, resource usage, and system health.

## Quick Commands

```bash
# Test metrics endpoint
make prometheus-metrics

# Raw curl with admin auth
curl -H "Authorization: Bearer admin-key" http://127.0.0.1:9090/metrics

# Check specific metrics
curl -H "Authorization: Bearer admin-key" http://127.0.0.1:9090/metrics | grep microchat_
```

## Available Metrics

| Metric | Type | Description | Labels |
|---|---|---|---|
| `microchat_request_duration_seconds` | Histogram | Duration of gRPC requests | `method` |
| `microchat_llm_call_duration_seconds` | Histogram | LLM provider call duration | `provider` |
| `microchat_active_sessions` | Gauge | Currently active sessions | - |
| `microchat_sessions_created_total` | Counter | Total sessions created | - |
| `microchat_rate_limit_exceeded_total` | Counter | Rate limit rejections | - |
| `microchat_request_bytes` | Histogram | Request payload sizes | `method` |

## Metric Types Explained

**Histogram**: Time-based measurements with configurable buckets
- Provides percentiles (p50, p95, p99) and totals
- Best for: response times, payload sizes, durations

**Counter**: Always-increasing values
- Resets on server restart
- Best for: total requests, errors, events

**Gauge**: Current point-in-time values
- Can increase or decrease
- Best for: active connections, memory usage, queue size

## Grafana Query Examples

### Response Time Percentiles
```promql
# 95th percentile response time by method
histogram_quantile(0.95, rate(microchat_request_duration_seconds_bucket[5m]))

# Average LLM call duration by provider
rate(microchat_llm_call_duration_seconds_sum[5m]) / rate(microchat_llm_call_duration_seconds_count[5m])
```

### System Load
```promql
# Active sessions trend
microchat_active_sessions

# Request rate by method
rate(microchat_request_duration_seconds_count[1m])
```

### Error Monitoring  
```promql
# Rate limit hit rate
rate(microchat_rate_limit_exceeded_total[5m])

# Session creation rate vs active sessions (detect leaks)
rate(microchat_sessions_created_total[5m]) - rate(microchat_active_sessions[5m])
```

## Architecture

```
Client Request → gRPC Handler → [Timer Start] → LLM Provider → [Timer End] → Response
                      ↓                              ↓
                 Record Request                Record LLM Call
                 Duration & Size               Duration
                      ↓                              ↓
                 Prometheus Metrics ← HTTP /metrics ← Grafana/AlertManager
```

## Access & Security

| Component | Port | Authentication | Access |
|---|---|---|---|
| **Prometheus Metrics** | 9090 | Admin Bearer Token | Network accessible |
| **pprof Profiling** | 6060 | Admin Bearer Token | localhost only |
| **gRPC API** | 4000 | API Key + TLS | Network accessible |

**Production Access**: Direct network access for metrics, SSH tunnel for profiling:
```bash
# Metrics (network accessible)
curl -H "Authorization: Bearer admin-key" http://production-server:9090/metrics

# Profiling (localhost only, needs tunnel)  
ssh -L 6060:127.0.0.1:6060 user@production-server
curl -H "Authorization: Bearer admin-key" http://127.0.0.1:6060/debug/pprof/heap
```

## Troubleshooting

### Common Issues

**403 Unauthorized**: Check admin key in Authorization header
```bash
# Wrong
curl http://127.0.0.1:6060/metrics

# Correct  
curl -H "Authorization: Bearer your-admin-key" http://127.0.0.1:6060/metrics
```

**Connection refused**: Server not running or wrong port
```bash
# Check if server is running
make server

# Verify port configuration  
echo $METRICS_PORT  # Default: 9090
echo $PPROF_PORT    # Default: 6060
```

**No metrics appear**: Prometheus collectors not initialized
- Metrics are created on first use
- Make some API calls to populate data

### Performance Impact

**Metrics collection overhead**: ~0.1% CPU per 1000 RPS
- Histograms: Minimal impact, pre-allocated buckets
- Counters/Gauges: Near-zero overhead
- Network cost: ~2KB per scrape

## Integration Examples

### Basic Prometheus Config
```yaml
scrape_configs:
  - job_name: 'microchat'
    static_configs:
      - targets: ['localhost:9090']
    authorization:
      credentials: 'admin-key'
    metrics_path: '/metrics'
    scrape_interval: 15s
```

### AlertManager Rules
```yaml
groups:
  - name: microchat
    rules:
    - alert: HighResponseTime
      expr: histogram_quantile(0.95, rate(microchat_request_duration_seconds_bucket[5m])) > 2
      annotations:
        summary: "95% of requests taking >2s"
    
    - alert: RateLimitSpike  
      expr: rate(microchat_rate_limit_exceeded_total[1m]) > 10
      annotations:
        summary: "Rate limit exceeded >10/min"
```