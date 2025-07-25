package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// Number of concurrent DNS lookup workers
	NumWorkers = 50
	// Buffer size for input queue
	InputBufferSize = 1000
	// Timeout for DNS lookups
	DNSTimeout = 5 * time.Second
)

// Known CDN IP ranges and ASNs for detection
var cdnProviders = map[string][]string{
	"cloudflare": {
		"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22",
		"103.31.4.0/22", "141.101.64.0/18", "108.162.192.0/18",
		"190.93.240.0/20", "188.114.96.0/20", "197.234.240.0/22",
		"198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
		"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	},
	"cloudfront": {
		"52.84.0.0/15", "54.230.0.0/16", "54.239.128.0/18",
		"99.84.0.0/16", "205.251.192.0/19", "54.239.192.0/19",
		"70.132.0.0/18", "13.32.0.0/15", "13.35.0.0/16",
		"204.246.164.0/22", "204.246.168.0/22", "71.152.0.0/17",
	},
	"aws_global_accelerator": {
		"75.2.0.0/16", "99.77.0.0/16", "99.83.0.0/16",
		"108.136.0.0/13", "130.176.0.0/12", "150.222.0.0/16",
		"15.177.0.0/18", "52.93.0.0/16", "54.239.0.0/16",
	},
	"fastly": {
		"23.235.32.0/20", "43.249.72.0/22", "103.244.50.0/24",
		"103.245.222.0/23", "103.245.224.0/24", "104.156.80.0/20",
		"140.248.64.0/18", "140.248.128.0/17", "146.75.0.0/16",
		"151.101.0.0/16", "157.52.64.0/18", "167.82.0.0/17",
		"167.82.128.0/20", "167.82.160.0/20", "167.82.224.0/20",
		"172.111.64.0/18", "185.31.16.0/22", "199.27.72.0/21",
		"199.232.0.0/16",
	},
	"akamai": {
		"23.0.0.0/12", "2.16.0.0/13", "23.192.0.0/11", "23.32.0.0/11",
		"23.64.0.0/14", "23.72.0.0/13", "96.16.0.0/15", "96.6.0.0/15",
		"104.64.0.0/10", "184.24.0.0/13", "184.50.0.0/15", "184.84.0.0/14",
		"172.224.0.0/12", "172.240.0.0/13",
	},
}

type DNSRequest struct {
	line       string
	expectedIP string
	domain     string
}

type DNSResult struct {
	line        string
	shouldPrint bool
	status      string
	resolvedIPs []string // Now stores formatted "IP[hostname]" or "IP" strings
}

func main() {
	// Command line flags for DNS configuration
	var (
		workers     = flag.Int("workers", NumWorkers, "Number of concurrent DNS workers")
		bufferSize  = flag.Int("buffer", InputBufferSize, "Input buffer size")
		dnsTimeout  = flag.Duration("timeout", DNSTimeout, "DNS lookup timeout")
		forceGoogle = flag.Bool("force-google", false, "Force Google DNS (8.8.8.8) only")
		forceCF     = flag.Bool("force-cloudflare", false, "Force Cloudflare DNS (1.1.1.1) only")
		noSystemDNS = flag.Bool("no-system-dns", false, "Skip system DNS resolver")
		verbose     = flag.Bool("v", false, "Verbose output (show which DNS strategy worked)")
	)
	flag.Parse()

	// Validate mutually exclusive options
	if *forceGoogle && *forceCF {
		fmt.Fprintf(os.Stderr, "Error: Cannot use both -force-google and -force-cloudflare\n")
		os.Exit(1)
	}

	// Regular expression to parse the input format: IP:PORT [DOMAIN]
	re := regexp.MustCompile(`^(\d+\.\d+\.\d+\.\d+):(\d+)\s+\[([^\]]+)\]`)
	
	// Channels for communication
	inputChan := make(chan DNSRequest, *bufferSize)
	outputChan := make(chan DNSResult, *bufferSize)
	
	// Create worker pool for DNS lookups
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go dnsWorker(inputChan, outputChan, &wg, *dnsTimeout, *forceGoogle, *forceCF, *noSystemDNS, *verbose)
	}
	
	// Output worker to print results
	outputDone := make(chan bool)
	go outputWorker(outputChan, outputDone)
	
	// Read input asynchronously
	scanner := bufio.NewScanner(os.Stdin)
	inputCount := 0
	
	go func() {
		defer close(inputChan)
		
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			
			// Skip empty lines
			if line == "" {
				continue
			}
			
			// Parse the input line
			matches := re.FindStringSubmatch(line)
			if len(matches) != 4 {
				// If line doesn't match expected format, queue it for printing
				select {
				case outputChan <- DNSResult{line: line, shouldPrint: true, status: "MALFORMED", resolvedIPs: nil}:
				case <-time.After(time.Second):
					// If output buffer is full, print directly to avoid blocking
					fmt.Printf("%s MALFORMED\n", line)
				}
				continue
			}
			
			expectedIP := matches[1]
			domain := matches[3]
			
			// Send to workers for processing
			request := DNSRequest{
				line:       line,
				expectedIP: expectedIP,
				domain:     domain,
			}
			
			select {
			case inputChan <- request:
				inputCount++
			case <-time.After(time.Second):
				// If input buffer is full, process inline to avoid blocking
				result := processDNSRequest(request, *dnsTimeout, *forceGoogle, *forceCF, *noSystemDNS, *verbose)
				select {
				case outputChan <- result:
				case <-time.After(time.Second):
					if result.shouldPrint {
						if len(result.resolvedIPs) > 0 {
							fmt.Printf("%s %s %s\n", result.line, result.status, strings.Join(result.resolvedIPs, ","))
						} else {
							fmt.Printf("%s %s\n", result.line, result.status)
						}
					}
				}
			}
		}
		
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		}
	}()
	
	// Wait for all workers to finish processing
	wg.Wait()
	close(outputChan)
	
	// Wait for output worker to finish
	<-outputDone
}

