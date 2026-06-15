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

type DomainWriter struct {
	Wayback  *os.File
	Katana   *os.File
	All      *os.File
	Filtered *os.File
}

func main() {
	ucMode := flag.Bool("uc", false, "Enable URL Collection mode (runs Wayback, Katana)")
	fuMode := flag.Bool("fu", false, "Filter URLs. If used alone, filters existing files in ./tmp/")
	fileFlag := flag.String("f", "", "File containing target URLs or Domains")
	
	flag.Usage = func() {
		const (
			Reset  = "\033[0m"
			Bold   = "\033[1m"
			Red    = "\033[31m"
			Green  = "\033[32m"
			Yellow = "\033[33m"
			Blue   = "\033[34m"
			Cyan   = "\033[36m"
		)
		fmt.Printf("\n%s%s[Cexss]%s - High-performance XSS discovery tool\n\n", Bold, Cyan, Reset)
		fmt.Printf("%s%sUSAGE:%s\n", Bold, Blue, Reset)
		fmt.Printf("  cexss [flags] [target]\n\n")
		fmt.Printf("%s%sFLAGS:%s\n", Bold, Blue, Reset)
		fmt.Printf("  %s%-4s%s  %s\n", Green, "-uc", Reset, "Enable URL Collection mode (runs Wayback, Katana)")
		fmt.Printf("  %s%-4s%s  %s\n", Green, "-fu", Reset, "Filter URLs (remove static assets). Filters ./tmp/ if used alone")
		fmt.Printf("  %s%-4s%s  %s\n", Green, "-f", Reset, "File containing target URLs or Domains")
		fmt.Printf("  %s%-4s%s  %s\n\n", Green, "-h", Reset, "Show this help message and exit")
		fmt.Printf("%s%sEXAMPLES:%s\n", Bold, Blue, Reset)
		fmt.Printf("  %s▶%s cexss -uc -fu example.com\n", Yellow, Reset)
		fmt.Printf("  %s▶%s cexss -fu example.com\n", Yellow, Reset)
		fmt.Printf("  %s▶%s cat domains.txt | cexss -uc -fu\n\n", Yellow, Reset)
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

	if *ucMode {
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

		processPipeline(urlChan, *fuMode)

	} else if *fuMode {
		logger.Info("Filter mode (-fu) enabled without collection. Filtering existing files in ./tmp/...")
		filterExistingDomains(inputChan)
		logger.Success("Filtering finished.")
		return

	} else {
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

// processPipeline now includes EXPLICIT STATISTICS to prove filtering is working.
func processPipeline(urlChan <-chan collector.CollectedURL, shouldFilter bool) {
	logger.Info("Processing URLs...")
	if shouldFilter {
		logger.Info("Filter mode (-fu) is ENABLED. Removing static assets.")
	} else {
		logger.Warning("Filter mode (-fu) is DISABLED. Keeping all URLs.")
	}
	
	writers := make(map[string]*DomainWriter)
	seenFiltered := make(map[string]struct{})
	
	// Statistics counters
	totalProcessed := 0
	filteredOut := 0
	savedUnique := 0

	for res := range urlChan {
		url := strings.TrimSpace(res.URL)
		if url == "" { continue }
		totalProcessed++

		safeDomain := sanitizeDomain(res.Domain)
		dw, exists := writers[safeDomain]
		if !exists {
			dw = initDomainFiles(safeDomain)
			writers[safeDomain] = dw
		}

		// 1. Save to raw tool-specific files
		if res.Source == "wayback" && dw.Wayback != nil {
			fmt.Fprintln(dw.Wayback, url)
		} else if res.Source == "katana" && dw.Katana != nil {
			fmt.Fprintln(dw.Katana, url)
		}

		// 2. Save to combined raw file
		if dw.All != nil {
			fmt.Fprintln(dw.All, url)
		}

		// 3. Normalize URL for filtering
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			url = "http://" + url
		}

		// 4. FILTERING LOGIC
		if shouldFilter {
			if filter.IsStaticResource(url) {
				filteredOut++
				continue // Skip saving to filtered.txt
			}
		}

		// 5. Deduplication
		if _, exists := seenFiltered[url]; exists {
			continue 
		}

		seenFiltered[url] = struct{}{}
		savedUnique++
		
		// 6. Save to filtered file
		if dw.Filtered != nil {
			fmt.Fprintln(dw.Filtered, url)
		}
	}

	// Close all open files
	for _, dw := range writers {
		if dw.Wayback != nil { dw.Wayback.Close() }
		if dw.Katana != nil { dw.Katana.Close() }
		if dw.All != nil { dw.All.Close() }
		if dw.Filtered != nil { dw.Filtered.Close() }
	}

	// Print explicit statistics
	logger.Success("Pipeline finished.")
	if shouldFilter {
		logger.Info("Stats: Processed %d URLs | Filtered out %d static assets | Saved %d unique valid URLs", totalProcessed, filteredOut, savedUnique)
	} else {
		logger.Info("Stats: Processed %d URLs | Saved %d unique URLs (No filtering applied)", totalProcessed, savedUnique)
	}
	logger.Info("Check ./tmp/ for results.")
}

func filterExistingDomains(inputChan <-chan string) {
	for domain := range inputChan {
		domain = strings.TrimSpace(domain)
		if domain == "" { continue }

		safeDomain := sanitizeDomain(domain)
		dir := filepath.Join("tmp", safeDomain)

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			logger.Error("Directory not found for %s: %s", domain, dir)
			continue
		}

		logger.Info("Filtering existing URLs for %s...", domain)

		allFilePath := filepath.Join(dir, "all.txt")
		inFile, err := os.Open(allFilePath)
		if err != nil {
			logger.Error("Could not open %s: %v (Did you run -uc first?)", allFilePath, err)
			continue
		}

		filteredFilePath := filepath.Join(dir, "filtered.txt")
		outFile, err := os.Create(filteredFilePath)
		if err != nil {
			logger.Error("Could not create %s: %v", filteredFilePath, err)
			inFile.Close()
			continue
		}

		seen := make(map[string]struct{})
		scanner := bufio.NewScanner(inFile)
		countTotal := 0
		countFiltered := 0

		for scanner.Scan() {
			rawURL := strings.TrimSpace(scanner.Text())
			if rawURL == "" { continue }
			countTotal++

			if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
				rawURL = "http://" + rawURL
			}

			if _, exists := seen[rawURL]; exists { continue }
			if filter.IsStaticResource(rawURL) { continue }

			seen[rawURL] = struct{}{}
			fmt.Fprintln(outFile, rawURL)
			countFiltered++
		}

		inFile.Close()
		outFile.Close()

		logger.Success("Filtered %s: %d total -> %d valid URLs", domain, countTotal, countFiltered)
	}
}

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
