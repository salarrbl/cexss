package normalize

import (
	"net/url"
	"strings"
)

func Domain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}

func EnsureScheme(rawURL string) string {
	if strings.HasPrefix(rawURL, "http://") || strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	return "https://" + rawURL
}

func SafeFilename(name string) string {
	r := strings.NewReplacer(
		"http://", "",
		"https://", "",
		"/", "_",
		":", "_",
		"?", "_",
		"&", "_",
	)
	return r.Replace(name)
}

func StripProtocol(name string) string {
	s := strings.TrimPrefix(name, "http://")
	s = strings.TrimPrefix(s, "https://")
	return s
}

func UniqueHostPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host + u.Path
}

func Host(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func Clean(rawURL string) string {
	if !strings.HasPrefix(rawURL, "http") {
		rawURL = "https://" + rawURL
	}
	return rawURL
}
