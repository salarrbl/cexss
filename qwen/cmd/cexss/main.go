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
	domainFlag := flag.String("d", "", "Domain to collect URLs for (e.g., example.com)")
	flag.Parse()

	urlChan := make(chan string, 100)
	var wg sync.WaitGroup

	if *domainFlag != "" {
		// We are running TWO collectors concurrently, so we add 2 to the WaitGroup
		wg.Add(2)

		// --- Goroutine 1: Wayback Machine ---
		go func() {
			defer wg.Done()
			wayback := collector.NewWaybackCollector()
			if err := wayback.Fetch(*domainFlag, urlChan); err != nil {
				logger.Error("Wayback collector failed: %v", err)
			}
		}()

		// --- Goroutine 2: Katana Crawler ---
		go func() {
			defer wg.Done()
			katana := collector.NewKatanaCollector()
			if err := katana.Fetch(*domainFlag, urlChan); err != nil {
				logger.Error("Katana collector failed: %v", err)
			}
		}()

		// --- Goroutine 3: The Channel Closer ---
		// CRITICAL GO LESSON: We cannot let the collectors close the channel.
		// If Wayback finishes first and closes the channel, Katana will PANIC 
		// when it tries to send a URL into a closed channel!
		// Instead, we start a separate goroutine that waits for BOTH to finish, 
		// and ONLY THEN closes the channel.
		go func() {
			wg.Wait()
			close(urlChan)
		}()

	} else {
		// Fallback to existing stdin/file/single URL logic
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

	// This loop will automatically exit when Goroutine 3 closes the channel
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
		logger.Success("Collected & Valid: %s", target)
	}

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
