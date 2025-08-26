# To Do's

## Request flow

`client -> gzip -> server -> decompress
-> llm api -> compress -> client`

- [x] Setup basic gRPC server
- [x] Add compression (gzip)
- [] Session management
- [] Rate limiting
- [] LLM integration
- [] ???

## Tracking bytes

- Application level: measure size of serialized
protobuf payload. Data representing only the chat
message. Does not contain any protocol overhead.
- OS process level: All bytes sent/received on
behalf of the application PID. Should contain:
  - gRPC/HTTP2 overhead: framing, headers, control messages
  - TLS overhead: handshake, per record overhead. Note: can we
  eliminate repeated connection startup?
  - TCP/IP overhead: headers per packet, conn mgmt packets (syn/ack, fin)
  - DNS lookups: resolve proxy server hostname to ip addr
  - Certificate checks: ocsp or crl checks to validate servers tls certs.
- Measure Initial TLS Handshake: Investigate using a custom `grpc.Dialer` or
     wrapping the `tls.Conn` to count the bytes of the initial handshake, which is
     currently unmeasured.
- Estimate TCP/IP Overhead: Since direct measurement is not cross-platform,
  add a feature to estimate and account for TCP/IP header overhead (e.g., add a
  configurable ~40 bytes per sent/received message group).

## Compression checklist

- [] Payload redefined from HTTP REST JSON APIs to gRPC serialized protocol buffer
- [] reduce `.proto` file to contain only minimal reqd information
- [] in `.proto` use bit packed data structures (session_id is fixed32,
model and message types are enum ~1byte, use gRPC error code so none created,
efficient protobuf strings for messages)
- [] Enable TLS compression use gzip requests on client side: enable on server side
- [] Connection pooling; not restarting sessions
- []
- []
