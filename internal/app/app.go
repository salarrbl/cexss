package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/salarrbl/cexss/internal/collector"
	"github.com/salarrbl/cexss/internal/config"
	"github.com/salarrbl/cexss/internal/encoding"
	"github.com/salarrbl/cexss/internal/input"
	"github.com/salarrbl/cexss/internal/mutation"
	"github.com/salarrbl/cexss/internal/normalize"
	"github.com/salarrbl/cexss/internal/param"
	"github.com/salarrbl/cexss/internal/progress"
	"github.com/salarrbl/cexss/internal/urlgen"
	"github.com/salarrbl/cexss/pkg/filter"
	"github.com/salarrbl/cexss/pkg/logger"
)

type App struct {
	cfg     *config.Config
	targets []string
}

type domainWriter struct {
	gau      *os.File
	katana   *os.File
	all      *os.File
	filtered *os.File
}

type paramCount struct {
	Name  string
	Count int
}

func New(cfg *config.Config) *App {
	return &App{cfg: cfg}
}

func (a *App) Run() error {
	if err := a.cfg.Validate(); err != nil {
		return err
	}

	a.targets = input.Targets(a.cfg.Target, a.cfg.TargetFile)
	if len(a.targets) == 0 {
		logger.Error("No targets provided")
		os.Exit(1)
	}

	switch {
	case a.cfg.CollectURLs:
		return a.runPipeline()
	case a.cfg.FilterURLs:
		return a.runFilterExisting()
	case a.cfg.ParamSearch:
		return a.runParamDiscovery()
	case a.cfg.URLGen:
		payloads := a.loadPayloads()
		if len(payloads) == 0 {
			logger.Warning("No payloads loaded, using default")
			payloads = []string{"cexss"}
		}
		return a.runURLGeneration(payloads)
	default:
		return a.runDefault()
	}
}

// ============================================================================
// Mode: Default (URLs from args -> filter -> save)
// ============================================================================

func (a *App) runDefault() error {
	urlChan := make(chan collector.CollectedURL, 200)
	go func() {
		for _, u := range a.targets {
			urlChan <- collector.CollectedURL{URL: u, Source: "stdin", Domain: normalize.Domain(u)}
		}
		close(urlChan)
	}()
	a.processPipeline(urlChan, false)
	return nil
}

// ============================================================================
// Pipeline: Domain-by-domain full processing
// ============================================================================

func (a *App) runPipeline() error {
	rep := progress.New()
	hasSubStages := a.cfg.ParamSearch || a.cfg.URLGen || a.cfg.RunNuclei

	for i, target := range a.targets {
		rep.StartDomain(i+1, len(a.targets), target)
		err := a.processDomain(rep, target, hasSubStages)
		if err != nil {
			rep.DomainFailed(err)
			logger.Warning("Domain '%s' had errors, continuing to next...", target)
		} else {
			rep.DomainDone()
		}
		rep.Separator()
	}

	rep.Summary()
	return nil
}

