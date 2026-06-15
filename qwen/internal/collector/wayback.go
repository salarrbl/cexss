package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/salarrbl/cexss/pkg/logger"
)

type WaybackCollector struct {
	client *http.Client
}

func NewWaybackCollector() *WaybackCollector {
	return &WaybackCollector{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *WaybackCollector) Fetch(target string, out chan<- CollectedURL) error {
	apiURL := fmt.Sprintf("http://web.archive.org/cdx/search/cdx?url=*.%s&output=json&fl=original&collapse=urlkey", target)
	
	// We removed the "Collecting URLs..." log to keep the console clean.
	
	resp, err := w.client.Get(apiURL)
	if err != nil {
		// RED LOG: If it fails, we log it in red and return the error.
		logger.Error("Wayback failed for %s: %v", target, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error("Wayback failed for %s: status %d", target, resp.StatusCode)
		return fmt.Errorf("wayback returned status %d", resp.StatusCode)
	}

	var results [][]string
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		logger.Error("Wayback failed for %s: json decode error", target)
		return err
	}

	count := 0
	for i := 1; i < len(results); i++ {
		if len(results[i]) > 0 {
			// Send the struct with metadata!
			out <- CollectedURL{
				URL:    results[i][0],
				Source: "wayback",
				Domain: target,
			}
			count++
		}
	}

	// GREEN LOG: Only log when completely finished.
	logger.Success("Wayback done for %s (%d URLs)", target, count)
	return nil
}
