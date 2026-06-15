package filter

import (
	"net/url"
	"strings"
)

// BlacklistedExtensions contains standard static file extensions.
var BlacklistedExtensions = []string{
	".json", ".js", ".css", ".jpg", ".jpeg", ".png", ".svg", ".gif", ".mp4", ".mp3",
	".pdf", ".doc", ".exe", ".zip", ".xml", ".woff", ".woff2", ".ttf", ".otf", ".ico",
	".bmp", ".eot", ".flv", ".webm", ".webp", ".ppt", ".pptx", ".scss", ".tif", ".tiff",
	".m4a", ".m4p", ".fnt", ".ogg", ".ogv", ".wmv", ".mov", ".rtf", ".swf", ".htc",
	".image", ".rf", ".txt", ".msi",".jpg%5C",".jpeg%5C",
}

// BlacklistedSuffixes handles your specific request for hyphenated image names 
// (e.g., "thumbnail-800x600-jpg").
var BlacklistedSuffixes = []string{
	"-jpg", "-jpeg", "-png", "-gif",
}

// IsStaticResource checks if the URL points to a file type we don't want to scan.
func IsStaticResource(rawURL string) bool {
	// 1. Parse the URL to isolate the Path from Query Parameters.
	// Example: "http://site.com/img.jpg?size=large" -> Path is "/img.jpg"
	u, err := url.Parse(rawURL)
	if err != nil {
		// If the URL is completely malformed, fallback to checking the raw string.
		return checkRawString(rawURL)
	}

	// Get the path and convert to lowercase so ".JPG" matches ".jpg"
	path := strings.ToLower(u.Path)
	if path == "" || path == "/" {
		return false // No path means it's just a domain root, not a static file.
	}

	// 2. Check standard extensions (e.g., .jpg, .css)
	for _, ext := range BlacklistedExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}

	// 3. Check hyphenated suffixes (e.g., -jpg, -png)
	for _, suffix := range BlacklistedSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}

	return false
}

// checkRawString is a fallback if url.Parse fails.
func checkRawString(rawURL string) bool {
	u := strings.ToLower(rawURL)
	
	// Strip query string manually for the raw check
	if idx := strings.Index(u, "?"); idx != -1 {
		u = u[:idx]
	}
	
	for _, ext := range BlacklistedExtensions {
		if strings.HasSuffix(u, ext) {
			return true
		}
	}
	for _, suffix := range BlacklistedSuffixes {
		if strings.HasSuffix(u, suffix) {
			return true
		}
	}
	return false
}