func dnsWorker(inputChan <-chan DNSRequest, outputChan chan<- DNSResult, wg *sync.WaitGroup, dnsTimeout time.Duration, forceGoogle, forceCF, noSystemDNS, verbose bool) {
	defer wg.Done()
	
	for request := range inputChan {
		result := processDNSRequest(request, dnsTimeout, forceGoogle, forceCF, noSystemDNS, verbose)
		
		select {
		case outputChan <- result:
		case <-time.After(time.Second):
			// If output buffer is full, print directly
			if result.shouldPrint {
				if len(result.resolvedIPs) > 0 {
					fmt.Printf("%s %s %s\n", result.line, result.status, strings.Join(result.resolvedIPs, ","))
				} else {
					fmt.Printf("%s %s\n", result.line, result.status)
				}
			}
		}
	}
}

func processDNSRequest(request DNSRequest, dnsTimeout time.Duration, forceGoogle, forceCF, noSystemDNS, verbose bool) DNSResult {
	// Create context with configurable timeout for DNS lookup
	ctx, cancel := context.WithTimeout(context.Background(), dnsTimeout*3) // 3x timeout for retries
	defer cancel()
	
	// Build DNS resolution strategies based on flags
	var strategies []func(context.Context, string) ([]net.IPAddr, error)
	
	if forceGoogle {
		// Only use Google DNS
		strategies = []func(context.Context, string) ([]net.IPAddr, error){
			func(ctx context.Context, domain string) ([]net.IPAddr, error) {
				resolver := &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
						d := net.Dialer{Timeout: dnsTimeout}
						return d.DialContext(ctx, network, "8.8.8.8:53")
					},
				}
				return resolver.LookupIPAddr(ctx, domain)
			},
		}
	} else if forceCF {
		// Only use Cloudflare DNS
		strategies = []func(context.Context, string) ([]net.IPAddr, error){
			func(ctx context.Context, domain string) ([]net.IPAddr, error) {
				resolver := &net.Resolver{
					PreferGo: true,
					Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
						d := net.Dialer{Timeout: dnsTimeout}
						return d.DialContext(ctx, network, "1.1.1.1:53")
					},
				}
				return resolver.LookupIPAddr(ctx, domain)
			},
		}
	} else {
		// Multiple DNS resolution strategies to handle caching/config issues
		if !noSystemDNS {
			// Strategy 1: System default resolver
			strategies = append(strategies, func(ctx context.Context, domain string) ([]net.IPAddr, error) {
				return net.DefaultResolver.LookupIPAddr(ctx, domain)
			})
			
			// Strategy 2: Force Go's built-in resolver (bypasses system DNS)
			strategies = append(strategies, func(ctx context.Context, domain string) ([]net.IPAddr, error) {
				resolver := &net.Resolver{PreferGo: true}
				return resolver.LookupIPAddr(ctx, domain)
			})
		}
		
		// Strategy 3: Google DNS (8.8.8.8)
		strategies = append(strategies, func(ctx context.Context, domain string) ([]net.IPAddr, error) {
			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{Timeout: dnsTimeout}
					return d.DialContext(ctx, network, "8.8.8.8:53")
				},
			}
			return resolver.LookupIPAddr(ctx, domain)
		})
		
		// Strategy 4: Cloudflare DNS (1.1.1.1)
		strategies = append(strategies, func(ctx context.Context, domain string) ([]net.IPAddr, error) {
			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{Timeout: dnsTimeout}
					return d.DialContext(ctx, network, "1.1.1.1:53")
				},
			}
			return resolver.LookupIPAddr(ctx, domain)
		})
		
		// Strategy 5: Quad9 DNS (9.9.9.9) - security-focused DNS
		strategies = append(strategies, func(ctx context.Context, domain string) ([]net.IPAddr, error) {
			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{Timeout: dnsTimeout}
					return d.DialContext(ctx, network, "9.9.9.9:53")
				},
			}
			return resolver.LookupIPAddr(ctx, domain)
		})
		
		// Strategy 6: OpenDNS (208.67.222.222)
		strategies = append(strategies, func(ctx context.Context, domain string) ([]net.IPAddr, error) {
			resolver := &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
					d := net.Dialer{Timeout: dnsTimeout}
					return d.DialContext(ctx, network, "208.67.222.222:53")
				},
			}
			return resolver.LookupIPAddr(ctx, domain)
		})
	}
	
	var ips []net.IPAddr
	var err error
	var successfulStrategy int = -1
	
	// Try each strategy until one succeeds
	for i, strategy := range strategies {
		ips, err = strategy(ctx, request.domain)
		if err == nil && len(ips) > 0 {
			successfulStrategy = i
			break
		}
		// Small delay between attempts to avoid overwhelming DNS servers
		time.Sleep(50 * time.Millisecond)
	}
	
	// If all strategies failed, try one more time with LookupHost as fallback
	if err != nil {
		var hosts []string
		hosts, err = net.LookupHost(request.domain)
		if err == nil && len(hosts) > 0 {
			// Convert string IPs to IPAddr
			for _, host := range hosts {
				if ip := net.ParseIP(host); ip != nil {
					ips = append(ips, net.IPAddr{IP: ip})
				}
			}
			successfulStrategy = len(strategies) // Indicate fallback was used
		}
	}
	
	if err != nil || len(ips) == 0 {
		// All DNS resolution attempts failed
		return DNSResult{
			line:        request.line,
			shouldPrint: true,
			status:      "DNS_FAILURE",
			resolvedIPs: nil,
		}
	}
	
	// Convert resolved IPs to strings and check for matches
	var resolvedIPStrings []string
	var foundMatch bool
	
	for _, ip := range ips {
		ipStr := ip.IP.String()
		resolvedIPStrings = append(resolvedIPStrings, ipStr)
		if ipStr == request.expectedIP {
			foundMatch = true
		}
	}
	
	if foundMatch {
		// Found match, don't print
		return DNSResult{
			line:        request.line,
			shouldPrint: false,
			status:      "MATCH",
			resolvedIPs: resolvedIPStrings,
		}
	}
	
	// Expected IP was NOT found - determine if it's CDN or regular mismatch
	cdnProvider := detectCDN(resolvedIPStrings)
	status := "IP_MISMATCH"
	if cdnProvider != "" {
		status = fmt.Sprintf("CDN_MISMATCH_%s", strings.ToUpper(cdnProvider))
	}
	
	// Add strategy indicator for debugging if verbose mode is enabled
	if verbose {
		strategyNames := []string{"system", "go-builtin", "google", "cloudflare", "quad9", "opendns", "fallback"}
		if successfulStrategy >= 0 && successfulStrategy < len(strategyNames) {
			status = fmt.Sprintf("%s_VIA_%s", status, strings.ToUpper(strategyNames[successfulStrategy]))
		}
	}
	
	// Perform reverse DNS lookups for better intelligence
	formattedIPs := performReverseLookups(net.DefaultResolver, ctx, resolvedIPStrings)
	
	return DNSResult{
		line:        request.line,
		shouldPrint: true,
		status:      status,
		resolvedIPs: formattedIPs,
	}
}

