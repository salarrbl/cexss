package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/salarrbl/cexss/internal/collector"
	"github.com/salarrbl/cexss/pkg/filter"
	"github.com/salarrbl/cexss/pkg/logger"
)

func main() {
	// 1. Define Flags
	// -uc is a boolean switch that turns on URL Collection mode
	ucMode := flag.Bool("uc", false, "Enable URL Collection mode (runs Wayback, Katana, etc.)")
	fileFlag := flag.String("f", "", "File containing target URLs or Domains")
	
	// Customize the help menu so it looks professional
	flag.Usage = func() {
		fmt.Printf("Cexss - High-performance XSS discovery tool\n\n")
		fmt.Printf("Usage: %s [flags] [target]\n\n", os.Args[0])
		fmt.Println("Flags:")
		flag.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Println("  # Run URL collection on a single domain")
		fmt.Printf("  %s -uc example.com\n", os.Args[0])
		fmt.Println("  # Run URL collection via stdin pipe")
		fmt.Println("  echo example.com | cexss -uc")
		fmt.Println("  # Process a single URL directly (bypasses collection)")
		fmt.Println("  cexss http://example.com/page?id=1")
	}

	// 2. Show help if NO flags or arguments are provided at all
	// os.Args[0] is the program name. If length is 1, nothing else was typed.
	if len(os.Args) == 1 {
		flag.Usage()
		os.Exit(0)
	}

	// 3. Parse the flags
	flag.Parse()

	// 4. Capture the Positional Argument (e.g., the "domain.com" in "-uc domain.com")
	singleTarget := ""
	if flag.NArg() > 0 {
		singleTarget = flag.Arg(0)
	}

	// 5. Create the unified input channel (handles single target, file, or stdin)
	inputChan := getInputChannel(singleTarget, *fileFlag)

	// 6. Create the channel for URLs that will go through the pipeline
	urlChan := make(chan string, 100)

	if *ucMode {
		logger.Info("URL Collection mode (-uc) enabled. Running collectors...")
		
		go func() {
			var collectorWg sync.WaitGroup
			seenDomains := make(map[string]struct{})

			for domain := range inputChan {
				domain = strings.TrimSpace(domain)
				if domain == "" {
					continue
				}
				
				if _, exists := seenDomains[domain]; exists {
					continue
				}
				seenDomains[domain] = struct{}{}

				// Spawn Wayback Collector
				collectorWg.Add(1)
				go func(d string) {
					defer collectorWg.Done()
					wayback := collector.NewWaybackCollector()
					if err := wayback.Fetch(d, urlChan); err != nil {
						logger.Error("Wayback failed for %s: %v", d, err)
					}
				}(domain)

				// Spawn Katana Collector
				collectorWg.Add(1)
				go func(d string) {
					defer collectorWg.Done()
					katana := collector.NewKatanaCollector()
					if err := katana.Fetch(d, urlChan); err != nil {
						logger.Error("Katana failed for %s: %v", d, err)
					}
				}(domain)
			}

			// Wait for ALL collectors to finish, then safely close the channel
			collectorWg.Wait()
			close(urlChan)
		}()

	} else {
		// URL Mode: Input is already URLs, just pass them through to the pipeline
		go func() {
			for u := range inputChan {
				urlChan <- u
			}
			close(urlChan)
		}()
	}

	// 7. Process the URLs (Normalize, Deduplicate, Filter)
	logger.Info("Processing URLs...")
	seen := make(map[string]struct{})

	for target := range urlChan {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		// Normalize URL (ensure it has a scheme so url.Parse works later)
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			target = "http://" + target
		}

		if _, exists := seen[target]; exists {
			continue
		}

		if filter.IsStaticResource(target) {
			continue
		}

		seen[target] = struct{}{}
		logger.Success("Valid URL: %s", target)
	}

	logger.Info("Pipeline finished.")
}

// getInputChannel handles reading from a single target string, -f file, or stdin.
func getInputChannel(singleTarget, filePath string) <-chan string {
	out := make(chan string)

	go func() {
		defer close(out)

		// 1. Single target from positional argument
		if singleTarget != "" {
			out <- singleTarget
		}

		// 2. File input
		if filePath != "" {
			readFile(filePath, out)
		}

		// 3. Stdin input (if data is being piped)
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			readStdin(out)
		}
	}()

	return out
}

func readFile(path string, out chan<- string) {
	file, err := os.Open(path)
	if err != nil {
		logger.Error("Could not open file: %v", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			out <- line
		}
	}
}

func readStdin(out chan<- string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			out <- line
		}
	}
}
