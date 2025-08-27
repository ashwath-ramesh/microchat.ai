# Roadmap

## Current Status

Ultra-low bandwidth chat client for high-latency connections.

**Current features:**

- gRPC-based client-server architecture
- Protocol buffer serialization  
- gzip compression
- TLS security
- Bandwidth tracking and metrics

## Upcoming Features

### Server-Side Optimization

- **Stateful server** - Store conversation history server-side
- **Delta protocol** - Send only new messages, not full history
- **Rate limiting** - Prevent abuse and manage server resources

### Advanced Compression

- **Zstd with adaptive dictionaries** - Better compression ratios
- **Context-aware compression** - Conversation-specific dictionaries

### LLM Integration

- **Multi-LLM support** - Connect to various language model providers
- **Smart context pruning** - Summarize conversation history
- **Streaming responses** - Real-time token streaming

### Bandwidth Optimization

- **Bidirectional streaming** - Persistent connections

## Goals

**Primary:** Minimize bandwidth usage while maintaining chat functionality
**Target:** 90%+ bandwidth reduction vs traditional chat apps
**Use case:** AI chat over satellite, airplane wifi, constrained networks
