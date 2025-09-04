# microchat.ai

**Bandwidth compressed chats with a SOTA LLM over the wire.**

I am frequently on a plane with a 30MB in-flight Wi-Fi plan, and I want to 
have a conversation with a SOTA LLM like Claude or Gemini.

The problem is that standard chat clients burn through data. Their requests 
are verbose and the text isn't compressed, so my 30MB data cap disappears 
in minutes.

So how can I have a long, useful conversation with a SOTA LLM without
running out of data?

I built `microchat.ai`, a system that uses a compression proxy to solve 
this problem. It works using two parts:

1. **A terminal client (`cmd/client/`):** A CLI application that:
   - Connects via gRPC to a proxy server with gzip compression
   - Uses Protocol Buffers for compact binary encoding
   - Tracks real-time bandwidth usage at payload and wire levels
   - Shows exactly how many bytes you're sending/receiving

2. **A proxy server (`cmd/server/`):** A gRPC server that:
   - Receives your compressed messages over TLS-secured gRPC
   - Forwards them to LLM APIs (Claude, GPT-4, Gemini)
   - Compresses the LLM response before sending back
   - Maintains ephemeral sessions without logging

This architecture strips out protocol overhead and focuses on transferring 
only essential, compressed data, letting you chat for hours, not minutes.

## Privacy & Data Handling

- **Ephemeral sessions**: No persistent storage - all data held in RAM only
- **No user tracking**: Random session IDs, no accounts or personal data  
- **TLS encrypted**: All client-server communication is encrypted
- **Messages forwarded**: Your messages are sent to LLM providers

Never send passwords or sensitive information through any chat system.

## Client Setup

**Build and connect to production server:**

```bash
# Build client binary (from project root)
go build -o microchat-client cmd/client/*.go

# Get API key from server admin, then connect
export MICROCHAT_API_KEY=your_api_key
./microchat-client -addr="microchat.ai:443"

# With metrics tracking:
./microchat-client -addr="microchat.ai:443" -metrics

# With detailed metrics:  
./microchat-client -addr="microchat.ai:443" -metrics-detail
```

The client automatically detects production domains and uses system certs.

## Server Setup  

**Deploy production server:**

```bash
# Get Gemini API key: https://ai.google.dev/gemini-api/docs/api-key
# Build server binary
go build -o server cmd/server/*.go

# Configure environment (.env file)
API_KEYS=key1,key2,key3
DAILY_CALL_LIMIT=100  
GEMINI_API_KEY=your_gemini_key_here

# Run server
./server
```

## Development

For local development with self-signed certs, see `.env.example` for config.
