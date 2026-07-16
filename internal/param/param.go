package param

import (
	"net/url"
	"regexp"
	"strings"
)

// Discoverer is the engine for finding parameters.
type Discoverer struct{}

// NewDiscoverer creates a new instance.
func NewDiscoverer() *Discoverer {
	return &Discoverer{}
}

// ExtractFromURL extracts parameter names from the query string of a URL.
func (d *Discoverer) ExtractFromURL(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	
	var params []string
	for key := range u.Query() {
		params = append(params, key)
	}
	return params
}

// formParamRegex is a highly optimized Regular Expression.
// (?i) makes it case-insensitive.
// It looks for <input, <select, or <textarea tags, and captures whatever is inside name="..." or name='...'
var formParamRegex = regexp.MustCompile(`(?i)<(?:input|select|textarea)[^>]+name=["']([^"']+)["']`)

// ExtractFromHTML parses an HTML body string and returns a list of parameter names found in forms.
func (d *Discoverer) ExtractFromHTML(htmlBody string) []string {
	// FindAllStringSubmatch returns a slice of all matches. 
	// match[0] is the full tag (e.g., <input name="user">), match[1] is just "user".
	matches := formParamRegex.FindAllStringSubmatch(htmlBody, -1)
	
	var params []string
	seen := make(map[string]struct{})
	
	for _, match := range matches {
		if len(match) > 1 {
			// Clean up the parameter name: lowercase it and remove whitespace
			paramName := strings.ToLower(strings.TrimSpace(match[1]))
			
			// Ignore useless HTML button/submit names that aren't real data parameters
			if paramName == "" || paramName == "submit" || paramName == "button" || paramName == "reset" {
				continue
			}
			
			// Deduplicate parameters found on the same page
			if _, exists := seen[paramName]; !exists {
				seen[paramName] = struct{}{}
				params = append(params, paramName)
			}
		}
	}
	return params
}

// GetUniquePaths takes a massive list of URLs and returns a deduplicated list based ONLY on their Path.
func GetUniquePaths(urls []string) []string {
	seen := make(map[string]struct{})
	var uniqueURLs []string
	
	for _, rawURL := range urls {
		u, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		
		uniqueKey := u.Host + u.Path
		
		if _, exists := seen[uniqueKey]; !exists {
			seen[uniqueKey] = struct{}{}
			uniqueURLs = append(uniqueURLs, rawURL) 
		}
	}
	return uniqueURLs
}
