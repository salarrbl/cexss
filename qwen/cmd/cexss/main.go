package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/salarrbl/cexss/internal/collector"
	"github.com/salarrbl/cexss/internal/param"
	"github.com/salarrbl/cexss/internal/urlgen"
	"github.com/salarrbl/cexss/pkg/filter"
	"github.com/salarrbl/cexss/pkg/logger"
)

type DomainWriter struct {
	Wayback, Katana, All, Filtered *os.File
}

type ParamCount struct {
	Name  string
	Count int
}

func main() {
	ucMode := flag.Bool("uc", false, "Enable URL Collection")
	fuMode := flag.Bool("fu", false, "Filter URLs")
	psMode := flag.Bool("ps", false, "Parameter Search")
	ugMode := flag.Bool("ug", false, "URL Generation")
	
	strategyFlag := flag.String("strategy", "normal", "Strategy: normal, combine, ignore, all")
	strategyValueFlag := flag.String("strategy_value", "replace", "Mutation: replace, suffix")
	templateFlag := flag.String("t", "payloads/detect-Cexss.yaml", "Path to Nuclei template")
	
	fileFlag := flag.String("f", "", "File containing targets")
	nucleiFlag := flag.Bool("nuclei", false, "Pipe results to Nuclei")
	
	flag.Usage = func() {
		const (Reset, Bold, Cyan, Green, Yellow, Blue = "\033[0m", "\033[1m", "\033[36m", "\033[32m", "\033[33m", "\033[34m")
		fmt.Printf("\n%s%s[Cexss]%s - Advanced XSS Automation\n\n", Bold, Cyan, Reset)
		fmt.Printf("%s%sFLAGS:%s\n", Bold, Blue, Reset)
		fmt.Printf("  %s%-14s%s %s\n", Green, "-uc", Reset, "Collect URLs (Wayback, Katana)")
		fmt.Printf("  %s%-14s%s %s\n", Green, "-fu", Reset, "Filter static assets")
		fmt.Printf("  %s%-14s%s %s\n", Green, "-ps", Reset, "Discover & Rank parameters")
		fmt.Printf("  %s%-14s%s %s\n", Green, "-ug", Reset, "Generate xamir.txt")
		fmt.Printf("  %s%-14s%s %s\n", Green, "-strategy", Reset, "Mode: normal, combine, ignore, all")
		fmt.Printf("  %s%-14s%s %s\n", Green, "-strategy_value", Reset, "Mutation: replace, suffix")
		fmt.Printf("  %s%-14s%s %s\n", Green, "-t", Reset, "Nuclei template path (default: payloads/detect-Cexss.yaml)")
		fmt.Printf("  %s%-14s%s %s\n\n", Green, "-nuclei", Reset, "Run Nuclei scan on generated URLs")
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

	targets := getTargets(singleTarget, *fileFlag)
	if len(targets) == 0 {
		logger.Error("No targets.")
		os.Exit(1)
	}

	payloads := loadPayloads()

	if *ucMode {
		logger.Info("URL Collection mode (-uc) enabled.")
		urlChan := make(chan collector.CollectedURL, 100)
		go func() {
			var wg sync.WaitGroup
			seen := make(map[string]struct{})
			for _, d := range targets {
				if _, ok := seen[d]; ok {
					continue
				}
				seen[d] = struct{}{}
				wg.Add(2)
				go func(d string) {
					defer wg.Done()
					collector.NewWaybackCollector().Fetch(d, urlChan)
				}(d)
				go func(d string) {
					defer wg.Done()
					collector.NewKatanaCollector().Fetch(d, urlChan)
				}(d)
			}
			wg.Wait()
			close(urlChan)
		}()
		
		processPipeline(urlChan, *fuMode)
		if *psMode {
			processParameterDiscovery(targets)
		}
		if *ugMode {
			processURLGeneration(targets, *strategyFlag, *strategyValueFlag, payloads)
		}
		if *nucleiFlag && *ugMode {
			runNuclei(targets, *templateFlag)
		}

	} else if *fuMode {
		filterExistingDomains(targets)
	} else if *psMode {
		processParameterDiscovery(targets)
	} else if *ugMode {
		processURLGeneration(targets, *strategyFlag, *strategyValueFlag, payloads)
		if *nucleiFlag {
			runNuclei(targets, *templateFlag)
		}
	} else {
		urlChan := make(chan collector.CollectedURL, 100)
		go func() {
			for _, u := range targets {
				urlChan <- collector.CollectedURL{URL: u, Source: "stdin", Domain: "default"}
			}
			close(urlChan)
		}()
		processPipeline(urlChan, false)
	}
}

func loadPayloads() []string {
	lines, err := readLinesFromFile("payloads/payloads.txt")
	if err != nil {
		logger.Warning("Could not load payloads/payloads.txt: %v", err)
		return []string{"cexss"}
	}
	var clean []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "#") {
			clean = append(clean, l)
		}
	}
	logger.Info("Loaded %d custom payloads.", len(clean))
	return clean
}

