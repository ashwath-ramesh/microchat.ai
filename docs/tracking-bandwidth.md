# Notes on tracking bandwidth usage

## Summary

We use package to track bandwidth usage:
- `google.golang.org/grpc/stats`
- `https://github.com/grpc/grpc-go`

### References

1. **grpc wire protocol**: [github.com/grpc/grpc/blob/master/doc/protocol-http2.md](https://github.com/grpc/grpc/blob/master/doc/protocol-http2.md)
2. **grpc stats source**: [github.com/grpc/grpc-go/blob/master/stats/stats.go](https://github.com/grpc/grpc-go/blob/master/stats/stats.go)
3. **http/2 spec**: [rfc 7540](https://datatracker.ietf.org/doc/html/rfc7540)

At a high level, this is what we get from the package:

| wirelength (bytes) | out | in |
|--------------------|-----|----|
| header             | -   | x  |
| payload            | x   | x  |
| trailer            | -   | x  |

## Detailed breakdown of grpc stats wirelength measurements

| component | direction (client→server) | grpc stats access | wirelength includes | wirelength excludes | size info available |
|-----------|-----------|-------------------|---------------------|---------------------|---------------------|
| **header** | out  | `wirelength`: no | n/a - not measured | n/a - not measured | none |
| **payload** | out | `wirelength`: yes. full stats. | • 5-byte grpc frame header<br>• compressed payload (if compression enabled) | • http/2 data frame header<br>• tls/tcp/ip overhead | • `stat.length` (uncompressed)<br>• `stat.compressedlength`<br>• `stat.wirelength` (with grpc frame) |
| **trailer** | out | `wirelength`: no. deprecated. | n/a - never set | n/a - never set | none |

| component | direction (server→client) | grpc stats access | wirelength includes | wirelength excludes | size info available |
|-----------|-----------|-------------------|---------------------|---------------------|---------------------|
| **header** | in | `wirelength`: yes | • hpack compressed headers<br>• `:method`, `:path`, `:scheme`, `:authority`<br>• `content-type`, `grpc-encoding`<br>• custom metadata | • http/2 9-byte frame header<br>• tls overhead<br>• tcp/ip headers | `stat.wirelength` only |
| **payload** | in | `wirelength`: yes. full stats. | • 5-byte grpc frame header<br>• compressed payload (if compression enabled) | • http/2 data frame header<br>• tls/tcp/ip overhead | • `stat.length` (uncompressed)<br>• `stat.compressedlength`<br>• `stat.wirelength` (with grpc frame) |
| **trailer** | in | `wirelength`: yes | • hpack compressed trailers<br>• `grpc-status`, `grpc-message`<br>• custom trailing metadata | • http/2 9-byte frame header<br>• tls/tcp/ip overhead | `stat.wirelength` only |


### protocol stack layers

```
application layer    [the protobuf message]
     ↓
grpc layer          [5-byte frame header + compressed payload]
     ↓  
http/2 layer        [http/2 frames, headers, data, etc.]
     ↓
tls layer           [tls records, encryption, mac]
     ↓
tcp layer           [tcp headers, sequence numbers]
     ↓
ip layer            [ip headers, routing info]
```

### What each layer does (vs what grpc stats can see)

| layer | what happens | what grpc stats can see | what grpc stats cant see |
|-------|-------------|------------------------|---------------------------|
| **application** | protobuf serialization | message size via interceptor | internal object structure |
| **grpc** | • adds 5-byte frame header<br>• applies compression<br>• manages streaming | • frame header (in wirelength)<br>• compression ratio<br>• payload sizes | • internal state management<br>• stream multiplexing details |
| **http/2** | • headers/data/trailer frames<br>• 9-byte frame headers<br>• flow control<br>• hpack compression | • hpack compressed header content (partial) | • frame headers (9 bytes)<br>• settings frames<br>• ping frames<br>• window_update frames |
| **tls** | • handshake (~2-5kb)<br>• record headers (5 bytes)<br>• encryption + mac<br>• ~5-10% overhead | nothing directly | • all tls overhead<br>• handshake bytes<br>• certificate exchange<br>• encryption overhead |
| **tcp** | • 20-60 byte headers<br>• segmentation<br>• acks<br>• retransmission | nothing | • all tcp headers<br>• ack packets<br>• retransmissions<br>• connection establishment |
| **ip** | • 20 byte headers<br>• routing<br>• fragmentation | nothing | • all ip headers<br>• routing metadata |

### Coverage summary

| metric level | components tracked | components not tracked | coverage |
|--------------|-------------------|------------------------|----------|
| **application level** (`appbytes*`) | • raw protobuf message size only | • all protocol overhead<br>• compression<br>• headers/trailers | ~10-20% of actual traffic |
| **process level** (`procbytes*`) | • grpc frame headers (5 bytes)<br>• compressed payloads<br>• http/2 headers content<br>• all inheader, inpayload, intrailer<br>• outpayload | • outheader (not available)<br>• outtrailer (deprecated)<br>• http/2 frame headers (9 bytes each)<br>• http/2 control frames (ping, settings, window_update)<br>• tls handshake (~2-5kb initial)<br>• tls encryption overhead (~5-10%)<br>• tcp/ip headers (~40 bytes/packet) | ~60-70% of actual traffic |

### Example

For a simple grpc call with a 100-byte protobuf message:

```
application sees:     100 bytes (protobuf)
grpc wirelength:      105 bytes (5-byte header + 100 payload)
http/2 on wire:       ~150 bytes (with headers, frames)
tls on wire:          ~200 bytes (with encryption, mac)
tcp/ip on wire:       ~300 bytes (multiple packets with headers)
```

### Insights

- **client sends but cant measure**: outheader size (metadata going to server)
- **client measures everything recieved**: inheader + inpayload + intrailer
- **missing ~30-40%**: mostly from tls overhead and tcp/ip protocol headers
- **compression benefit**: visible as difference between `length` and `compressedlength` in payload stats

### Known limitations: what isnt measured
- `grpc stats handler` provides high-precision metrics within 
the application layer and cannot see the full picture of what is transmitted by the
operating system's network stack. The key blind spots are:

- **tcp/ip headers (~40-60 bytes per packet):** every piece of data is broken into
  packets, and each packet has tcp (~20 bytes) and ip (~20 bytes) headers. For a 1kb
  message sent in a single packet, this adds an unmeasured ~40 bytes of overhead.
- **initial tls handshake (~2-4 kb):** the connection setup involves a one-time
     exchange of security certificates and keys. This traffic occurs before the stats
     handler begins tracking rpcs and is not included in the totals.
- **dns & other control traffic:** the initial dns lookup to find the server and
     any underlying network control packets (e.g., arp) are not measured. This is
     typically a very small, one-time cost.

note: current implementation captures about 60-70% of actual network traffic, which is ok (for now) for understanding app bandwidth usage.
