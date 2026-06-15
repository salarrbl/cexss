package main

import (
	"bufio"
	"flag"
	"os"
	"strings"
	"sync"

	"github.com/salarrbl/cexss/internal/collector"
	"github.com/salarrbl/cexss/pkg/filter"
	"github.com/salarrbl/cexss/pkg/logger"
)

func main() {
	urlFlag := flag.String("u", "", "Single target URL")
	fileFlag := flag.String("f", "", "File containing target URLs")
	
	// NEW: Add a flag for domain collection
	domainFlag := flag.String("d", "", "Domain to collect URLs for (e.g., example.com)")
	
	flag.Parse()

	urlChan := make(chan string, 100)

	// We use a WaitGroup to wait for our background goroutines to finish 
	// before the program exits.
	var wg sync.WaitGroup

	// --- INPUT LOADING LOGIC ---
	if *domainFlag != "" {
		// If the user provided a domain, we use the URL Collection step!
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(urlChan) // Close the channel when collection is done

			wayback := collector.NewWaybackCollector()
			if err := wayback.Fetch(*domainFlag, urlChan); err != nil {
				logger.Error("Wayback collector failed: %v", err)
			}
		}()
	} else {
		// Otherwise, fall back to the existing stdin/file/single URL logic
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(urlChan)

			if *urlFlag != "" {
				urlChan <- *urlFlag
			}

			if *fileFlag != "" {
				readFile(*fileFlag, urlChan)
			}

			stat, _ := os.Stdin.Stat()
			if (stat.Mode() & os.ModeCharDevice) == 0 {
				readStdin(urlChan)
			}
		}()
	}

	logger.Info("Cexss initialized. Processing targets...")

	seen := make(map[string]struct{})

	// We range over the channel. This loop will automatically exit 
	// when the channel is closed by the goroutines above.
	for target := range urlChan {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		if _, exists := seen[target]; exists {
			continue 
		}
		
		if filter.IsStaticResource(target) {
			continue 
		}

		seen[target] = struct{}{}

		// For now, we just log that the URL survived the collection and filtering steps.
		// In the next steps, we will pass this to the Parameter Discovery Engine.
		logger.Success("Collected & Valid: %s", target)
	}

	// Wait for all input/collector goroutines to finish cleanly
	wg.Wait()
	logger.Info("Pipeline finished.")
}

func readFile(path string, out chan string) {
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

func readStdin(out chan string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			out <- line
		}
	}
}
