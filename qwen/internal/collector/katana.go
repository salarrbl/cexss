package collector

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"

	"github.com/salarrbl/cexss/pkg/logger"
)

// KatanaCollector executes the external Katana binary to crawl live URLs.
type KatanaCollector struct{}

// NewKatanaCollector creates a new instance.
func NewKatanaCollector() *KatanaCollector {
	return &KatanaCollector{}
}

// Fetch implements the Collector interface by running Katana via os/exec.
func (k *KatanaCollector) Fetch(target string, out chan<- string) error {
	logger.Info("Starting Katana crawler for %s...", target)

	// Katana requires a full URL (with http:// or https://). 
	// If the user just passed "example.com", we must prepend the scheme so Katana doesn't crash.
	targetURL := target
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	// exec.Command creates a command object but doesn't run it yet. 
	// "katana" is the binary name. 
	// "-u" specifies the target URL.
	// "-silent" tells Katana to ONLY output the discovered URLs, hiding its banner and info logs.
	cmd := exec.Command("katana", "-u", targetURL, "-silent")

	// StdoutPipe creates a pipe that we can read from in Go. 
	// This connects Katana's standard output directly to our Go program.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe for katana: %w", err)
	}

	// Start runs the command in the background. 
	// If Katana is not installed on your system, this is where it will fail.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start katana (is it installed and in your PATH?): %w", err)
	}

	// We use a bufio.Scanner to read the output line-by-line as Katana generates it.
	// This is highly memory efficient because we don't wait for Katana to finish 
	// before processing the URLs. We process them the exact millisecond they are found.
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			// Send the discovered URL directly into our pipeline channel!
			out <- line
		}
	}

	// Wait for the Katana process to finish and check for any execution errors.
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("katana process finished with error: %w", err)
	}

	logger.Success("Katana crawling finished for %s", target)
	return nil
}