func processURLGeneration(targets []string, strategy, mutationMode string, payloads []string) {
	generator := urlgen.NewGenerator(25, payloads, mutationMode)

	for _, domain := range targets {
		safeDomain := sanitizeDomain(domain)
		dir := filepath.Join("tmp", safeDomain)
		
		logger.Info("Starting URL Generation (%s / %s) for %s...", strategy, mutationMode, domain)

		urls, err := readLinesFromFile(filepath.Join(dir, "filtered.txt"))
		if err != nil {
			logger.Error("Missing filtered.txt for %s. Run -uc -fu first.", domain)
			continue
		}

		allParams, err := readLinesFromFile(filepath.Join(dir, "params.txt"))
		if err != nil {
			logger.Error("Missing params.txt for %s. Run -ps first.", domain)
			continue
		}

		xamirFile := filepath.Join(dir, "xamir.txt")
		outFile, _ := os.Create(xamirFile)
		totalGenerated := 0

		for _, u := range urls {
			if !strings.HasPrefix(u, "http") {
				u = "http://" + u
			}
			existingParams := urlgen.GetExistingParams(u)
			generated := generator.Generate(u, existingParams, allParams, strategy)
			
			for _, genURL := range generated {
				fmt.Fprintln(outFile, genURL)
				totalGenerated++
			}
		}

		outFile.Close()
		logger.Success("Generated %d URLs (Chunks x Payloads) for %s. Saved to %s", totalGenerated, domain, xamirFile)
	}
}

