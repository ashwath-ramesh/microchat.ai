# microchat.ai

**Bandwidth compressed chats with a SOTA LLM over the wire.**

I am frequently on a plane with a 30MB in-flight Wi-Fi plan, and I want to have a
conversation with a SOTA LLM like Claude or Gemini.

The problem is that standard chat clients burn through data. Their requests are
verbose and the text isn't compressed, so my 30MB data cap disappears in
minutes.

So how can I have a long, useful conversation with a SOTA LLM without
running out of data?

I built `microchat.ai`, a system that uses a compression proxy to solve this
problem. It works using two parts:

1. **A terminal client (`cmd/client/`):** A CLI application that:
   - Connects over the wire via gRPC to a proxy server with gzip compression
   - Uses Protocol Buffers for compact binary encoding
   - Tracks real-time bandwidth usage at both payload and wire levels
   - Shows you exactly how many bytes you're sending/receiving

2. **A proxy server (`cmd/server/`):** A gRPC server that:
   - Receives your compressed messages over TLS-secured gRPC
   - Forwards them to LLM APIs (Claude, GPT-4, Gemini)
   - Compresses the LLM response before sending back
   - Maintains ephemeral sessions without logging conversations

This architecture strips out all the protocol overhead and focuses on
transferring only the essential, compressed data, letting you chat for hours,
not minutes.

## Privacy & Data Handling

This system minimizes data collection while maintaining practical functionality:

**What we track:**

- **IP addresses**: Used for rate limiting
- **Session IDs**: Random 16-bit identifiers for session request correlation
- **Bandwidth metrics**: Request/response sizes for system monitoring

**What we DON'T store:**

- **Conversation content**: Messages are proxied to LLMs but never stored
- **User identities**: No authentication, accounts, or tracking currently
- **Chat history**: Sessions are ephemeral - everything forgotten on disconnect

**Technical details:**

- Sessions use random 16-bit IDs (65,536 possibilities) in memory only
- No database - all state is held in RAM and cleared on disconnect
- TLS encryption for all client-server communication
- Future versions may implement client-side caching for optimization

**Important limitations:**

- Your messages ARE sent to LLM providers with their own data policies
- IP-based rate limiting means shared networks (offices, cafes) share limits
- Server logs may contain metadata (timestamps, errors) but never messages

Never send passwords, API keys, or other sensitive information through any chat system.

## Usage Limits

To keep this service free for everyone, I have to currently pay for the
server and LLM calls myself. To prevent abuse, I've set reasonable
rate limits on the public proxy. These limits should be more than
enough for normal conversations.

## Deployment

**Local Development:**

- `git clone <repo>` and `cd microchat.ai`
- `./certs/generate-certs.sh` to create certificates  
- `cp .env.example .env` for configuration
- `make server` and `make client` to run

**Production with Caddy:**

- Install Go and Caddy on VPS
- Configure Caddyfile: `yourdomain.com { reverse_proxy localhost:4000 }`
- Create production `.env` (no TLS vars needed - Caddy handles TLS)
- `make server` to run (app serves on :4000, Caddy proxies with TLS)
