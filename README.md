# san-resolver
Takes input from tlsx output and checks if certificate SAN resolves to the IP address. If it doesn't resolve to the IP address, print the input line.

## Compilation

### Prerequisites
- Go 1.16 or later installed

### Build Commands

```bash
# Install from GitHub URL
go install github.com/sdcampbell/san-resolver@latest

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

Note: You MUST use the `-nc` flag with tlsx to remove color codes because it breaks san-resolver.

```
$ wget -q -O - https://raw.githubusercontent.com/arkadiyt/bounty-targets-data/refs/heads/main/data/bugcrowd_data.json | jq '.[].targets.in_scope[] | select(.type == "network") | .target' | grep -E '([0-9]{1,3}\.){3}[0-9]{1,3}' | sed 's/"//g' | tlsx -san -silent -nc | ./san-resolver
203.13.127.195:443 [www.optus.com.au] CDN_MISMATCH_AKAMAI 23.212.249.212[a23-212-249-212.deploy.static.akamaitechnologies.com],23.212.249.206[a23-212-249-206.deploy.static.akamaitechnologies.com]
203.13.127.252:443 [www.gomo.com.au] CDN_MISMATCH_AKAMAI 2600:1408:c400:4d::1749:cf45[g2600-1408-c400-004d-0000-0000-1749-cf45.deploy.static.akamaitechnologies.com],2600:1408:c400:4d::1749:cf49[g2600-1408-c400-004d-0000-0000-1749-cf49.deploy.static.akamaitechnologies.com],23.212.249.212[a23-212-249-212.deploy.static.akamaitechnologies.com]
203.13.127.195:443 [optus.com.au] CDN_MISMATCH_AKAMAI 23.48.247.241[a23-48-247-241.deploy.static.akamaitechnologies.com],23.48.247.242[a23-48-247-242.deploy.static.akamaitechnologies.com]
203.13.127.195:443 [www.optus.com] CDN_MISMATCH_AKAMAI 23.212.249.206[a23-212-249-206.deploy.static.akamaitechnologies.com],23.212.249.212[a23-212-249-212.deploy.static.akamaitechnologies.com]
203.13.127.30:443 [ww2.optus.com.au] DNS_FAILURE
203.13.127.30:443 [www.ww2.optus.com.au] DNS_FAILURE
203.13.129.74:443 [www.colesmobile.com.au] CDN_MISMATCH_AWS_GLOBAL_ACCELERATOR 75.2.57.205[aa82e5f6a2d228f90.awsglobalaccelerator.com]
203.13.129.74:443 [colesmobile.com.au] CDN_MISMATCH_AWS_GLOBAL_ACCELERATOR 75.2.57.205[aa82e5f6a2d228f90.awsglobalaccelerator.com]
203.16.76.99:443 [optus.com.au] CDN_MISMATCH_AKAMAI 23.48.247.242[a23-48-247-242.deploy.static.akamaitechnologies.com],23.48.247.241[a23-48-247-241.deploy.static.akamaitechnologies.com]
203.13.128.211:443 [adfsauth.optus.com.au] IP_MISMATCH 203.13.128.216
203.13.128.216:443 [adfs.optus.com.au] IP_MISMATCH 203.13.128.211
203.16.76.99:443 [www.cfs2.au.singtelgroup.net] DNS_FAILURE
203.13.128.211:443 [enterpriseregistration.optus.com.au] DNS_FAILURE
203.13.128.216:443 [enterpriseregistration.optus.com.au] DNS_FAILURE
203.16.76.99:443 [cfs2.au.singtelgroup.net] DNS_FAILURE
203.13.128.215:443 [www.partner.connect.optus.com.au] DNS_FAILURE
203.13.128.216:443 [www.adfs.optus.com.au] DNS_FAILURE
203.13.128.211:443 [www.adfs.optus.com.au] DNS_FAILURE
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

