package filter

import (
	"strings"
)

// BlacklistedExtensions is the list you provided to ignore
var BlacklistedExtensions = []string{
	".json", ".js", ".css", ".jpg", ".jpeg", ".png", ".svg", ".gif", ".mp4", ".mp3",
	".pdf", ".doc", ".exe", ".zip", ".xml", ".woff", ".woff2", ".ttf", ".otf", ".ico",
	".bmp", ".eot", ".flv", ".webm", ".webp", ".ppt", ".pptx", ".scss", ".tif", ".tiff",
	".m4a", ".m4p", ".fnt", ".ogg", ".ogv", ".wmv", ".mov", ".rtf", ".swf", ".htc",
	".image", ".rf", ".txt", ".msi",
}

// IsStaticResource checks if the URL points to a file type we don't want to scan
func IsStaticResource(url string) bool {
	// Convert to lowercase to catch .JPG as well as .jpg
	u := strings.ToLower(url)
	
	for _, ext := range BlacklistedExtensions {
		if strings.HasSuffix(u, ext) {
			return true
		}
	}
	return false
}