func performReverseLookups(resolver *net.Resolver, ctx context.Context, ips []string) []string {
	type reverseResult struct {
		ip       string
		hostname string
	}
	
	// Channel to collect reverse lookup results
	results := make(chan reverseResult, len(ips))
	
	// Perform reverse lookups concurrently
	for _, ip := range ips {
		go func(ipAddr string) {
			// Create a shorter timeout context for reverse lookups
			reverseCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			
			hostnames, err := resolver.LookupAddr(reverseCtx, ipAddr)
			if err != nil || len(hostnames) == 0 {
				// No reverse DNS or lookup failed
				results <- reverseResult{ip: ipAddr, hostname: ""}
			} else {
				// Use the first hostname, remove trailing dot if present
				hostname := strings.TrimSuffix(hostnames[0], ".")
				results <- reverseResult{ip: ipAddr, hostname: hostname}
			}
		}(ip)
	}
	
	// Collect results
	reverseMap := make(map[string]string)
	for i := 0; i < len(ips); i++ {
		select {
		case result := <-results:
			reverseMap[result.ip] = result.hostname
		case <-ctx.Done():
			// Timeout - use remaining IPs without hostnames
			break
		}
	}
	
	// Format results as "IP[hostname]" or "IP"
	var formattedIPs []string
	for _, ip := range ips {
		if hostname, exists := reverseMap[ip]; exists && hostname != "" {
			formattedIPs = append(formattedIPs, fmt.Sprintf("%s[%s]", ip, hostname))
		} else {
			formattedIPs = append(formattedIPs, ip)
		}
	}
	
	return formattedIPs
}

func detectCDN(ips []string) string {
	for _, ip := range ips {
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			continue
		}
		
		for provider, cidrs := range cdnProviders {
			for _, cidr := range cidrs {
				_, ipnet, err := net.ParseCIDR(cidr)
				if err != nil {
					continue
				}
				if ipnet.Contains(parsedIP) {
					return provider
				}
			}
		}
	}
	return ""
}

func outputWorker(outputChan <-chan DNSResult, done chan<- bool) {
	defer func() { done <- true }()
	
	for result := range outputChan {
		if result.shouldPrint {
			if len(result.resolvedIPs) > 0 {
				fmt.Printf("%s %s %s\n", result.line, result.status, strings.Join(result.resolvedIPs, ","))
			} else {
				fmt.Printf("%s %s\n", result.line, result.status)
			}
		}
	}
}
