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
- **Session IDs**: Random identifiers for session request correlation
- **Bandwidth metrics**: Request/response sizes for system monitoring
- **Conversation history**: Messages stored in memory with structured metadata
to maintain conversation context and enable bandwidth optimization

**What we DON'T store:**

- **User identities**: No authentication, accounts, or tracking
- **Persistent chat history**: Sessions are ephemeral - all conversation data is held only in RAM
- **Message logs**: Server logs contain operational metadata but never actual message content

**Technical details:**

- Sessions use randomly generated IDs stored in memory only
- No database - all conversation state is held in RAM and cleared when sessions end
- Structured message storage enables delta protocol optimization (sending only new messages)
- TLS encryption for all client-server communication
- Automatic cleanup removes idle sessions to prevent memory buildup

**Important limitations:**

- Your messages ARE sent to LLM providers with their own data policies
- IP-based rate limiting means shared networks (offices, cafes) share limits
- Conversation history persists in server memory during active sessions for context and optimization
- All data is ephemeral but may remain in memory until session cleanup occurs

Never send passwords, API keys, or other sensitive information through any chat system.

## Usage Limits

To keep this service free for everyone, I have to currently pay for the
server and LLM calls myself. To prevent abuse, I've set reasonable
rate limits on the public proxy. These limits should be more than
enough for normal conversations.

## Quick Start

**Development (no API key needed):**

```bash
git clone <repo> && cd microchat.ai
./certs/generate-certs.sh
cp .env.example .env
make dev-server        # Terminal 1
make dev-client-echo    # Terminal 2
```

**Production (requires Gemini API key):**

1. Get API key: <https://ai.google.dev/gemini-api/docs/api-key>
2. Add to `.env`: `GEMINI_API_KEY=your_key_here`  
3. Run: `make prod-server` and `make prod-client-gemini`

## Environment Variables

Copy `.env.example` to `.env` and configure:

- `GEMINI_API_KEY` - Your Gemini API key (production only)
- `APP_ENV=development` - Enables Echo provider (no API key needed)
- Certificate paths - Use defaults for development
