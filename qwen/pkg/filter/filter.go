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
	".image", ".rf", ".txt", ".msi",".apk",
}

// BlacklistedSuffixes handles hyphenated AND underscored image names.
var BlacklistedSuffixes = []string{
	"-jpg", "-jpeg", "-png", "-gif",
	"_jpg", "_jpeg", "_png", "_gif", // NEW: Added underscore versions!
}

// IsStaticResource checks if the URL points to a file type we don't want to scan.
func IsStaticResource(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return checkRawString(rawURL)
	}

	pathStr := strings.ToLower(u.Path)
	if pathStr == "" || pathStr == "/" {
		return false
	}

	// CRITICAL FIX: Strip trailing slashes, backslashes, and whitespace.
	// This perfectly handles URLs ending in "%5C" (backslash) or "%2F" (slash).
	// Example: "image.jpg%5C%5C%5C" becomes "image.jpg"
	pathStr = strings.TrimRight(pathStr, "/\\ \t\n\r")

	// 1. Check standard extensions (e.g., ".jpg")
	for _, ext := range BlacklistedExtensions {
		if strings.HasSuffix(pathStr, ext) {
			return true
		}
	}

	// 2. Check hyphenated/underscored suffixes (e.g., "-jpg", "_jpg")
	for _, suffix := range BlacklistedSuffixes {
		if strings.HasSuffix(pathStr, suffix) {
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
	
	// Apply the same trailing garbage cleanup here
	u = strings.TrimRight(u, "/\\ \t\n\r")
	
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
