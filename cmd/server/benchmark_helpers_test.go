package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"microchat.ai/cmd/server/llm"
	pb "microchat.ai/proto"
)

// setupBenchApp creates an application instance for benchmarking
func setupBenchApp() (*application, *llm.MockProvider) {
	printSystemInfo() // Show system info once per benchmark run
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mockProvider := llm.NewMockProvider("Bench-Provider")
	
	app := &application{
		logger:       logger,
		sessionStore: NewSessionStore(time.Hour, 10000, 100000, 100*1024*1024), // More generous limits
		providerFactory: func(model pb.Model, logger *slog.Logger) llm.Provider {
			return mockProvider
		},
	}
	
	return app, mockProvider
}

// createSession helper creates a new session and returns its ID
func createSession(app *application) (string, error) {
	resp, err := app.StartSession(context.Background(), &pb.StartSessionRequest{})
	if err != nil {
		return "", err
	}
	return resp.SessionId, nil
}

// generateRealisticMessage creates messages of varying realistic sizes
func generateRealisticMessage(size string, index int) string {
	switch size {
	case "small": // 100-200 chars - typical chat
		return fmt.Sprintf("Hey, I have a question about implementing feature #%d. Can you help me understand the best approach for this use case?", index)
	case "medium": // 1-2KB - code discussion
		base := fmt.Sprintf("I'm working on implementing a new feature and running into some issues. Here's what I'm trying to do: %d. ", index)
		filler := "The main challenge is handling concurrent access while maintaining data consistency. I've considered using mutexes but I'm wondering if there's a more elegant solution using channels or atomic operations. The performance implications are also important since this code will be called frequently in the hot path. What would you recommend considering the trade-offs between complexity and performance? I've seen some approaches that use sync.RWMutex but I'm not sure if that's overkill for this use case. "
		return base + strings.Repeat(filler, 3)
	case "large": // 5-10KB - code snippets
		code := `func (s *Server) HandleRequest(ctx context.Context, req *Request) (*Response, error) {
	// Validate input
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}
	
	// Check rate limiting
	if !s.rateLimiter.Allow() {
		return nil, ErrRateLimited
	}
	
	// Process request with timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	
	result, err := s.processor.Process(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to process: %w", err)
	}
	
	return &Response{Data: result}, nil
}`
		return fmt.Sprintf("Here's the code I'm having trouble with (iteration %d):\n\n%s\n\nThe issue is that this sometimes hangs under high load. I suspect it might be related to the context handling or the rate limiter. Can you help me debug this?", index, strings.Repeat(code, 2))
	default:
		return fmt.Sprintf("Test message %d", index)
	}
}


// System info display for benchmarks
var systemInfoOnce sync.Once

func printSystemInfo() {
	systemInfoOnce.Do(func() {
		fmt.Println("=== System Information ===")
		
		// CPU Info
		cpuInfo := getCPUInfo()
		fmt.Printf("CPU: %s\n", cpuInfo)
		
		// Memory Info  
		memInfo := getMemoryInfo()
		fmt.Printf("Memory: %s\n", memInfo)
		
		// OS Info
		fmt.Printf("OS: %s %s (%s)\n", getOSName(), getOSVersion(), runtime.GOOS)
		
		// Go Info
		fmt.Printf("Go: %s (GOMAXPROCS=%d)\n", runtime.Version(), runtime.GOMAXPROCS(0))
		
		fmt.Println("===========================")
	})
}

func getCPUInfo() string {
	switch runtime.GOOS {
	case "darwin":
		if brand := sysctlString("machdep.cpu.brand_string"); brand != "" {
			cores := sysctlInt("hw.ncpu")
			return fmt.Sprintf("%s (%d-core, %s)", brand, cores, runtime.GOARCH)
		}
	case "linux":
		if info := readProcCPUInfo(); info != "" {
			return info
		}
	}
	
	// Fallback
	return fmt.Sprintf("Unknown CPU (%d-core, %s)", runtime.NumCPU(), runtime.GOARCH)
}

func getMemoryInfo() string {
	switch runtime.GOOS {
	case "darwin":
		if mem := sysctlInt("hw.memsize"); mem > 0 {
			gb := mem / (1024 * 1024 * 1024)
			return fmt.Sprintf("%dGB", gb)
		}
	case "linux":
		if mem := readProcMemInfo(); mem != "" {
			return mem
		}
	}
	
	// Fallback - get current memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf("~%dMB available", m.Sys/(1024*1024))
}

func getOSName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return runtime.GOOS
	}
}

func getOSVersion() string {
	switch runtime.GOOS {
	case "darwin":
		if version := sysctlString("kern.version"); version != "" {
			// Parse Darwin version to macOS version
			if strings.Contains(version, "Darwin Kernel Version") {
				parts := strings.Fields(version)
				if len(parts) >= 4 {
					return parts[3]
				}
			}
		}
	case "linux":
		if version := readFile("/proc/version"); version != "" {
			// Extract kernel version
			if strings.Contains(version, "Linux version") {
				parts := strings.Fields(version)
				if len(parts) >= 3 {
					return parts[2]
				}
			}
		}
	}
	
	return "Unknown"
}

// Helper functions for system info extraction
func sysctlString(name string) string {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func sysctlInt(name string) int {
	str := sysctlString(name)
	if str == "" {
		return 0
	}
	val, err := strconv.Atoi(str)
	if err != nil {
		return 0
	}
	return val
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readProcCPUInfo() string {
	content := readFile("/proc/cpuinfo")
	if content == "" {
		return ""
	}
	
	lines := strings.Split(content, "\n")
	var modelName string
	var coreCount int
	
	for _, line := range lines {
		if strings.HasPrefix(line, "model name") {
			parts := strings.Split(line, ":")
			if len(parts) > 1 {
				modelName = strings.TrimSpace(parts[1])
			}
		}
		if strings.HasPrefix(line, "processor") {
			coreCount++
		}
	}
	
	if modelName != "" {
		return fmt.Sprintf("%s (%d-core, %s)", modelName, coreCount, runtime.GOARCH)
	}
	
	return fmt.Sprintf("Unknown CPU (%d-core, %s)", runtime.NumCPU(), runtime.GOARCH)
}

func readProcMemInfo() string {
	content := readFile("/proc/meminfo")
	if content == "" {
		return ""
	}
	
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				kb, err := strconv.Atoi(parts[1])
				if err == nil {
					gb := kb / (1024 * 1024)
					return fmt.Sprintf("%dGB", gb)
				}
			}
			break
		}
	}
	
	return "Unknown"
}