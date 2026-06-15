package main

import (
	"bufio"
	"flag"
	"os"
	"strings"

	"github.com/salarrbl/cexss/pkg/filter"
	"github.com/salarrbl/cexss/pkg/logger"
)

func main() {
	urlFlag := flag.String("u", "", "Single target URL")
	fileFlag := flag.String("f", "", "File containing target URLs")
	flag.Parse()

	urlChan := make(chan string, 100)

	// Start the input loader goroutine
	go func() {
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

	logger.Info("Cexss initialized. Processing targets...")

	// 1. Deduplication Map
	// We use struct{} because it occupies 0 bytes of memory. 
	// This map will store unique URLs we have already processed.
	seen := make(map[string]struct{})

	for target := range urlChan {
		// Clean the input (remove spaces)
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}

		// 2. Check for Deduplication
		if _, exists := seen[target]; exists {
			continue // Skip if we already saw this URL
		}
		
		// 3. Check for Static Extensions
		if filter.IsStaticResource(target) {
			continue // Skip images, css, etc.
		}

		// Mark as seen
		seen[target] = struct{}{}

		// If it passes all checks, it's a valid target
		logger.Success("Valid target: %s", target)
	}
}

// ... readFile and readStdin functions remain the same as before ...

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