func (a *App) processDomain(rep *progress.Reporter, domain string, hasSubStages bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	safeDir := filepath.Join(a.cfg.OutputDir, normalize.SafeFilename(domain))
	os.MkdirAll(safeDir, 0755)

	// Stages 1-2: Collection from Katana + GAU
	urlChan := make(chan collector.CollectedURL, 2000)
	var collectWg sync.WaitGroup
	var katanaErr, gauErr error

	collectWg.Add(1)
	go func() {
		defer collectWg.Done()
		rep.StageStart(progress.StageKatana)
		katanaCol := collector.NewKatanaCollector()
		if err := katanaCol.Fetch(ctx, domain, urlChan); err != nil {
			katanaErr = err
			rep.StageFailed(progress.StageKatana, err)
			return
		}
		rep.StageDone(progress.StageKatana)
	}()

	collectWg.Add(1)
	go func() {
		defer collectWg.Done()
		rep.StageStart(progress.StageGAU)
		gauCol := collector.NewGAUCollector()
		if err := gauCol.Fetch(ctx, domain, urlChan); err != nil {
			gauErr = err
			rep.StageFailed(progress.StageGAU, err)
			return
		}
		rep.StageDone(progress.StageGAU)
	}()

	// Stage 3-4: Merge, deduplicate, filter, and save in real-time
	filteredOut, err := os.Create(filepath.Join(safeDir, "filtered.txt"))
	if err != nil {
		return fmt.Errorf("create filtered.txt: %w", err)
	}
	allOut, err := os.Create(filepath.Join(safeDir, "all.txt"))
	if err != nil {
		filteredOut.Close()
		return fmt.Errorf("create all.txt: %w", err)
	}

	rep.StageStart(progress.StageMerge)
	rep.StageStart(progress.StageFilter)

	seenAll := map[string]struct{}{}
	seenFiltered := map[string]struct{}{}
	total, filtered, saved := 0, 0, 0

	closeCollectors := make(chan struct{})
	go func() {
		collectWg.Wait()
		close(urlChan)
		close(closeCollectors)
	}()

	for res := range urlChan {
		url := strings.TrimSpace(res.URL)
		if url == "" {
			continue
		}
		total++

		url = normalize.EnsureScheme(url)

		if _, exists := seenAll[url]; !exists {
			seenAll[url] = struct{}{}
			fmt.Fprintln(allOut, url)
		}

		if a.cfg.FilterURLs && filter.IsStaticResource(url) {
			filtered++
			continue
		}

		if _, exists := seenFiltered[url]; exists {
			continue
		}
		seenFiltered[url] = struct{}{}
		saved++
		fmt.Fprintln(filteredOut, url)
	}

	allOut.Close()
	filteredOut.Close()

	rep.StageDone(progress.StageMerge, fmt.Sprintf("%d total", total))
	if a.cfg.FilterURLs {
		rep.StageDone(progress.StageFilter, fmt.Sprintf("%d unique", saved))
	} else {
		rep.StageSkipped(progress.StageFilter)
	}

	logger.Info("Domain '%s': %d URLs collected, %d filtered, %d saved unique", domain, total, filtered, saved)

	if katanaErr != nil && gauErr != nil {
		return fmt.Errorf("both collectors failed: katana: %v, gau: %v", katanaErr, gauErr)
	}

	if !hasSubStages {
		return nil
	}

	// Stage 5: Parameter extraction
	rep.StageStart(progress.StageParams)
	if err := a.extractParamsForDomain(domain, safeDir); err != nil {
		rep.StageFailed(progress.StageParams, err)
		logger.Warning("Parameter extraction failed for %s: %v", domain, err)
	} else {
		rep.StageDone(progress.StageParams)
	}

	// Stage 6: Encoded parameter verification
	rep.StageStart(progress.StageEncodeCheck)
	encodedCount, err := a.verifyEncodedParams(domain, safeDir)
	if err != nil {
		rep.StageFailed(progress.StageEncodeCheck, err)
		logger.Warning("Encoded parameter verification failed for %s: %v", domain, err)
	} else {
		extra := fmt.Sprintf("%d encoded", encodedCount)
		rep.StageDone(progress.StageEncodeCheck, extra)
	}

	// Stage 7: URL generation
	if a.cfg.URLGen {
		rep.StageStart(progress.StageURLGen)
		payloads := a.loadPayloads()
		if len(payloads) == 0 {
			payloads = []string{"cexss"}
		}
		if err := a.generateURLsForDomain(domain, safeDir, payloads); err != nil {
			rep.StageFailed(progress.StageURLGen, err)
			logger.Warning("URL generation failed for %s: %v", domain, err)
		} else {
			rep.StageDone(progress.StageURLGen)
		}
	}

	// Stage 8: Nuclei scan
	if a.cfg.RunNuclei && a.cfg.URLGen {
		rep.StageStart(progress.StageNuclei)
		if err := a.runNucleiOnDomain(domain, safeDir); err != nil {
			rep.StageFailed(progress.StageNuclei, err)
			logger.Warning("Nuclei scan failed for %s: %v", domain, err)
		} else {
			rep.StageDone(progress.StageNuclei)
		}
	} else if a.cfg.RunNuclei {
		logger.Warning("Nuclei requires -ug flag, skipping for %s", domain)
	}

	rep.StageDone(progress.StageSave)

	return nil
}

// ============================================================================
// Pipeline helpers
// ============================================================================

