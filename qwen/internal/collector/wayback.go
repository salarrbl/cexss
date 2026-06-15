package collector

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"

	"github.com/salarrbl/cexss/pkg/logger"
)

// WaybackCollector executes the external waybackurls binary.
type WaybackCollector struct{}

// NewWaybackCollector creates a new instance.
func NewWaybackCollector() *WaybackCollector {
	return &WaybackCollector{}
}

// Fetch implements the Collector interface by running waybackurls via os/exec.
func (w *WaybackCollector) Fetch(target string, out chan<- CollectedURL) error {
	// waybackurls expects just the domain name (e.g., "example.com").
	// If the user accidentally passed "http://example.com", we strip the protocol.
	target = strings.TrimPrefix(target, "http://")
	target = strings.TrimPrefix(target, "https://")

	// Create the command. Notice we don't pass any arguments here, 
	// because waybackurls reads from stdin.
	cmd := exec.Command("waybackurls")

	// 1. Create a pipe for Standard Input (stdin).
	// This allows our Go program to send data TO the waybackurls process.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		logger.Error("Wayback failed for %s: stdin pipe error", target)
		return err
	}

	// 2. Create a pipe for Standard Output (stdout).
	// This allows our Go program to read the URLs FROM the waybackurls process.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Error("Wayback failed for %s: stdout pipe error", target)
		return err
	}

	// 3. Start the process in the background.
	if err := cmd.Start(); err != nil {
		logger.Error("Wayback failed for %s: %v (is it installed?)", target, err)
		return err
	}

	// 4. Write the domain into waybackurls' stdin.
	// We use fmt.Fprintln because it automatically adds a newline (\n) at the end.
	// CLI tools read line-by-line, so the newline is required!
	fmt.Fprintln(stdin, target)
	
	// 5. CRITICAL: Close the stdin pipe!
	// If we don't close it, waybackurls will sit there forever waiting 
	// for us to type more domains, and the program will hang.
	stdin.Close()

	// 6. Read the output line-by-line as waybackurls discovers URLs.
	scanner := bufio.NewScanner(stdout)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			out <- CollectedURL{
				URL:    line,
				Source: "wayback",
				Domain: target,
			}
			count++
		}
	}

	// 7. Wait for the process to finish and check for errors.
	if err := cmd.Wait(); err != nil {
		logger.Error("Wayback failed for %s: process error", target)
		return err
	}

	// Log success only when completely finished.
	logger.Success("Wayback done for %s (%d URLs)", target, count)
	return nil
}
