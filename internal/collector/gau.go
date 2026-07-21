package collector

import (
	"bufio"
	"context"
	"os/exec"
	"strings"

	"github.com/salarrbl/cexss/pkg/logger"
)

type GAUCollector struct{}

func NewGAUCollector() *GAUCollector {
	return &GAUCollector{}
}

func (g *GAUCollector) Fetch(ctx context.Context, target string, out chan<- CollectedURL) error {
	target = strings.TrimPrefix(target, "http://")
	target = strings.TrimPrefix(target, "https://")

	cmd := exec.CommandContext(ctx, "gau", target)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "error") {
				logger.Warning("GAU stderr: %s", scanner.Text())
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		logger.Warning("GAU failed for %s: %v", target, err)
		return err
	}

	scanner := bufio.NewScanner(stdout)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			out <- CollectedURL{
				URL:    line,
				Source: "gau",
				Domain: target,
			}
			count++
		}
	}

	if err := cmd.Wait(); err != nil {
		return err
	}

	if count > 0 {
		logger.Success("GAU done for %s (%d URLs)", target, count)
	}
	return nil
}