func (a *App) extractParamsForDomain(domain, safeDir string) error {
	discoverer := param.NewDiscoverer()
	wordlistParams := a.loadWordlists()
	logger.Info("Loaded %d parameters from wordlists", len(wordlistParams))

	filteredFile := filepath.Join(safeDir, "filtered.txt")
	urls, err := input.ReadLines(filteredFile)
	if err != nil {
		urls, err = input.ReadLines(filepath.Join(safeDir, "all.txt"))
		if err != nil {
			return fmt.Errorf("no URL files found: %w", err)
		}
	}

	paramFreq := map[string]int{}
	for _, u := range urls {
		u = normalize.EnsureScheme(u)
		for _, p := range discoverer.ExtractFromURL(u) {
			paramFreq[p]++
		}
	}

	for _, wp := range wordlistParams {
		if _, exists := paramFreq[wp]; !exists {
			paramFreq[wp] = 0
		}
	}

	sorted := sortParams(paramFreq)
	paramsFile := filepath.Join(safeDir, "params.txt")
	a.writeParamsFile(paramsFile, sorted)

	topN := 5
	if len(sorted) < topN {
		topN = len(sorted)
	}
	topList := make([]string, topN)
	for i := 0; i < topN; i++ {
		topList[i] = fmt.Sprintf("%s(%d)", sorted[i].Name, sorted[i].Count)
	}

	logger.Success("%s: %d params | Top: %s", domain, len(sorted), strings.Join(topList, " "))
	return nil
}

func (a *App) verifyEncodedParams(domain, safeDir string) (int, error) {
	paramsFile := filepath.Join(safeDir, "params.txt")
	params, err := input.ReadLines(paramsFile)
	if err != nil {
		return 0, fmt.Errorf("read params.txt: %w", err)
	}

	filteredFile := filepath.Join(safeDir, "filtered.txt")
	urls, err := input.ReadLines(filteredFile)
	if err != nil {
		return 0, fmt.Errorf("read filtered.txt: %w", err)
	}

	detector := encoding.NewDetector()
	var allResults []encoding.Result

	paramSet := make(map[string]bool, len(params))
	for _, p := range params {
		paramSet[p] = true
	}

	for _, u := range urls {
		for paramName := range paramSet {
			val := urlgen.GetParamValue(u, paramName)
			if val == "" {
				continue
			}
			results := detector.Detect(paramName, val)
			allResults = append(allResults, results...)
		}
	}

	if len(allResults) > 0 {
		encFile := filepath.Join(safeDir, "encoded_params.txt")
		f, err := os.Create(encFile)
		if err != nil {
			return 0, fmt.Errorf("create encoded_params.txt: %w", err)
		}
		defer f.Close()

		seen := map[string]bool{}
		for _, r := range allResults {
			key := r.Param + "|" + r.Decoded
			if seen[key] {
				continue
			}
			seen[key] = true
			fmt.Fprintf(f, "param=%s | encoding=%s | original=%s | decoded=%s\n", r.Param, r.Encoding, r.Value, r.Decoded)
		}

		logger.Success("%s: %d encoded parameters verified -> %s", domain, len(seen), encFile)
		return len(seen), nil
	}

	logger.Info("%s: No encoded parameters found", domain)
	return 0, nil
}

func (a *App) generateURLsForDomain(domain, safeDir string, payloads []string) error {
	mutator := mutation.New(a.cfg.StrategyValue)
	logger.Info("Generating URLs for %s (strategy=%s mutation=%s)...", domain, a.cfg.Strategy, a.cfg.StrategyValue)

	urls, err := input.ReadLines(filepath.Join(safeDir, "filtered.txt"))
	if err != nil {
		return fmt.Errorf("read filtered.txt: %w", err)
	}

	allParams, err := input.ReadLines(filepath.Join(safeDir, "params.txt"))
	if err != nil {
		return fmt.Errorf("read params.txt: %w", err)
	}

	generator := urlgen.NewGenerator(a.cfg.MaxParams, payloads, mutator)

	xamirPath := filepath.Join(safeDir, "xamir.txt")
	out, err := os.Create(xamirPath)
	if err != nil {
		return fmt.Errorf("create xamir.txt: %w", err)
	}
	defer out.Close()

	domainGenerated := 0
	for _, u := range urls {
		u = normalize.EnsureScheme(u)
		existingParams := urlgen.GetExistingParams(u)
		generated := generator.Generate(u, existingParams, allParams, a.cfg.Strategy)

		for _, gen := range generated {
			fmt.Fprintln(out, gen)
			domainGenerated++
		}
	}

	logger.Success("%s: %d URLs generated -> %s", domain, domainGenerated, xamirPath)
	return nil
}

