# san-resolver
san-resolver accepts input from [tlsx](https://github.com/projectdiscovery/tlsx) output and checks if the certificate Subject Alternative Name (SAN) resolves to the input IP address. If it doesn't resolve to that IP address, print the input line.

This tool is published for use in authorized penetration testing only.

A “vhost,” short for “virtual host,” is a configuration used in web servers to host multiple websites (domains) on a single server or IP address. It allows a server to serve different content based on the request’s hostname, effectively enabling multiple domains to share the same server resources. When you request a web application by IP address in your browser or other tool, the server responds with the default web application. There may be additional web applications to discover when presented with the correct `Host` header. This is a problem for penetration testers and bug bounty hunters when given a scope of IP or network addresses. 

The output of this tool may signify one of the following:

1. `CDN_MISMATCH_*`: The vhost resolves to a known CDN provider IP address and you've found an unprotected CDN origin server.
2. `DNS_FAILURE`: The vhost does not resolve to any IP address. The organization may be resolving the hostname only on internal DNS servers, or the application may no longer be in service and the owner forgot to remove it from the Internet, or other reasons.
3. `IP_MISMATCH`: The vhost resolves to a different IP address. This may be a development or staging server and could have less protection than the production server.

This information may allow you to bypass the CDN WAF and test the origin server directly by adding an entry to your `/etc/hosts` file, or specifying a HOST header with various security testing tools. This methodology has enabled the author to detect unprotected CDN origin servers and old, forgotten applications during external penetration tests.

## Compilation

### Prerequisites
- Go 1.16 or later installed

### Installation

```bash
# Install from GitHub URL
go install github.com/sdcampbell/san-resolver@latest
```

## Usage

Note: san-resolver is designed to accept input only from piped [tlsx](https://github.com/projectdiscovery/tlsx) uncolored (`-nc`) input. Normally you don't need to specify any of the following CLI arguments:

```bash
san-resolver --help
Usage of san-resolver:
  -buffer int
    	Input buffer size (default 1000)
  -force-cloudflare
    	Force Cloudflare DNS (1.1.1.1) only
  -force-google
    	Force Google DNS (8.8.8.8) only
  -no-system-dns
    	Skip system DNS resolver
  -timeout duration
    	DNS lookup timeout (default 5s)
  -v	Verbose output (show which DNS strategy worked)
  -workers int
    	Number of concurrent DNS workers (default 50
```

This example shows uncovering web applications that would not have been apparent if you had scanned the IP or network address due to the way load balancers and proxies work. In the example, san-resolver identifies unprotected CDN origin servers. Now you can add these to your hosts file and bypass the CDN WAF.

These IP addresses were in scope on Bugcrowd at the time this scan was run.

```
$ cat ips.txt | tlsx -san -silent -nc | san-resolver
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

