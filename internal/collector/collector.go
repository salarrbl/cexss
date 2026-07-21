package collector

import "context"

type CollectedURL struct {
	URL    string
	Source string
	Domain string
}

type Collector interface {
	Fetch(ctx context.Context, target string, out chan<- CollectedURL) error
}