func (a *App) runNucleiOnDomain(domain, safeDir string) error {
	xamirPath := filepath.Join(safeDir, "xamir.txt")
	if _, err := os.Stat(xamirPath); os.IsNotExist(err) {
		return fmt.Errorf("xamir.txt not found, run URL generation first")
	}

	if _, err := os.Stat(a.cfg.TemplatePath); os.IsNotExist(err) {
		return fmt.Errorf("nuclei template not found: %s", a.cfg.TemplatePath)
	}

	resultsPath := filepath.Join(safeDir, "nuclei_results.txt")
	outFile, err := os.Create(resultsPath)
	if err != nil {
		return fmt.Errorf("create nuclei_results.txt: %w", err)
	}
	defer outFile.Close()

	logger.Info("Starting Nuclei scan for %s with template: %s", domain, a.cfg.TemplatePath)

	cmd := exec.Command("nuclei", "-l", xamirPath, "-t", a.cfg.TemplatePath, "-silent")
	cmd.Stdout = io.MultiWriter(os.Stdout, outFile)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start nuclei: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("nuclei exited with error: %w", err)
	}

	logger.Success("Nuclei scan completed for %s -> %s", domain, resultsPath)
	return nil
}

// ============================================================================
// Pipeline: writes URLs to per-domain files (shared by runDefault and domain pipeline)
// ============================================================================

func (a *App) processPipeline(urlChan <-chan collector.CollectedURL, shouldFilter bool) {
	logger.Info("Processing URL stream...")
	if shouldFilter {
		logger.Info("Static resource filtering is ENABLED")
	} else {
		logger.Warning("Static resource filtering is DISABLED")
	}

	writers := map[string]*domainWriter{}
	seenFiltered := map[string]struct{}{}
	total, filtered, saved := 0, 0, 0

	for res := range urlChan {
		url := strings.TrimSpace(res.URL)
		if url == "" {
			continue
		}
		total++

		safeDomain := normalize.SafeFilename(res.Domain)
		dw, ok := writers[safeDomain]
		if !ok {
			dw = a.initDomainFiles(safeDomain)
			writers[safeDomain] = dw
		}

		switch res.Source {
		case "gau", "wayback":
			if dw.gau != nil {
				fmt.Fprintln(dw.gau, url)
			}
		case "katana":
			if dw.katana != nil {
				fmt.Fprintln(dw.katana, url)
			}
		}
		if dw.all != nil {
			fmt.Fprintln(dw.all, url)
		}

		url = normalize.EnsureScheme(url)

		if shouldFilter && filter.IsStaticResource(url) {
			filtered++
			continue
		}

		if _, exists := seenFiltered[url]; exists {
			continue
		}
		seenFiltered[url] = struct{}{}
		saved++

		if dw.filtered != nil {
			fmt.Fprintln(dw.filtered, url)
		}
	}

	for _, dw := range writers {
		if dw.gau != nil {
			dw.gau.Close()
		}
		if dw.katana != nil {
			dw.katana.Close()
		}
		if dw.all != nil {
			dw.all.Close()
		}
		if dw.filtered != nil {
			dw.filtered.Close()
		}
	}

	logger.Success("Pipeline completed")
	logger.Info("Processed: %d | Filtered: %d | Saved: %d unique", total, filtered, saved)
}

func (a *App) initDomainFiles(domain string) *domainWriter {
	dir := filepath.Join(a.cfg.OutputDir, domain)
	os.MkdirAll(dir, 0755)
	return &domainWriter{
		gau:      a.createFile(filepath.Join(dir, "gau.txt")),
		katana:   a.createFile(filepath.Join(dir, "katana.txt")),
		all:      a.createFile(filepath.Join(dir, "all.txt")),
		filtered: a.createFile(filepath.Join(dir, "filtered.txt")),
	}
}

func (a *App) createFile(path string) *os.File {
	f, err := os.Create(path)
	if err != nil {
		logger.Warning("Could not create %s: %v", path, err)
		return nil
	}
	return f
}

// ============================================================================
// Mode: Filter existing files (-fu standalone)
// ============================================================================

func (a *App) runFilterExisting() error {
	for _, domain := range a.targets {
		safe := normalize.SafeFilename(domain)
		dir := filepath.Join(a.cfg.OutputDir, safe)

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			logger.Error("Directory not found: %s (run -uc first)", dir)
			continue
		}

		logger.Info("Filtering URLs for %s...", domain)

		in, err := os.Open(filepath.Join(dir, "all.txt"))
		if err != nil {
			logger.Error("Could not open all.txt: %v", err)
			continue
		}

		out, err := os.Create(filepath.Join(dir, "filtered.txt"))
		if err != nil {
			in.Close()
			continue
		}

		seen := map[string]struct{}{}
		scanner := bufio.NewScanner(in)
		total, kept := 0, 0

		for scanner.Scan() {
			raw := strings.TrimSpace(scanner.Text())
			if raw == "" {
				continue
			}
			total++

			raw = normalize.EnsureScheme(raw)
			if _, exists := seen[raw]; exists {
				continue
			}
			if filter.IsStaticResource(raw) {
				continue
			}
			seen[raw] = struct{}{}
			fmt.Fprintln(out, raw)
			kept++
		}

		in.Close()
		out.Close()
		logger.Success("%s: %d total -> %d valid URLs", domain, total, kept)
	}
	return nil
}