func runNuclei(targets []string, templatePath string) {
	logger.Info("Starting Nuclei Scan with template: %s", templatePath)
	
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		logger.Error("Template not found at %s. Please create it.", templatePath)
		return
	}

	var allXamirFiles []string
	for _, domain := range targets {
		path := filepath.Join("tmp", sanitizeDomain(domain), "xamir.txt")
		if _, err := os.Stat(path); err == nil {
			allXamirFiles = append(allXamirFiles, path)
		}
	}

	if len(allXamirFiles) == 0 {
		logger.Error("No xamir.txt files found.")
		return
	}

	// FIX: Nuclei sometimes fails with "-l -" (stdin) depending on the version.
	// We write all URLs to a temporary file and pass that file to Nuclei. This is 100% reliable.
	tmpFile, err := os.CreateTemp("", "cexss-nuclei-*.txt")
	if err != nil {
		logger.Error("Could not create temp file for Nuclei: %v", err)
		return
	}
	tmpPath := tmpFile.Name()
	
	// defer os.Remove(tmpPath) // Automatically delete the temp file when the function finishes
	// Note: I commented out the auto-delete so you can inspect the file if you want to debug Nuclei.
	// If you want it to auto-delete, just remove the // from the line above.

	// Write all URLs to the temp file
	totalURLs := 0
	for _, file := range allXamirFiles {
		lines, _ := readLinesFromFile(file)
		for _, line := range lines {
			fmt.Fprintln(tmpFile, line)
			totalURLs++
		}
	}
	tmpFile.Close()

	logger.Info("Prepared %d URLs for Nuclei scan in temporary file.", totalURLs)

	// Run Nuclei using the temporary file instead of stdin
	cmd := exec.Command("nuclei", "-l", tmpPath, "-t", templatePath, "-silent")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		logger.Error("Failed to start Nuclei: %v", err)
		return
	}

	if err := cmd.Wait(); err != nil {
		logger.Error("Nuclei error: %v", err)
	} else {
		logger.Success("Nuclei scan completed.")
	}
}
func processParameterDiscovery(targets []string) {
	discoverer := param.NewDiscoverer()
	wordlistParams := loadWordlists()
	logger.Info("Loaded %d parameters from smart wordlists.", len(wordlistParams))
	globalParams := make(map[string]int)

	for _, domain := range targets {
		safeDomain := sanitizeDomain(domain)
		dir := filepath.Join("tmp", safeDomain)
		logger.Info("Starting Parameter Discovery & Ranking for %s...", domain)

		filteredFile := filepath.Join(dir, "filtered.txt")
		urls, err := readLinesFromFile(filteredFile)
		if err != nil {
			allFile := filepath.Join(dir, "all.txt")
			urls, err = readLinesFromFile(allFile)
			if err != nil {
				logger.Error("Could not read URLs for %s: %v", domain, err)
				continue
			}
		}
		
		paramFrequency := make(map[string]int)
		for _, u := range urls {
			if !strings.HasPrefix(u, "http") {
				u = "http://" + u
			}
			params := discoverer.ExtractFromURL(u)
			for _, p := range params {
				paramFrequency[p]++
				globalParams[p]++
			}
		}

		for _, wp := range wordlistParams {
			if _, exists := paramFrequency[wp]; !exists {
				paramFrequency[wp] = 0
				globalParams[wp] = 0
			}
		}

		sortedParams := sortParamsByFrequency(paramFrequency)
		paramsFile := filepath.Join(dir, "params.txt")
		writeParamsToFile(paramsFile, sortedParams)

		topN := 5
		if len(sortedParams) < topN {
			topN = len(sortedParams)
		}
		topList := ""
		for i := 0; i < topN; i++ {
			topList += fmt.Sprintf("%s(%d) ", sortedParams[i].Name, sortedParams[i].Count)
		}
		logger.Success("Discovered %d total params for %s. Top %d: %s", len(sortedParams), domain, topN, topList)
	}

	if len(targets) > 1 {
		for _, wp := range wordlistParams {
			if _, exists := globalParams[wp]; !exists {
				globalParams[wp] = 0
			}
		}
		sortedGlobal := sortParamsByFrequency(globalParams)
		globalFile := filepath.Join("tmp", "global_params.txt")
		writeParamsToFile(globalFile, sortedGlobal)
		logger.Success("Merged and ranked %d global parameters. Saved to %s", len(sortedGlobal), globalFile)
	}
}

func sortParamsByFrequency(m map[string]int) []ParamCount {
	var sorted []ParamCount
	for k, v := range m {
		sorted = append(sorted, ParamCount{Name: k, Count: v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Count > sorted[j].Count
	})
	return sorted
}

func writeParamsToFile(path string, params []ParamCount) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, p := range params {
		fmt.Fprintln(file, p.Name)
	}
	return nil
}

