package collector

import (
	"bufio"
	// REMOVED: "fmt" because we are using logger instead
	"os/exec"
	"strings"

	"github.com/salarrbl/cexss/pkg/logger"
)

type KatanaCollector struct{}

func NewKatanaCollector() *KatanaCollector {
	return &KatanaCollector{}
}

func (k *KatanaCollector) Fetch(target string, out chan<- CollectedURL) error {
	targetURL := target
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		targetURL = "https://" + targetURL
	}

	cmd := exec.Command("katana", "-u", targetURL, "-silent")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Error("Katana failed for %s: pipe error", target)
		return err
	}

	if err := cmd.Start(); err != nil {
		logger.Error("Katana failed for %s: %v", target, err)
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
		logger.Error("Katana failed for %s: process error", target)
		return err
	}

	logger.Success("Katana done for %s (%d URLs)", target, count)
	return nil
}
