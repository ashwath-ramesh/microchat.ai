package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	pb "microchat.ai/proto"
)

// LoadTestConfig holds configuration for the load test
type LoadTestConfig struct {
	ServerAddress   string
	ConcurrentUsers int
	MessagesPerUser int
	TestDuration    time.Duration
	SkipTLSVerify   bool   // DEPRECATED: Use CACertPath instead for production
	CACertPath      string // Path to CA certificate file for TLS verification
	APIKey          string
}

// LoadTestResults holds the results of a load test
type LoadTestResults struct {
	TotalRequests  int64
	SuccessfulReqs int64
	FailedReqs     int64
	MinLatency     time.Duration
	MaxLatency     time.Duration
	Latencies      []time.Duration // All successful request latencies for percentile calculation
	StartTime      time.Time
	EndTime        time.Time
	ErrorsByType   map[string]int64
}

// LoadTester manages the load testing
type LoadTester struct {
	config  LoadTestConfig
	results LoadTestResults
	mu      sync.Mutex
	model   pb.Model // Model to use for testing
}

// NewLoadTester creates a new load tester
func NewLoadTester(config LoadTestConfig) *LoadTester {
	return &LoadTester{
		config: config,
		results: LoadTestResults{
			ErrorsByType: make(map[string]int64),
			MinLatency:   time.Hour, // Initialize to a large value
		},
		model: pb.Model_ECHO, // Default model
	}
}

// NewLoadTesterWithModel creates a new load tester with a specific model
func NewLoadTesterWithModel(config LoadTestConfig, model pb.Model) *LoadTester {
	lt := NewLoadTester(config)
	lt.model = model
	return lt
}

// runUser simulates a single user's session
func (lt *LoadTester) runUser(ctx context.Context, userID int, wg *sync.WaitGroup) {
	defer wg.Done()

	// Create TLS credentials
	var creds credentials.TransportCredentials
	var err error
	if lt.config.CACertPath != "" {
		// Use custom CA certificate for verification
		creds, err = lt.createTLSCredentialsWithCA()
		if err != nil {
			lt.recordError(fmt.Sprintf("tls_setup_error: %v", err))
			return
		}
	} else if lt.config.SkipTLSVerify {
		// DEPRECATED: Only for development/testing with self-signed certs
		// In production, always use CACertPath for proper certificate verification
		creds = credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})
	} else {
		// Use system's root CA certificates
		creds = credentials.NewTLS(&tls.Config{})
	}

	// Connect to server
	conn, err := grpc.NewClient(lt.config.ServerAddress,
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		lt.recordError(fmt.Sprintf("connection_error: %v", err))
		return
	}
	defer conn.Close()

	client := pb.NewChatServiceClient(conn)

	// Start session with authentication
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+lt.config.APIKey)
	sessionResp, err := client.StartSession(ctx, &pb.StartSessionRequest{})
	if err != nil {
		lt.recordError(fmt.Sprintf("start_session_error: %v", err))
		return
	}
	sessionID := sessionResp.SessionId

	// Send messages
	for i := 0; i < lt.config.MessagesPerUser; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// programming-focused messages for CLI tool testing
		programmingMessages := []string{
			"Explain the difference between goroutines and threads in Go.",
			"How do I implement a binary search algorithm in Python?",
			"What's the best way to handle errors in Go?",
			"Show me how to create a REST API endpoint in Go.",
			"Explain Docker containers and their benefits for development.",
			"How do I optimize database queries for better performance?",
			"What are the principles of clean code architecture?",
			"How do I implement JWT authentication in a web API?",
		}
		message := programmingMessages[i%len(programmingMessages)]

		// Add authentication for each chat request
		chatCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+lt.config.APIKey)

		startTime := time.Now()
		resp, err := client.Chat(chatCtx, &pb.ChatRequest{
			SessionId: sessionID,
			Model:     lt.model, // Use the model specified for this tester
			Message:   message,
		})
		if err != nil {
			lt.recordError(fmt.Sprintf("chat_error: %v", err))
			continue
		}

		_ = resp // Use the response
		latency := time.Since(startTime)
		lt.recordSuccess(latency)

		// Add delay between messages to respect rate limits (10 RPS = 100ms between requests)
		time.Sleep(120 * time.Millisecond)
	}
}

// recordSuccess records a successful request
func (lt *LoadTester) recordSuccess(latency time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	atomic.AddInt64(&lt.results.TotalRequests, 1)
	atomic.AddInt64(&lt.results.SuccessfulReqs, 1)

	// Store individual latency for percentile calculation
	lt.results.Latencies = append(lt.results.Latencies, latency)

	if latency < lt.results.MinLatency {
		lt.results.MinLatency = latency
	}
	if latency > lt.results.MaxLatency {
		lt.results.MaxLatency = latency
	}
}