// ============================================================================
// Mode: Parameter Discovery (-ps standalone)
// ============================================================================

func (a *App) runParamDiscovery() error {
	discoverer := param.NewDiscoverer()
	wordlistParams := a.loadWordlists()
	logger.Info("Loaded %d parameters from wordlists", len(wordlistParams))

	globalParams := map[string]int{}

	for _, domain := range a.targets {
		safe := normalize.SafeFilename(domain)
		dir := filepath.Join(a.cfg.OutputDir, safe)

		logger.Info("Discovering parameters for %s...", domain)

		filteredFile := filepath.Join(dir, "filtered.txt")
		urls, err := input.ReadLines(filteredFile)
		if err != nil {
			urls, err = input.ReadLines(filepath.Join(dir, "all.txt"))
			if err != nil {
				logger.Error("No URL files found for %s (run -uc first)", domain)
				continue
			}
		}

		paramFreq := map[string]int{}
		for _, u := range urls {
			u = normalize.EnsureScheme(u)
			for _, p := range discoverer.ExtractFromURL(u) {
				paramFreq[p]++
				globalParams[p]++
			}
		}

		for _, wp := range wordlistParams {
			if _, exists := paramFreq[wp]; !exists {
				paramFreq[wp] = 0
				globalParams[wp] += 0
			}
		}

		sorted := sortParams(paramFreq)
		paramsFile := filepath.Join(dir, "params.txt")
		a.writeParamsFile(paramsFile, sorted)

		topN := 5
		if len(sorted) < topN {
			topN = len(sorted)
		}
		topList := make([]string, topN)
		for i := 0; i < topN; i++ {
			topList[i] = fmt.Sprintf("%s(%d)", sorted[i].Name, sorted[i].Count)
		}

		logger.Success("%s: %d params | Top: %s", domain, len(sorted), strings.Join(topList, " "))
	}

	if len(a.targets) > 1 {
		for _, wp := range wordlistParams {
			if _, exists := globalParams[wp]; !exists {
				globalParams[wp] = 0
			}
		}
		sorted := sortParams(globalParams)
		globalFile := filepath.Join(a.cfg.OutputDir, "global_params.txt")
		a.writeParamsFile(globalFile, sorted)
		logger.Success("Global params: %d -> %s", len(sorted), globalFile)
	}

	return nil
}

// ============================================================================
// Mode: URL Generation (-ug standalone)
// ============================================================================

func (a *App) runURLGeneration(payloads []string) error {
	logger.Info("Loaded %d payloads for generation", len(payloads))
	mutator := mutation.New(a.cfg.StrategyValue)
	totalGenerated := 0

	for _, domain := range a.targets {
		safe := normalize.SafeFilename(domain)
		dir := filepath.Join(a.cfg.OutputDir, safe)

		logger.Info("Generating URLs for %s (strategy=%s mutation=%s)...",
			domain, a.cfg.Strategy, a.cfg.StrategyValue)

		urls, err := input.ReadLines(filepath.Join(dir, "filtered.txt"))
		if err != nil {
			logger.Error("No filtered.txt for %s (run -uc -fu first)", domain)
			continue
		}

		allParams, err := input.ReadLines(filepath.Join(dir, "params.txt"))
		if err != nil {
			logger.Error("No params.txt for %s (run -ps first)", domain)
			continue
		}

		generator := urlgen.NewGenerator(a.cfg.MaxParams, payloads, mutator)

		xamirPath := filepath.Join(dir, "xamir.txt")
		out, err := os.Create(xamirPath)
		if err != nil {
			logger.Error("Could not create %s: %v", xamirPath, err)
			continue
		}

		domainGenerated := 0
		for _, u := range urls {
			u = normalize.EnsureScheme(u)
			existingParams := urlgen.GetExistingParams(u)
			generated := generator.Generate(u, existingParams, allParams, a.cfg.Strategy)

			for _, gen := range generated {
				fmt.Fprintln(out, gen)
				domainGenerated++
			}
		}
		out.Close()

		logger.Success("%s: %d URLs generated -> %s", domain, domainGenerated, xamirPath)
		totalGenerated += domainGenerated
	}

	logger.Success("Total: %d URLs generated across %d target(s)", totalGenerated, len(a.targets))

	if a.cfg.RunNuclei {
		a.runNuclei()
	}

	return nil
}

