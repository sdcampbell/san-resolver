# san-resolver
Takes input from tlsx output and checks if certificate SAN resolves to the IP address. If it doesn't resolve to the IP address, print the input line.

## Compilation

### Prerequisites
- Go 1.16 or later installed

### Build Commands

```bash
# Standard compilation
go build -o san-resolver san-resolver.go

# Optimized build (smaller binary, faster execution)
go build -ldflags="-w -s" -o san-resolver san-resolver.go

# Static binary (no external dependencies)
CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-w -s -extldflags '-static'" -o san-resolver san-resolver.go

# Cross-compilation for different architectures
GOOS=linux GOARCH=amd64 go build -o san-resolver-amd64 san-resolver.go
GOOS=linux GOARCH=arm64 go build -o san-resolver-arm64 san-resolver.go
```

### Make Executable
```bash
chmod +x san-resolver
```

## Usage

```
$ wget -q -O - https://raw.githubusercontent.com/arkadiyt/bounty-targets-data/refs/heads/main/data/bugcrowd_data.json | jq '.[].targets.in_scope[] | select(.type == "network") | .target' | grep -E '([0-9]{1,3}\.){3}[0-9]{1,3}' | sed 's/"//g' | tlsx -san -silent -c 50 -delay 3s | ./san-resolver
203.13.127.252:443 [gomo.com.au] DNS_FAILURE
203.13.127.252:443 [www.gomo.com.au] DNS_FAILURE
203.13.127.195:443 [www.optus.com.au] DNS_FAILURE
203.13.127.195:443 [www.optus.com] DNS_FAILURE
203.13.127.195:443 [optus.com.au] DNS_FAILURE
203.13.127.195:443 [optus.com] DNS_FAILURE
203.13.127.30:443 [www.ww2.optus.com.au] DNS_FAILURE
203.13.127.30:443 [ww2.optus.com.au] DNS_FAILURE
203.13.129.74:443 [colesmobile.com.au] DNS_FAILURE
203.13.129.74:443 [www.colesmobile.com.au] DNS_FAILURE
203.13.98.23:443 [ole.connect.optus.com.au] DNS_FAILURE
203.13.128.73:443 [service-api.optus.com.au] DNS_FAILURE
203.13.128.216:443 [adfs.optus.com.au] DNS_FAILURE
203.13.128.216:443 [www.adfs.optus.com.au] DNS_FAILURE
203.13.128.216:443 [adfsauth.optus.com.au] DNS_FAILURE
203.13.128.216:443 [enterpriseregistration.optus.com.au] DNS_FAILURE
203.13.128.211:443 [adfs.optus.com.au] DNS_FAILURE
203.13.128.211:443 [adfsauth.optus.com.au] DNS_FAILURE
203.13.128.211:443 [www.adfs.optus.com.au] DNS_FAILURE
203.13.128.211:443 [enterpriseregistration.optus.com.au] DNS_FAILURE
203.13.128.215:443 [partner.connect.optus.com.au] DNS_FAILURE
203.13.128.215:443 [www.partner.connect.optus.com.au] DNS_FAILURE
203.16.76.100:443 [cfs2auth.optus.com.au] DNS_FAILURE
203.16.76.102:443 [perappvpn.optus.com.au] DNS_FAILURE
203.16.76.99:443 [www.cfs2.au.singtelgroup.net] DNS_FAILURE
203.16.76.99:443 [cfs2.optus.com.au] DNS_FAILURE
203.16.76.99:443 [optus.com.au] DNS_FAILURE
203.16.76.99:443 [cfs2.au.singtelgroup.net] DNS_FAILURE
```

### Performance Tuning

The program includes several configurable constants for performance optimization:

```go
NumWorkers = 50          // Concurrent DNS workers (increase for more throughput)
InputBufferSize = 1000   // Input queue size (increase for bursty input)
DNSTimeout = 5 * time.Second  // DNS lookup timeout (adjust for network conditions)
```

To modify these values, edit the constants in `san-resolver.go` and recompile.

### Output Behavior

- **Silent Success**: Lines where DNS resolution matches the expected IP are not printed
- **Mismatch Detection**: Lines where DNS doesn't resolve to the expected IP are printed
- **Error Handling**: Malformed input or DNS resolution failures result in the line being printed

### Troubleshooting

- **High Memory Usage**: Reduce `NumWorkers` and `InputBufferSize` constants
- **Slow Performance**: Increase `NumWorkers` or decrease `DNSTimeout`
- **DNS Timeouts**: Increase `DNSTimeout` for slow networks
- **Large Input Files**: Use streaming with `cat large_file.txt | ./san-resolver`

