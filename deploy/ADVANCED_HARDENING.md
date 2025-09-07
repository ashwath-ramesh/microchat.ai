# Advanced Security Hardening

Optional security configurations for production deployments.

## Current Security Features

Default `microchat.service` includes:

- Dedicated service user (`microchat`)
- Filesystem isolation (`ProtectSystem=full`, `ProtectHome=true`)
- Device isolation (`PrivateDevices=true`)
- Capability restrictions (`CapabilityBoundingSet=CAP_NET_BIND_SERVICE`)
- System call filtering (`SystemCallFilter=@system-service`)

## Network Isolation Options

### Option 1: Complete Isolation + Proxy

Block all outbound connections, use proxy:

```ini
# In microchat.service [Service] section
IPAddressDeny=any
IPAddressAllow=localhost
Environment=HTTPS_PROXY=http://127.0.0.1:3128
```

Setup proxy:

```bash
sudo apt install squid
# Edit /etc/squid/squid.conf:
# acl apis dstdomain .googleapis.com .openai.com .anthropic.com
# http_access allow apis
# http_access deny all
```

### Option 2: DNS Whitelisting

Dynamic IP resolution:

```bash
# Script to resolve and allow API domains
for domain in generativelanguage.googleapis.com; do
    systemctl set-property microchat.service IPAddressAllow="$(dig +short $domain)/32"
done
```

### Option 3: Firewall

Use iptables:

```bash
sudo iptables -I OUTPUT -m owner --uid-owner microchat -j DROP
sudo iptables -I OUTPUT -m owner --uid-owner microchat \
    -d $(dig +short generativelanguage.googleapis.com) -j ACCEPT
```

## Additional Hardening

### Memory Protection

```ini
MemoryDenyWriteExecute=true
RestrictRealtime=true
LockPersonality=true
```

### Filesystem

```ini
ProtectKernelTunables=true
ProtectKernelModules=true
ReadWritePaths=/opt/microchat/logs
```

### Resource Limits

```ini
CPUQuota=50%
MemoryMax=512M
TasksMax=100
```

## Testing

### Verify Isolation

```bash
# Should fail
sudo -u microchat ls /root

# Should work  
sudo -u microchat nc -l 4000
```

### Check Capabilities

```bash
sudo -u microchat capsh --print
```

