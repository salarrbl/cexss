package collector

import (
	"bufio"
	"context"
	"os/exec"
	"strings"

	"github.com/salarrbl/cexss/pkg/logger"
)

type KatanaCollector struct{}

func NewKatanaCollector() *KatanaCollector {
	return &KatanaCollector{}
}

func (k *KatanaCollector) Fetch(ctx context.Context, target string, out chan<- CollectedURL) error {
	targetURL := target
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	cmd := exec.CommandContext(ctx, "katana", "-u", targetURL, "-silent")

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
			line := scanner.Text()
			if strings.Contains(line, "error") {
				logger.Warning("Katana stderr: %s", line)
			}
		}
	}()

	if err := cmd.Start(); err != nil {
		logger.Warning("Katana failed for %s: %v", target, err)
		return err
	}

	scanner := bufio.NewScanner(stdout)
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			out <- CollectedURL{
				URL:    line,
				Source: "katana",
				Domain: target,
			}
			count++
		}
	}

	if err := cmd.Wait(); err != nil {
		return err
	}

	if count > 0 {
		logger.Success("Katana done for %s (%d URLs)", target, count)
	}
	return nil
}
