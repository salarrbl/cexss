package config

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	CollectURLs bool
	FilterURLs  bool
	ParamSearch bool
	URLGen      bool
	RunNuclei   bool

	Strategy      string
	StrategyValue string
	TemplatePath  string
	TargetFile    string
	Target        string

	Concurrency int
	MaxParams   int
	OutputDir   string
	Verbose     bool
}

func Parse() *Config {
	cfg := &Config{
		Strategy:      "normal",
		StrategyValue: "replace",
		TemplatePath:  "payloads/detect-Cexss.yaml",
		Concurrency:   20,
		MaxParams:     25,
		OutputDir:     "tmp",
	}

	flag.BoolVar(&cfg.CollectURLs, "uc", false, "URL Collection mode (GAU + Katana)")
	flag.BoolVar(&cfg.FilterURLs, "fu", false, "Filter static resources from URLs")
	flag.BoolVar(&cfg.ParamSearch, "ps", false, "Discover & rank parameters")
	flag.BoolVar(&cfg.URLGen, "ug", false, "Generate test URLs for XSS fuzzing")
	flag.BoolVar(&cfg.RunNuclei, "nuclei", false, "Run Nuclei scan on generated URLs")

	flag.StringVar(&cfg.Strategy, "strategy", "normal", "Strategy: normal, combine, ignore, all")
	flag.StringVar(&cfg.StrategyValue, "strategy_value", "replace", "Mutation mode: replace, suffix, prefix")
	flag.StringVar(&cfg.TemplatePath, "t", "payloads/detect-Cexss.yaml", "Path to Nuclei template")
	flag.StringVar(&cfg.TargetFile, "f", "", "File containing targets (one per line)")

	flag.IntVar(&cfg.Concurrency, "c", 20, "Concurrency level for workers")
	flag.IntVar(&cfg.MaxParams, "mp", 25, "Max parameters per generated URL chunk")
	flag.BoolVar(&cfg.Verbose, "v", false, "Verbose debug logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `  cexss — Advanced XSS Automation v2

  TARGET INPUT (accepts all formats):
    cexss [flags] example.com
    cexss [flags] https://example.com/path?q=1
    cexss [flags] -f targets.txt
    cat domains.txt | cexss [flags] -

  PIPELINE (domain-by-domain, per domain):
    Katana ─┐
             ├─ Merge → Dedup → Filter → Params → Encode Check → Generate → Nuclei
    GAU ────┘

  MODES (combine any):
    -uc        Collect URLs from GAU (GetAllURLs) + Katana crawler
    -fu        Filter out static assets (images, CSS, JS, fonts, etc.)
    -ps        Discover & rank parameters from URLs + wordlists
    -ug        Generate XSS test URLs with payloads for every parameter
    -nuclei    Run Nuclei scanner on generated URLs

  STRATEGIES (-strategy):
    normal    Only use parameters already in the URL     (default)
    combine   URL params + wordlist params combined
    ignore    Strip URL params, use wordlist params only
    all       Run normal + combine + ignore (most coverage)

  MUTATIONS (-strategy_value):
    replace   Replace param value with payload            (default)
    suffix    Append payload to existing param value
    prefix    Prepend payload before existing param value

  SETTINGS:
    -t string     Nuclei YAML template path               (default: payloads/detect-Cexss.yaml)
    -f string     File with targets (one per line)
    -c int        Concurrency level                        (default: 20)
    -mp int       Max parameters per generated URL chunk   (default: 25)
    -v            Verbose debug output

  EXAMPLES:
    cexss -uc example.com
      Collect URLs only (GAU + Katana)

    cexss -uc -fu -ps -ug example.com
      Full pipeline: collect → filter → params → generate

    cexss -uc -fu -ps -ug -nuclei example.com
      Full pipeline with Nuclei scanning

    cexss -uc -fu -ps -ug -f domains.txt
      Full pipeline for multiple targets from file

    cexss -ug -strategy all -strategy_value prefix example.com
      Max coverage: all strategies, prefix mutation

    cexss -ug -f targets.txt
      Generate URLs from existing collected data (rerun)
`)
	}

	flag.Parse()

	if flag.NArg() > 0 {
		cfg.Target = flag.Arg(0)
	}

	return cfg
}

func (c *Config) Validate() error {
	validStrategies := map[string]bool{"normal": true, "combine": true, "ignore": true, "all": true}
	if !validStrategies[c.Strategy] {
		return fmt.Errorf("invalid strategy: %s (valid: normal, combine, ignore, all)", c.Strategy)
	}

	validMutations := map[string]bool{"replace": true, "suffix": true, "prefix": true}
	if !validMutations[c.StrategyValue] {
		return fmt.Errorf("invalid mutation: %s (valid: replace, suffix, prefix)", c.StrategyValue)
	}

	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	if c.MaxParams < 1 {
		c.MaxParams = 1
	}

	return nil
}
