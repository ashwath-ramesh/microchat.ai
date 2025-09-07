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
# See .env.example 

# Run server
./server
```

## Deployment

**VPS Setup Checklist:**

**Prerequisites:**

- [ ] Install Go
- [ ] Install Caddy

**Setup:**

- [ ] Clone repo:
  ```bash
  sudo git clone https://github.com/ashwath-ramesh/microchat.ai.git /opt/microchat
  cd /opt/microchat
  ```
- [ ] Create service user: `sudo useradd -r -s /bin/bash -d /opt/microchat microchat`
- [ ] Set ownership: `sudo chown -R microchat:microchat /opt/microchat`
- [ ] Generate certs as microchat user:
  ```bash
  sudo -u microchat ./certs/generate-certs.sh
  ```
  *(ECDSA P-384 certificates for internal TLS. Caddy handles public SSL automatically)*
- [ ] Configure environment:
  ```bash
  sudo cp .env.example .env
  sudo vim .env  # Set API_KEYS, GEMINI_API_KEY, PORT=4000
  sudo chown microchat:microchat .env
  ```
- [ ] Configure sudoers for service restart:
  ```bash
  echo "microchat ALL=(ALL) NOPASSWD: /bin/systemctl restart microchat" | sudo tee /etc/sudoers.d/microchat
  ```
- [ ] Install service:
  ```bash
  sudo cp deploy/microchat.service /etc/systemd/system/
  sudo systemctl enable microchat
  sudo systemctl start microchat
  ```
- [ ] Setup Caddy with security hardening:
  ```bash
  sudo cp deploy/Caddyfile /etc/caddy/
  sudo mkdir -p /var/log/caddy
  sudo chown caddy:caddy /var/log/caddy
  sudo systemctl enable caddy
  sudo systemctl start caddy
  ```

**Deploy updates:**

```bash
# Run as microchat user (NOT as root)
cd /opt/microchat && sudo -u microchat ./deploy/deploy.sh
```
