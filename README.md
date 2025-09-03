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

- **API keys**: Used for authentication and rate limiting per user
- **Session IDs**: Random identifiers for session request correlation
- **Bandwidth metrics**: Request/response sizes for system monitoring
- **Conversation history**: Messages stored in memory with structured metadata
to maintain conversation context and enable bandwidth optimization

**What we DON'T store:**

- **User identities**: No persistent accounts or personal data tracking
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
- API key-based rate limiting provides per-user limits
- Conversation history persists in server memory during active sessions for context and optimization
- All data is ephemeral but may remain in memory until session cleanup occurs

Never send passwords, personal API keys, or other sensitive information through any chat system.

## Authentication

The server now requires API key authentication for all endpoints except health checks.

### Server Setup

Configure API keys in the server environment:

```bash
# Server .env
API_KEYS=key1,key2,key3
DAILY_CALL_LIMIT=100
GEMINI_API_KEY=your_gemini_key
```

### Client Setup

Configure your API key in the client environment:

```bash
# Client .env  
API_KEY=key1
```

## Quick Start

**Development (requires API key):**

```bash
git clone <repo> && cd microchat.ai
./certs/generate-certs.sh
cp .env.example .env
# Edit .env to add API_KEYS for server and API_KEY for client
make dev-server        # Terminal 1
make dev-client-echo    # Terminal 2
```

**Production (requires API keys + Gemini API key):**

1. Get Gemini API key: <https://ai.google.dev/gemini-api/docs/api-key>
2. Generate your authentication API keys
3. Add to server `.env`: `API_KEYS=key1,key2`, `DAILY_CALL_LIMIT=100`, and `GEMINI_API_KEY=your_key_here`
4. Add to client `.env`: `API_KEY=key1`
5. Run: `make prod-server` and `make prod-client-gemini`

## Environment Variables

Copy `.env.example` to `.env` and configure:

**Server:**
- `API_KEYS` - Comma-separated list of valid API keys (required)
- `DAILY_CALL_LIMIT` - Daily call limit per API key (default: 100)
- `GEMINI_API_KEY` - Your Gemini API key (production only)
- `APP_ENV=development` - Enables Echo provider
- Certificate paths - Use defaults for development

**Client:**  
- `API_KEY` - Your API key for server authentication (required)
- Certificate paths - Use defaults for development
