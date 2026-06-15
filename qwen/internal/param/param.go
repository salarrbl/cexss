package param

import (
	"net/url"
)

// Discoverer is the engine for finding parameters.
type Discoverer struct{}

// NewDiscoverer creates a new instance.
func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

// ExtractFromURL extracts parameter names from the query string of a URL.
// Example: "http://site.com/search?q=test&page=2" -> returns ["q", "page"]
// This requires ZERO network requests because the parameters are already in the URL!
func (d *Discoverer) ExtractFromURL(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	
	var params []string
	// u.Query() returns a map of all parameters. We just need the keys.
	for key := range u.Query() {
		params = append(params, key)
	}
	return params
}

// GetUniquePaths takes a massive list of URLs and returns a deduplicated list 
// based ONLY on their Path. 
// THIS IS THE SECRET TO SPEED: If we have 10,000 URLs, they might only have 200 unique paths.
// We only need to fetch the HTML/JS for those 200 paths, saving 98% of our network requests!
func GetUniquePaths(urls []string) []string {
	seen := make(map[string]struct{})
	var uniqueURLs []string
	
	for _, rawURL := range urls {
		u, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		
		// Create a unique key using Host + Path.
		// We ignore the query string (?q=1) and fragment (#section).
		// Example: "http://site.com/search?q=1" and "http://site.com/search?q=2" 
		// both result in the uniqueKey: "site.com/search"
		uniqueKey := u.Host + u.Path
		
		if _, exists := seen[uniqueKey]; !exists {
			seen[uniqueKey] = struct{}{} // Mark as seen
			// We save the full rawURL so we can fetch it later if needed
			uniqueURLs = append(uniqueURLs, rawURL) 
		}
	}
	return uniqueURLs
}
