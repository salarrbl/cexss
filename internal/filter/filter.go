package filter

import (
	"net/url"
	"strings"
)

func IsValid(rawURL string) bool {
	if rawURL == "" || !strings.HasPrefix(rawURL, "http") {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	return true
}

func HostOnly(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}

func UniquePath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host + u.Path
}
