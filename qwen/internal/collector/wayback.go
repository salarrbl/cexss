package collector

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/salarrbl/cexss/pkg/logger"
)

// WaybackCollector fetches historical URLs from the Wayback Machine CDX API.
type WaybackCollector struct {
	// We include an http.Client so we can set timeouts and reuse connections.
	client *http.Client
}

// NewWaybackCollector creates a new instance with a configured HTTP client.
func NewWaybackCollector() *WaybackCollector {
	return &WaybackCollector{
		client: &http.Client{
			Timeout: 30 * time.Second, // Prevent the tool from hanging forever
		},
	}
}

// Fetch implements the Collector interface for the Wayback Machine.
func (w *WaybackCollector) Fetch(target string, out chan<- string) error {
	// The CDX API endpoint. 
	// url=*.%s means "give me all URLs for this domain and all subdomains".
	// output=json returns JSON instead of plain text.
	// fl=original means "only give me the original URL column".
	// collapse=urlkey removes exact duplicate URLs from the API response to save bandwidth.
	apiURL := fmt.Sprintf("http://web.archive.org/cdx/search/cdx?url=*.%s&output=json&fl=original&collapse=urlkey", target)
	
	logger.Info("Collecting URLs from Wayback for %s...", target)

	// Make the HTTP GET request
	resp, err := w.client.Get(apiURL)
	if err != nil {
		return fmt.Errorf("wayback request failed: %w", err)
	}
	// defer ensures the response body is closed when the function finishes, 
	// preventing memory/connection leaks.
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wayback returned non-200 status: %d", resp.StatusCode)
	}

	// The CDX API returns a JSON array of arrays. 
	// The very first row is the header (e.g., ["original"]), 
	// and the subsequent rows are the actual URLs (e.g., ["http://example.com/page"]).
	var results [][]string
	
	// json.NewDecoder streams the JSON directly from the network response into our variable.
	// This is much more memory-efficient than reading the whole body into a byte slice first.
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return fmt.Errorf("failed to decode wayback json: %w", err)
	}

	// We start at index 1 to skip the header row.
	for i := 1; i < len(results); i++ {
		// Safety check: ensure the row actually has data
		if len(results[i]) > 0 {
			// Send the URL down the pipeline channel
			out <- results[i][0]
		}
	}

	logger.Success("Wayback collection finished for %s", target)
	return nil
}
