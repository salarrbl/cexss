package collector

// CollectedURL holds the URL and metadata about where it came from.
// We use a struct so the main pipeline knows which file to save it to.
type CollectedURL struct {
	URL    string // The actual URL
	Source string // "wayback" or "katana"
	Domain string // The target domain (e.g., "example.com")
}

// Collector is the interface for all URL collection engines.
// Notice we changed the channel type from `chan<- string` to `chan<- CollectedURL`.
type Collector interface {
	Fetch(target string, out chan<- CollectedURL) error
}
