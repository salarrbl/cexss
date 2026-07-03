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

	flag.BoolVar(&cfg.CollectURLs, "uc", false, "URL Collection mode (Wayback + Katana)")
	flag.BoolVar(&cfg.FilterURLs, "fu", false, "Filter static resources from URLs")
	flag.BoolVar(&cfg.ParamSearch, "ps", false, "Discover & rank parameters")
	flag.BoolVar(&cfg.URLGen, "ug", false, "Generate test URLs for XSS fuzzing")
	flag.BoolVar(&cfg.RunNuclei, "nuclei", false, "Run Nuclei scan on generated URLs")

	flag.StringVar(&cfg.Strategy, "strategy", "normal", "Strategy: normal, combine, ignore, all")
	flag.StringVar(&cfg.StrategyValue, "strategy_value", "replace", "Mutation mode: replace, suffix")
	flag.StringVar(&cfg.TemplatePath, "t", "payloads/detect-Cexss.yaml", "Path to Nuclei template")
	flag.StringVar(&cfg.TargetFile, "f", "", "File containing targets (one per line)")

	flag.IntVar(&cfg.Concurrency, "c", 20, "Concurrency level for workers")
	flag.IntVar(&cfg.MaxParams, "mp", 25, "Max parameters per generated URL chunk")
	flag.BoolVar(&cfg.Verbose, "v", false, "Verbose debug logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `  cexss — Advanced XSS Automation v2

  MODES:
    -uc        Collect URLs from Wayback Machine + Katana crawler
    -fu        Filter out static assets (images, CSS, JS, fonts, etc.)
    -ps        Discover parameters from URLs and wordlists, rank by frequency
    -ug        Generate mutated URLs with XSS payloads for every parameter
    -nuclei    Pipe generated URLs through Nuclei scanner

  SETTINGS:
    -strategy       normal | combine | ignore | all       (default: normal)
    -strategy_value replace | suffix                      (default: replace)
    -t              Path to Nuclei YAML template          (default: payloads/detect-Cexss.yaml)
    -f              File with target domains/URLs
    -c              Number of concurrent workers          (default: 20)
    -mp             Max parameters per generated URL      (default: 25)
    -v              Enable verbose debug output

  EXAMPLES:
    cexss -uc example.com                              Collect URLs only
    cexss -uc -fu -ps -ug example.com                  Full pipeline: collect → filter → params → generate
    cexss -uc -fu -ps -ug -nuclei example.com          Full pipeline + Nuclei scan
    cexss -ug -f targets.txt                           Generate URLs for multiple targets
    cexss -ug -strategy combine -strategy_value suffix example.com
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
