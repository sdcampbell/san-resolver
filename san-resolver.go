package main

import (
	"bufio"
	"context"
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

type DNSRequest struct {
	line       string
	expectedIP string
	domain     string
}

type DNSResult struct {
	line      string
	shouldPrint bool
}

func main() {
	// Regular expression to parse the input format: IP:PORT [DOMAIN]
	re := regexp.MustCompile(`^(\d+\.\d+\.\d+\.\d+):(\d+)\s+\[([^\]]+)\]`)
	
	// Channels for communication
	inputChan := make(chan DNSRequest, InputBufferSize)
	outputChan := make(chan DNSResult, InputBufferSize)
	
	// Create worker pool for DNS lookups
	var wg sync.WaitGroup
	for i := 0; i < NumWorkers; i++ {
		wg.Add(1)
		go dnsWorker(inputChan, outputChan, &wg)
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
				case outputChan <- DNSResult{line: line, shouldPrint: true}:
				case <-time.After(time.Second):
					// If output buffer is full, print directly to avoid blocking
					fmt.Println(line)
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
				result := processDNSRequest(request)
				select {
				case outputChan <- result:
				case <-time.After(time.Second):
					if result.shouldPrint {
						fmt.Println(result.line)
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

func dnsWorker(inputChan <-chan DNSRequest, outputChan chan<- DNSResult, wg *sync.WaitGroup) {
	defer wg.Done()
	
	for request := range inputChan {
		result := processDNSRequest(request)
		
		select {
		case outputChan <- result:
		case <-time.After(time.Second):
			// If output buffer is full, print directly
			if result.shouldPrint {
				fmt.Println(result.line)
			}
		}
	}
}

func processDNSRequest(request DNSRequest) DNSResult {
	// Create context with timeout for DNS lookup
	ctx, cancel := context.WithTimeout(context.Background(), DNSTimeout)
	defer cancel()
	
	// Create a custom resolver with timeout
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: DNSTimeout,
			}
			return d.DialContext(ctx, network, address)
		},
	}
	
	// Resolve the domain to IP addresses
	ips, err := resolver.LookupIPAddr(ctx, request.domain)
	if err != nil {
		// DNS resolution failed, should print the line
		return DNSResult{line: request.line, shouldPrint: true}
	}
	
	// Check if any of the resolved IPs match the expected IP
	for _, ip := range ips {
		if ip.IP.String() == request.expectedIP {
			// Found match, don't print
			return DNSResult{line: request.line, shouldPrint: false}
		}
	}
	
	// Expected IP was NOT found in DNS resolution, should print the line
	return DNSResult{line: request.line, shouldPrint: true}
}

func outputWorker(outputChan <-chan DNSResult, done chan<- bool) {
	defer func() { done <- true }()
	
	for result := range outputChan {
		if result.shouldPrint {
			fmt.Println(result.line)
		}
	}
}