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
type DomainWriter struct {
	Wayback  *os.File
	Katana   *os.File
	All      *os.File
	Filtered *os.File
}

func main() {
	// 1. Define Flags
	ucMode := flag.Bool("uc", false, "Enable URL Collection mode (runs Wayback, Katana)")
	fuMode := flag.Bool("fu", false, "Filter URLs. If used alone, filters existing files in ./tmp/")
	fileFlag := flag.String("f", "", "File containing target URLs or Domains")
	
	flag.Usage = func() {
		fmt.Printf("Cexss - High-performance XSS discovery tool\n\n")
		fmt.Printf("Usage: %s [flags] [target]\n\n", os.Args[0])
		fmt.Println("Flags:")
		flag.PrintDefaults()
		fmt.Println("\nExamples:")
		fmt.Println("  # Collect URLs and filter out static assets")
		fmt.Printf("  %s -uc -fu example.com\n", os.Args[0])
		fmt.Println("  # Filter existing collected URLs in ./tmp/example.com/")
		fmt.Printf("  %s -fu example.com\n", os.Args[0])
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

	// --- ROUTING LOGIC ---
	// We check the flags to decide which "Mode" the tool should run in.
	
	if *ucMode {
		// MODE 1: URL COLLECTION (with or without filtering)
		logger.Info("URL Collection mode (-uc) enabled.")
		urlChan := make(chan collector.CollectedURL, 100)
		
		go func() {
			var collectorWg sync.WaitGroup
			seenDomains := make(map[string]struct{})

			for domain := range inputChan {
				domain = strings.TrimSpace(domain)
				if domain == "" { continue }
				if _, exists := seenDomains[domain]; exists { continue }
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

		// Pass the channel to the pipeline processor
		processPipeline(urlChan, *fuMode)

	} else if *fuMode {
		// MODE 2: FILTER EXISTING FILES (The new feature!)
		logger.Info("Filter mode (-fu) enabled without collection. Filtering existing files in ./tmp/...")
		filterExistingDomains(inputChan)
		logger.Success("Filtering finished.")
		return // Exit early, we don't need the rest of the pipeline

	} else {
		// MODE 3: DIRECT URL PROCESSING (No collection, no filtering)
		urlChan := make(chan collector.CollectedURL, 100)
		go func() {
			for u := range inputChan {
				urlChan <- collector.CollectedURL{URL: u, Source: "stdin", Domain: "default"}
			}
			close(urlChan)
		}()
		
		processPipeline(urlChan, false)
	}
}

// filterExistingDomains reads all.txt from the tmp directory and filters it.
func filterExistingDomains(inputChan <-chan string) {
	for domain := range inputChan {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}

		safeDomain := sanitizeDomain(domain)
		dir := filepath.Join("tmp", safeDomain)

		// 1. Check if directory exists
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			logger.Error("Directory not found for %s: %s", domain, dir)
			continue
		}

		logger.Info("Filtering existing URLs for %s...", domain)

		// 2. Open all.txt for reading
		allFilePath := filepath.Join(dir, "all.txt")
		inFile, err := os.Open(allFilePath)
		if err != nil {
			logger.Error("Could not open %s: %v (Did you run -uc first?)", allFilePath, err)
			continue
		}

		// 3. Create/overwrite filtered.txt
		filteredFilePath := filepath.Join(dir, "filtered.txt")
		outFile, err := os.Create(filteredFilePath)
		if err != nil {
			logger.Error("Could not create %s: %v", filteredFilePath, err)
			inFile.Close()
			continue
		}

		// 4. Process line by line
		seen := make(map[string]struct{})
		scanner := bufio.NewScanner(inFile)
		countTotal := 0
		countFiltered := 0

		for scanner.Scan() {
			rawURL := strings.TrimSpace(scanner.Text())
			if rawURL == "" {
				continue
			}
			countTotal++

			// Normalize URL
			if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
				rawURL = "http://" + rawURL
			}

			// Deduplicate
			if _, exists := seen[rawURL]; exists {
				continue
			}

			// Filter static resources
			if filter.IsStaticResource(rawURL) {
				continue
			}

			// Save to filtered file
			seen[rawURL] = struct{}{}
			fmt.Fprintln(outFile, rawURL)
			countFiltered++
		}

		inFile.Close()
		outFile.Close()

		logger.Success("Filtered %s: %d total -> %d valid URLs", domain, countTotal, countFiltered)
	}
}

// processPipeline handles the standard URL processing loop for collection and direct URL modes.
func processPipeline(urlChan <-chan collector.CollectedURL, shouldFilter bool) {
	logger.Info("Processing URLs...")
	if shouldFilter {
		logger.Info("Filter mode (-fu) enabled. Removing static assets.")
	} else {
		logger.Warning("Filter mode (-fu) is DISABLED. Keeping all URLs.")
	}
	
	writers := make(map[string]*DomainWriter)
	seenFiltered := make(map[string]struct{})

	for res := range urlChan {
		url := strings.TrimSpace(res.URL)
		if url == "" { continue }

		safeDomain := sanitizeDomain(res.Domain)
		dw, exists := writers[safeDomain]
		if !exists {
			dw = initDomainFiles(safeDomain)
			writers[safeDomain] = dw
		}

		if res.Source == "wayback" && dw.Wayback != nil {
			fmt.Fprintln(dw.Wayback, url)
		} else if res.Source == "katana" && dw.Katana != nil {
			fmt.Fprintln(dw.Katana, url)
		}

		if dw.All != nil {
			fmt.Fprintln(dw.All, url)
		}

		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "http://" + url
		}

		if shouldFilter {
			if filter.IsStaticResource(url) {
				continue 
			}
		}

		if _, exists := seenFiltered[url]; exists {
			continue 
		}

		seenFiltered[url] = struct{}{}
		if dw.Filtered != nil {
			fmt.Fprintln(dw.Filtered, url)
		}
	}

	for _, dw := range writers {
		if dw.Wayback != nil { dw.Wayback.Close() }
		if dw.Katana != nil { dw.Katana.Close() }
		if dw.All != nil { dw.All.Close() }
		if dw.Filtered != nil { dw.Filtered.Close() }
	}

	logger.Success("Pipeline finished. Check ./tmp/ for results.")
}

// --- Helper Functions ---

func initDomainFiles(domain string) *DomainWriter {
	dir := filepath.Join("tmp", domain)
	os.MkdirAll(dir, 0755)

	dw := &DomainWriter{}
	dw.Wayback, _ = os.Create(filepath.Join(dir, "wayback.txt"))
	dw.Katana, _ = os.Create(filepath.Join(dir, "katana.txt"))
	dw.All, _ = os.Create(filepath.Join(dir, "all.txt"))
	dw.Filtered, _ = os.Create(filepath.Join(dir, "filtered.txt"))
	return dw
}

func sanitizeDomain(d string) string {
	d = strings.ReplaceAll(d, "http://", "")
	d = strings.ReplaceAll(d, "https://", "")
	d = strings.ReplaceAll(d, "/", "_")
	d = strings.ReplaceAll(d, ":", "_")
	return d
}

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
