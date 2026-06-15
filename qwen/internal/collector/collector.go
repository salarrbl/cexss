package collector

// Collector is an interface that defines the contract for all URL collection engines.
// Any struct that implements the Fetch method automatically satisfies this interface.
// This allows us to treat Wayback, Katana, and CommonCrawl exactly the same way in main.go.
type Collector interface {
	// Fetch takes a target (usually a domain like "example.com")
	// and sends all discovered URLs into the 'out' channel.
	// It returns an error if the collection process fails.
	Fetch(target string, out chan<- string) error
}
