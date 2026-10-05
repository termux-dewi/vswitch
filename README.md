# vswitch

Modular Go virtual Layer-2 switch based on the supplied `draf2.go` specification.

## Build

```bash
go mod tidy
go build ./cmd/vswitch
./vswitch
```

The environment running the build must have network access to download Go modules. The project includes checked-in protobuf Go sources under `tunnel/` so `protoc` is not required for the normal build.

## Runtime

- File UDS and Linux abstract UDS
- UDP frame transport
- Bidirectional gRPC stream
- gVisor IPv4/TCP/UDP/ICMP userspace stack
- ARP gateway responder
- DHCP lease persistence
- REST API and WebSocket metrics
- Embedded Tailwind/Chart.js dashboard
- JSON configuration

The `/api/v1/config` endpoint persists configuration, but listener/network changes take effect after restart.

## Network Emulation

Per-port traffic shaping can be applied at runtime via the REST API, from the web dashboard, or pre-configured in `config.json`. Supported parameters: latency, jitter, packet loss, bandwidth throttling, duplication, corruption, and reordering.

Built-in presets: `satellite`, `3g`, `lossy_wifi`, `congested`, `intermittent`.

### API endpoints

- `GET /api/v1/emulation` – active profiles, shaper stats, and preset names
- `GET /api/v1/emulation/presets` – full preset definitions
- `GET /api/v1/emulation/ports` – port keys that can be emulated
- `POST /api/v1/emulation` – apply a profile: `{"key":"udp:10.0.0.2:5678","preset":"3g"}` or custom `{"key":"...","profile":{"latency_ms":100,"jitter_ms":20,"loss_percent":2,"bandwidth_kbps":512}}`
- `DELETE /api/v1/emulation/<key>` – remove emulation from a port

### config.json

```json
"emulation": {"profiles": [{"key": "udp:10.0.0.2:5678", "preset": "3g"}]}
```

Configured profiles are applied at startup; unknown port keys are skipped with a log message.

## Android Client (gRPC + TLS)

The Android client connects to the vswitch gRPC server, creates a TUN interface, and bridges IP traffic over the virtual L2 network. It performs DHCP and ARP automatically.

### Generate TLS certificates

```bash
sh scripts/gen-certs.sh
```

This creates `certs/server.crt` and `certs/server.key`. Set them in `config.json`:

```json
"tls_cert": "certs/server.crt",
"tls_key": "certs/server.key"
```

### Build for Android (Termux, arm64)

```bash
sh scripts/build-android.sh
```

Copy `dist/vswitch-client-arm64` to the Android device.

### Run the client

```bash
# With TLS
sudo ./vswitch-client -addr <server-ip>:50051 -ca certs/server.crt -service my-phone -tun vswitch0

# Without TLS (insecure)
sudo ./vswitch-client -addr <server-ip>:50051 -service my-phone -tun vswitch0
```

Flags:

| Flag | Default | Description |
|---|---|---|
| `-addr` | `127.0.0.1:50051` | gRPC server address |
| `-ca` | (empty) | CA certificate; empty = insecure |
| `-service` | `android-client` | Service name (determines persistent MAC/IP) |
| `-tun` | `vswitch0` | TUN interface name |

The client auto-reconnects on disconnect. Requires root on Android for TUN access.
