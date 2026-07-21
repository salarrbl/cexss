package progress

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/salarrbl/cexss/pkg/logger"
)

type StageType string

const (
	StageKatana      StageType = "Katana"
	StageGAU         StageType = "GAU"
	StageMerge       StageType = "Merge & Deduplicate"
	StageFilter      StageType = "Filter Static Resources"
	StageParams      StageType = "Parameter Extraction"
	StageEncodeCheck StageType = "Encoded Verification"
	StageURLGen      StageType = "URL Generation"
	StageNuclei      StageType = "Nuclei Scan"
	StageSave        StageType = "Save Results"
	StageCleanup     StageType = "Cleanup"
)

type StageStatus int

const (
	StatusPending StageStatus = iota
	StatusRunning
	StatusDone
	StatusFailed
	StatusSkipped
)

type stageInfo struct {
	typ    StageType
	status StageStatus
	err    error
	extra  string
}

type Reporter struct {
	mu           sync.Mutex
	current      int
	total        int
	domain       string
	stages       []stageInfo
	startedAt    time.Time
	domainStart  time.Time
}

func New() *Reporter {
	return &Reporter{startedAt: time.Now()}
}

func (r *Reporter) StartDomain(current, total int, domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.current = current
	r.total = total
	r.domain = domain
	r.domainStart = time.Now()
	r.stages = []stageInfo{
		{typ: StageKatana, status: StatusPending},
		{typ: StageGAU, status: StatusPending},
		{typ: StageMerge, status: StatusPending},
		{typ: StageFilter, status: StatusPending},
		{typ: StageParams, status: StatusPending},
		{typ: StageEncodeCheck, status: StatusPending},
		{typ: StageURLGen, status: StatusPending},
		{typ: StageNuclei, status: StatusPending},
		{typ: StageSave, status: StatusPending},
	}

	fmt.Println()
	logger.Info("[%d/%d] %s", current, total, domain)
}

func (r *Reporter) StageStart(stage StageType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.stages {
		if r.stages[i].typ == stage {
			r.stages[i].status = StatusRunning
			break
		}
	}
}

func (r *Reporter) StageDone(stage StageType, extra ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.stages {
		if r.stages[i].typ == stage {
			r.stages[i].status = StatusDone
			if len(extra) > 0 {
				r.stages[i].extra = extra[0]
			}
			break
		}
	}

	r.printLine(stage, true, "")
}

func (r *Reporter) StageFailed(stage StageType, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.stages {
		if r.stages[i].typ == stage {
			r.stages[i].status = StatusFailed
			r.stages[i].err = err
			break
		}
	}

	r.printLine(stage, false, err.Error())
}

func (r *Reporter) StageSkipped(stage StageType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range r.stages {
		if r.stages[i].typ == stage {
			r.stages[i].status = StatusSkipped
			break
		}
	}
}

func (r *Reporter) DomainDone() {
	r.mu.Lock()
	defer r.mu.Unlock()

	elapsed := time.Since(r.domainStart).Round(time.Second)
	logger.Success("Domain '%s' completed in %s", r.domain, elapsed)
}

func (r *Reporter) DomainFailed(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	logger.Error("Domain '%s' failed: %v", r.domain, err)
}

func (r *Reporter) Summary() {
	r.mu.Lock()
	defer r.mu.Unlock()

	total := time.Since(r.startedAt).Round(time.Second)
	fmt.Println()
	logger.Success("Pipeline finished in %s", total)
}

func (r *Reporter) printLine(stage StageType, success bool, errMsg string) {
	icon := "✓"
	color := "\033[32m"
	prefix := ""

	if !success {
		icon = "✗"
		color = "\033[31m"
	}

	for _, s := range r.stages {
		if s.typ == stage {
			if s.extra != "" {
				prefix = fmt.Sprintf("  %s%s%s %s (%s)", color, icon, "\033[0m", s.typ, s.extra)
			} else {
				prefix = fmt.Sprintf("  %s%s%s %s", color, icon, "\033[0m", s.typ)
			}
			break
		}
	}

	if errMsg != "" {
		logger.Warning("%s — %s", prefix, errMsg)
	} else {
		fmt.Println(prefix)
	}
}

func (r *Reporter) Separator() {
	fmt.Println(strings.Repeat("─", 40))
}
