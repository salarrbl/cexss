package executor

import (
	"crypto/tls"
	"io"
	"net/http"
	"strings"
	"time"
)

type Prober struct {
	client *http.Client
}

type ProbeResult struct {
	URL           string
	StatusCode    int
	ContentLength int
	Reflected     bool
	Error         error
}

func NewProber(timeout time.Duration) *Prober {
	return &Prober{
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (p *Prober) Probe(rawURL string, payload string) ProbeResult {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return ProbeResult{URL: rawURL, Error: err}
	}

	req.Header.Set("User-Agent", "Cexss/2.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return ProbeResult{URL: rawURL, Error: err}
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*64))
	if readErr != nil {
		return ProbeResult{URL: rawURL, StatusCode: resp.StatusCode, Error: readErr}
	}

	reflected := false
	if payload != "" {
		reflected = strings.Contains(string(body), payload)
	}

	return ProbeResult{
		URL:           rawURL,
		StatusCode:    resp.StatusCode,
		ContentLength: int(resp.ContentLength),
		Reflected:     reflected,
	}
}