// FIXED: Expanded all if/else blocks to multiple lines to satisfy Go's strict syntax rules.
func processPipeline(urlChan <-chan collector.CollectedURL, shouldFilter bool) {
	logger.Info("Processing URLs...")
	if shouldFilter {
		logger.Info("Filter mode (-fu) is ENABLED.")
	} else {
		logger.Warning("Filter mode (-fu) is DISABLED.")
	}
	
	writers := make(map[string]*DomainWriter)
	seenFiltered := make(map[string]struct{})
	totalProcessed, filteredOut, savedUnique := 0, 0, 0

	for res := range urlChan {
		url := strings.TrimSpace(res.URL)
		if url == "" {
			continue
		}
		totalProcessed++

		safeDomain := sanitizeDomain(res.Domain)
		dw, exists := writers[safeDomain]
		if !exists {
			dw = initDomainFiles(safeDomain)
			writers[safeDomain] = dw
		}

		// FIXED: Expanded to multiple lines
		if res.Source == "wayback" && dw.Wayback != nil {
			fmt.Fprintln(dw.Wayback, url)
		} else if res.Source == "katana" && dw.Katana != nil {
			fmt.Fprintln(dw.Katana, url)
		}

		if dw.All != nil {
			fmt.Fprintln(dw.All, url)
		}

		if !strings.HasPrefix(url, "http") {
			url = "http://" + url
		}

		if shouldFilter {
			if filter.IsStaticResource(url) {
				filteredOut++
				continue
			}
		}
		
		if _, exists := seenFiltered[url]; exists {
			continue
		}

		seenFiltered[url] = struct{}{}
		savedUnique++
		
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

	logger.Success("Pipeline finished.")
	if shouldFilter {
		logger.Info("Stats: Processed %d URLs | Filtered out %d static assets | Saved %d unique valid URLs", totalProcessed, filteredOut, savedUnique)
	} else {
		logger.Info("Stats: Processed %d URLs | Saved %d unique URLs", totalProcessed, savedUnique)
	}
}

func filterExistingDomains(targets []string) {
	for _, domain := range targets {
		safeDomain := sanitizeDomain(domain)
		dir := filepath.Join("tmp", safeDomain)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			logger.Error("Dir not found: %s", dir)
			continue
		}
		logger.Info("Filtering existing URLs for %s...", domain)
		
		inFile, err := os.Open(filepath.Join(dir, "all.txt"))
		if err != nil {
			logger.Error("Could not open all.txt: %v", err)
			continue
		}
		
		outFile, _ := os.Create(filepath.Join(dir, "filtered.txt"))
		seen := make(map[string]struct{})
		scanner := bufio.NewScanner(inFile)
		countTotal, countFiltered := 0, 0

		for scanner.Scan() {
			rawURL := strings.TrimSpace(scanner.Text())
			if rawURL == "" {
				continue
			}
			countTotal++
			if !strings.HasPrefix(rawURL, "http") {
				rawURL = "http://" + rawURL
			}
			if _, exists := seen[rawURL]; exists {
				continue
			}
			if filter.IsStaticResource(rawURL) {
				continue
			}

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

func getTargets(singleTarget, filePath string) []string {
	var targets []string
	seen := make(map[string]struct{})
	addTarget := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		if _, exists := seen[t]; !exists {
			seen[t] = struct{}{}
			targets = append(targets, t)
		}
	}
	if singleTarget != "" {
		addTarget(singleTarget)
	}
	if filePath != "" {
		lines, err := readLinesFromFile(filePath)
		if err == nil {
			for _, line := range lines {
				addTarget(line)
			}
		}
	}
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			addTarget(scanner.Text())
		}
	}
	return targets
}

func readLinesFromFile(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func loadWordlists() []string {
	var words []string
	seen := make(map[string]struct{})
	files := []string{"wordlists/params", "wordlists/top-xss-parameter.txt"}
	for _, file := range files {
		lines, err := readLinesFromFile(file)
		if err != nil {
			continue
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if _, exists := seen[line]; !exists {
				seen[line] = struct{}{}
				words = append(words, line)
			}
		}
	}
	return words
}

func mapKeysToSlice(m map[string]struct{}) []string {
	var slice []string
	for k := range m {
		slice = append(slice, k)
	}
	return slice
}