// createTLSCredentialsWithCA creates TLS credentials using a custom CA certificate
func (lt *LoadTester) createTLSCredentialsWithCA() (credentials.TransportCredentials, error) {
	// Read CA certificate file
	caCert, err := ioutil.ReadFile(lt.config.CACertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %v", err)
	}

	// Create certificate pool and add CA certificate
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	// Create TLS config with custom CA
	config := &tls.Config{
		RootCAs: caCertPool,
	}

	return credentials.NewTLS(config), nil
}

// calculatePercentile calculates the nth percentile from a sorted slice of durations
func calculatePercentile(sortedLatencies []time.Duration, percentile float64) time.Duration {
	if len(sortedLatencies) == 0 {
		return 0
	}

	index := (percentile / 100.0) * float64(len(sortedLatencies)-1)
	if index == float64(int(index)) {
		return sortedLatencies[int(index)]
	}

	// Interpolate between two values
	lower := int(index)
	upper := lower + 1
	if upper >= len(sortedLatencies) {
		return sortedLatencies[lower]
	}

	weight := index - float64(lower)
	lowerVal := float64(sortedLatencies[lower].Nanoseconds())
	upperVal := float64(sortedLatencies[upper].Nanoseconds())
	interpolated := lowerVal + weight*(upperVal-lowerVal)

	return time.Duration(interpolated)
}

// recordError records a failed request
func (lt *LoadTester) recordError(errorType string) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	atomic.AddInt64(&lt.results.TotalRequests, 1)
	atomic.AddInt64(&lt.results.FailedReqs, 1)
	lt.results.ErrorsByType[errorType]++
}

