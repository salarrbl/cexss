package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/salarrbl/cexss/internal/collector"
	"github.com/salarrbl/cexss/pkg/filter"
	"github.com/salarrbl/cexss/pkg/logger"
)

// DomainWriter holds the open file handles for a specific domain.
// Keeping files open is crucial for high-performance I/O.
type DomainWriter struct {
	Wayback  *os.File
	Katana   *os.File
	All      *os.File
	Filtered *os.File
}

func main() {
	ucMode := flag.Bool("uc", false, "Enable URL Collection mode")
	fileFlag := flag.String("f", "", "File containing target URLs or Domains")
	
	flag.Usage = func() {
		fmt.Printf("Cexss - High-performance XSS discovery tool\n\n")
		fmt.Printf("Usage: %s [flags] [target]\n\n", os.Args[0])
		fmt.Println("Flags:")
		flag.PrintDefaults()
	}

	if len(os.Args) == 1 {
		flag.Usage()
		os.Exit(0)
	}

	flag.Parse()

	singleTarget := ""
	if flag.NArg() > 0 {
		singleTarget = flag.Arg(0)
	}

	inputChan := getInputChannel(singleTarget, *fileFlag)
	
	// CHANGED: The channel now carries the CollectedURL struct
	urlChan := make(chan collector.CollectedURL, 100)

	if *ucMode {
		logger.Info("URL Collection mode (-uc) enabled.")
		
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

				collectorWg.Add(2)
				go func(d string) {
					defer collectorWg.Done()
					collector.NewWaybackCollector().Fetch(d, urlChan)
				}(domain)

				go func(d string) {
					defer collectorWg.Done()
					collector.NewKatanaCollector().Fetch(d, urlChan)
				}(domain)
			}

			collectorWg.Wait()
			close(urlChan)
		}()
	} else {
		go func() {
			for u := range inputChan {
				// If not in UC mode, we assume input is already URLs.
				// We assign them a generic source and domain.
				urlChan <- collector.CollectedURL{URL: u, Source: "stdin", Domain: "default"}
			}
			close(urlChan)
		}()
	}

	logger.Info("Processing URLs and saving to ./tmp/...")
	
	// Map to keep track of our open files for each domain
	writers := make(map[string]*DomainWriter)
	
	// Deduplication map for the filtered URLs
	seenFiltered := make(map[string]struct{})

	// Process the stream
	for res := range urlChan {
		url := strings.TrimSpace(res.URL)
		if url == "" {
			continue
		}

		// 1. Sanitize domain name for folder creation (remove http://, replace / and :)
		safeDomain := sanitizeDomain(res.Domain)
		
		// 2. Initialize files for this domain if we haven't already
		dw, exists := writers[safeDomain]
		if !exists {
			dw = initDomainFiles(safeDomain)
			writers[safeDomain] = dw
		}

		// 3. Save to tool-specific file (wayback.txt or katana.txt)
		if res.Source == "wayback" && dw.Wayback != nil {
			fmt.Fprintln(dw.Wayback, url)
		} else if res.Source == "katana" && dw.Katana != nil {
			fmt.Fprintln(dw.Katana, url)
		}

		// 4. Save to combined raw file (all.txt)
		if dw.All != nil {
			fmt.Fprintln(dw.All, url)
		}

		// 5. Normalize URL for filtering
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "http://" + url
		}

		// 6. Filter and Deduplicate
		if _, exists := seenFiltered[url]; exists {
			continue // Skip duplicates
		}
		if filter.IsStaticResource(url) {
			continue // Skip static assets
		}

		// Mark as seen and save to filtered file
		seenFiltered[url] = struct{}{}
		if dw.Filtered != nil {
			fmt.Fprintln(dw.Filtered, url)
		}
	}

	// CRITICAL: Close all open files when the pipeline finishes!
	for _, dw := range writers {
		if dw.Wayback != nil { dw.Wayback.Close() }
		if dw.Katana != nil { dw.Katana.Close() }
		if dw.All != nil { dw.All.Close() }
		if dw.Filtered != nil { dw.Filtered.Close() }
	}

	logger.Success("Pipeline finished. Check ./tmp/ for results.")
}

// initDomainFiles creates the directory and opens the 4 files for a domain.
func initDomainFiles(domain string) *DomainWriter {
	dir := filepath.Join("tmp", domain)
	os.MkdirAll(dir, 0755)

	dw := &DomainWriter{}
	
	// We use os.Create which truncates the file if it exists, or creates it.
	// O_APPEND is not strictly needed here since we create a fresh file for this run,
	// but fmt.Fprintln handles the newlines automatically.
	dw.Wayback, _ = os.Create(filepath.Join(dir, "wayback.txt"))
	dw.Katana, _ = os.Create(filepath.Join(dir, "katana.txt"))
	dw.All, _ = os.Create(filepath.Join(dir, "all.txt"))
	dw.Filtered, _ = os.Create(filepath.Join(dir, "filtered.txt"))

	return dw
}

// sanitizeDomain cleans the domain string so it can be safely used as a folder name.
func sanitizeDomain(d string) string {
	d = strings.ReplaceAll(d, "http://", "")
	d = strings.ReplaceAll(d, "https://", "")
	d = strings.ReplaceAll(d, "/", "_")
	d = strings.ReplaceAll(d, ":", "_")
	return d
}

// --- Input Channel Logic (Unchanged) ---
func getInputChannel(singleTarget, filePath string) <-chan string {
	out := make(chan string)
	go func() {
		defer close(out)
		if singleTarget != "" { out <- singleTarget }
		if filePath != "" { readFile(filePath, out) }
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 { readStdin(out) }
	}()
	return out
}

func readFile(path string, out chan<- string) {
	file, err := os.Open(path)
	if err != nil { logger.Error("Could not open file: %v", err); return }
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" { out <- line }
	}
}

func readStdin(out chan<- string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" { out <- line }
	}
}
