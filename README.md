# 🌌 Pulsar

**High-performance synthetic event generator**
Generate realistic logs and network flows at scale up to **150000 events per second**

> ⚠️ **Pre-alpha software** — not ready for production use. Breaking changes are expected.


## Features

- **Protocols**: TCP, UDP
- **Formats**: NetFlow v5, Syslog (CEF)
- **Lock-free pipeline engine**: Zero-lock worker pools with atomic queues for sustained 150K+ EPS
- **CLI-first**: Optionla config files - everything tunable via flags or env vars. 
Perfect for scripts and CI
- **Built-in observability**: Metrics for throughput, drops, and latency out of the box


## Known problems

- TCP connection pool is basic (no smart retries; errors on shutdown)
- Event types and data are hardcoded - no support for custom format 
- Potential data loss on shutdown under high load (visible with -race flag)
- Incomplete metrics (network/processing stats not fully implemented) 
- Suboptimal memory usage at high EPS (OOM risk on constrained systems) 
- No build-time information (`-v` shows placeholder)

## Usage
### Run from source (for development)
```sh
go run ./cmd --help
```

### Build and run binary
```sh
go build -o pulsar .
./pulsar -e netflow -p udp -r 100000 -d localhost:2055 -t 10m
```

### Flags
| Flag | Type | Description | Default |
|------|------|-------------|---------|
| `-c, --config` | `FILE` | Configuration file path | - |
| `-r, --rate` | `N` | Events per second | `10` |
| `-d, --destinations` | `LIST` | Comma-separated destinations (now can work only with single) | `127.0.0.1:514` |
| `-p, --protocol` | `PROTO` | Protocol: `tcp` or `udp` | `udp` |
| `-e, --events` | `TYPE` | Events type: `netflow` or `syslog` | `netflow` |
| `-t, --duration` | `TIME` | How long to run (e.g., `30s`, `5m`, `1h`) | `0` (endless) |
| `-l, --log-level` | `LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `info` |
| `-v, --version` | - | Show version information | - |
| `-h, --help` | - | Show this help | - |

### Docker
> ⚠️**Wrk in progress** docker-compose is just an exampl and not optimal. Public image and stable version coming soon.

You can run multiple generators via `docker-compose.yml` (netflow + syslog, as example)

Build and run both generators for 5 minutes:
```sh 
docker-compose up --build
```

Send events to your destination instead of localhost:
```sh
TARGET_DESTINATION=siem.internal:2055 docker-compose up --build
```

Run only one generator (for example netflow):
```sh
docker-compose up --build pulsar-netflow
```


## Issues

Found a bug or have an idea? Open an issue!  
Please include:
- Command you ran  
- Expected vs actual behavior  

Pre-alpha means **bugs are expected** — your report helps a lot!


## Maintainer

This project was created and is maintained by **[Nikita Shabanov](https://github.com/nashabanov)**.