// ============================================================================
// Nuclei integration (standalone, after -ug)
// ============================================================================

func (a *App) runNuclei() {
	logger.Info("Starting Nuclei scan with template: %s", a.cfg.TemplatePath)

	if _, err := os.Stat(a.cfg.TemplatePath); os.IsNotExist(err) {
		logger.Error("Nuclei template not found: %s", a.cfg.TemplatePath)
		return
	}

	var xamirFiles []string
	for _, domain := range a.targets {
		path := filepath.Join(a.cfg.OutputDir, normalize.SafeFilename(domain), "xamir.txt")
		if _, err := os.Stat(path); err == nil {
			xamirFiles = append(xamirFiles, path)
		}
	}

	if len(xamirFiles) == 0 {
		logger.Error("No xamir.txt files found (run -ug first)")
		return
	}

	resultsPath := filepath.Join(a.cfg.OutputDir, "nuclei_results.txt")
	outFile, err := os.Create(resultsPath)
	if err != nil {
		logger.Error("Could not create nuclei_results.txt: %v", err)
		return
	}
	defer outFile.Close()

	tmpFile, err := os.CreateTemp("", "cexss-nuclei-*.txt")
	if err != nil {
		logger.Error("Could not create temp file: %v", err)
		return
	}
	tmpPath := tmpFile.Name()

	total := 0
	for _, f := range xamirFiles {
		lines, _ := input.ReadLines(f)
		for _, line := range lines {
			fmt.Fprintln(tmpFile, line)
			total++
		}
	}
	tmpFile.Close()

	logger.Info("Prepared %d URLs for Nuclei scan", total)

	cmd := exec.Command("nuclei", "-l", tmpPath, "-t", a.cfg.TemplatePath, "-silent")
	cmd.Stdout = io.MultiWriter(os.Stdout, outFile)
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		logger.Error("Failed to start Nuclei: %v", err)
		return
	}
	if err := cmd.Wait(); err != nil {
		logger.Error("Nuclei exited with error: %v", err)
	} else {
		logger.Success("Nuclei scan completed -> %s", resultsPath)
	}
}

// ============================================================================
// Helpers
// ============================================================================

func (a *App) loadPayloads() []string {
	lines, err := input.ReadLines("payloads/payloads.txt")
	if err != nil {
		logger.Warning("Could not load payloads/payloads.txt: %v", err)
		return []string{"cexss"}
	}
	var filtered []string
	for _, line := range lines {
		if !strings.HasPrefix(line, "#") {
			filtered = append(filtered, line)
		}
	}
	logger.Info("Loaded %d custom payloads", len(filtered))
	return filtered
}

func (a *App) loadWordlists() []string {
	var words []string
	seen := map[string]struct{}{}
	files := []string{"wordlists/params", "wordlists/top-xss-parameter.txt"}

	for _, file := range files {
		lines, err := input.ReadLines(file)
		if err != nil {
			if a.cfg.Verbose {
				logger.Warning("Could not read wordlist %s: %v", file, err)
			}
			continue
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
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

func (a *App) writeParamsFile(path string, params []paramCount) {
	f, err := os.Create(path)
	if err != nil {
		logger.Warning("Could not create %s: %v", path, err)
		return
	}
	defer f.Close()
	for _, p := range params {
		fmt.Fprintln(f, p.Name)
	}
}

func sortParams(m map[string]int) []paramCount {
	sorted := make([]paramCount, 0, len(m))
	for k, v := range m {
		sorted = append(sorted, paramCount{Name: k, Count: v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Count > sorted[j].Count
	})
	return sorted
}

func (a *App) domainFromFiles() []string {
	var domains []string
	seen := map[string]struct{}{}
	for _, t := range a.targets {
		safe := normalize.SafeFilename(t)
		dir := filepath.Join(a.cfg.OutputDir, safe)
		if _, err := os.Stat(dir); err == nil {
			if _, ok := seen[t]; !ok {
				seen[t] = struct{}{}
				domains = append(domains, t)
			}
		}
	}
	return domains
}

var _ sync.Mutex