// Run executes the load test
func (lt *LoadTester) Run() LoadTestResults {
	ctx, cancel := context.WithTimeout(context.Background(), lt.config.TestDuration)
	defer cancel()

	lt.results.StartTime = time.Now()

	var wg sync.WaitGroup

	// Start concurrent users
	for i := 0; i < lt.config.ConcurrentUsers; i++ {
		wg.Add(1)
		go lt.runUser(ctx, i, &wg)

		// Small delay between starting users to avoid thundering herd
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for all users to finish
	wg.Wait()

	lt.results.EndTime = time.Now()
	return lt.results
}

// PrintResults prints the load test results
func (lt *LoadTester) PrintResults() {
	results := lt.results
	duration := results.EndTime.Sub(results.StartTime)

	fmt.Printf("\n=== Load Test Results ===\n")
	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Concurrent Users: %d\n", lt.config.ConcurrentUsers)
	fmt.Printf("Messages Per User: %d\n", lt.config.MessagesPerUser)
	fmt.Printf("\n--- Request Statistics ---\n")
	fmt.Printf("Total Requests: %d\n", results.TotalRequests)
	fmt.Printf("Successful: %d\n", results.SuccessfulReqs)
	fmt.Printf("Failed: %d\n", results.FailedReqs)
	fmt.Printf("Success Rate: %.2f%%\n", float64(results.SuccessfulReqs)/float64(results.TotalRequests)*100)

	if results.SuccessfulReqs > 0 {
		fmt.Printf("\n--- Latency Distribution ---\n")

		// Sort latencies for percentile calculation
		sortedLatencies := make([]time.Duration, len(results.Latencies))
		copy(sortedLatencies, results.Latencies)
		sort.Slice(sortedLatencies, func(i, j int) bool {
			return sortedLatencies[i] < sortedLatencies[j]
		})

		fmt.Printf("Min Latency: %v\n", results.MinLatency)
		fmt.Printf("P50 (Median): %v\n", calculatePercentile(sortedLatencies, 50))
		fmt.Printf("P90: %v\n", calculatePercentile(sortedLatencies, 90))
		fmt.Printf("P99: %v\n", calculatePercentile(sortedLatencies, 99))
		fmt.Printf("P99.9: %v\n", calculatePercentile(sortedLatencies, 99.9))
		fmt.Printf("Max Latency: %v\n", results.MaxLatency)

		throughput := float64(results.SuccessfulReqs) / duration.Seconds()
		fmt.Printf("Throughput: %.2f requests/second\n", throughput)
	}

	if len(results.ErrorsByType) > 0 {
		fmt.Printf("\n--- Error Breakdown ---\n")
		for errorType, count := range results.ErrorsByType {
			fmt.Printf("%s: %d\n", errorType, count)
		}
	}
}

// getServerAddress constructs server address from environment variables
func getServerAddress() string {
	host := os.Getenv("SERVER_NAME")
	if host == "" {
		host = "localhost"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "4000" // Default port from .env
	}

	return fmt.Sprintf("%s:%s", host, port)
}

// getCACertPath gets the CA certificate path from environment or defaults to certs/ca.crt
func getCACertPath() string {
	// Check environment variable first
	caCertPath := os.Getenv("CA_CERT_PATH")
	if caCertPath != "" {
		return caCertPath
	}

	// Check default locations relative to load test directory
	defaultPaths := []string{
		"../../certs/ca.crt", // From cmd/loadtest/
		"certs/ca.crt",       // From project root
		"./ca.crt",           // Current directory
	}

	for _, path := range defaultPaths {
		if _, err := os.Stat(path); err == nil {
			absPath, err := filepath.Abs(path)
			if err == nil {
				log.Printf("Using CA certificate: %s", absPath)
				return absPath
			}
		}
	}

	// No CA certificate found - will fall back to SkipTLSVerify if explicitly enabled
	log.Printf("No CA certificate found. Set CA_CERT_PATH environment variable or ensure certs/ca.crt exists.")
	log.Printf("For production use, always provide a CA certificate. Only use SKIP_TLS_VERIFY=true for development.")
	return ""
}

// getAPIKey gets the first API key from the API_KEYS environment variable
func getAPIKey() string {
	apiKeys := os.Getenv("API_KEYS")
	if apiKeys == "" {
		log.Fatal("API_KEYS environment variable not set")
	}

	// Parse comma-separated keys and return the first non-admin key
	keys := strings.Split(apiKeys, ",")
	for _, key := range keys {
		key = strings.TrimSpace(key)
		// Skip admin keys (those with :admin suffix)
		if !strings.Contains(key, ":admin") && key != "" {
			return key
		}
	}

	log.Fatal("No valid API keys found in API_KEYS")
	return "" // unreachable
}

// runLoadTestForModel runs a load test for a specific model
func runLoadTestForModel(config LoadTestConfig, model pb.Model, modelName string) bool {
	log.Printf("\n=== Testing %s Model ===", modelName)
	log.Printf("Starting load test against %s with %d concurrent users, %d messages each...",
		config.ServerAddress, config.ConcurrentUsers, config.MessagesPerUser)

	// Create a new tester with the specific model
	tester := NewLoadTesterWithModel(config, model)
	results := tester.Run()

	fmt.Printf("\n=== %s Model Results ===\n", modelName)
	tester.PrintResults()

	// Check failure rate
	failureRate := float64(results.FailedReqs) / float64(results.TotalRequests)
	if failureRate > 0.05 { // More than 5% failures
		log.Printf("%s model test failed with %.2f%% failure rate", modelName, failureRate*100)
		return false
	}

	log.Printf("%s model test completed successfully!", modelName)
	return true
}

// Example usage
func main() {
	// Load .env file - check current directory first, then project root
	if err := godotenv.Load(".env"); err != nil {
		if err := godotenv.Load("../../.env"); err != nil {
			log.Printf("no .env file found, using environment variables only")
		}
	}

	config := LoadTestConfig{
		ServerAddress:   getServerAddress(),
		ConcurrentUsers: 5, // Reduced from 10 to respect rate limits
		MessagesPerUser: 3, // Reduced from 5 to avoid overwhelming server
		TestDuration:    30 * time.Second,
		CACertPath:      getCACertPath(),                                                 // Use CA certificate for proper TLS verification
		SkipTLSVerify:   getCACertPath() == "" && os.Getenv("SKIP_TLS_VERIFY") == "true", // Only skip TLS verification if no CA cert and explicitly requested
		APIKey:          getAPIKey(),
	}

	// Test both models
	models := []struct {
		model pb.Model
		name  string
	}{
		{pb.Model_ECHO, "ECHO"},
		{pb.Model_GEMINI_2_5_FLASH_LITE, "GEMINI_2_5_FLASH_LITE"},
	}

	allSuccess := true
	for i, modelTest := range models {
		success := runLoadTestForModel(config, modelTest.model, modelTest.name)
		if !success {
			allSuccess = false
		}

		// Add delay between model tests to avoid rate limiting (skip after last test)
		if i < len(models)-1 {
			log.Printf("Waiting 2 seconds before next model test...")
			time.Sleep(2 * time.Second)
		}
	}

	fmt.Printf("\n=== Overall Results ===\n")
	if allSuccess {
		log.Println("All model tests completed successfully!")
	} else {
		log.Println("Some model tests failed.")
	}
}
