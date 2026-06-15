package urlgen

import (
	"net/url"
)

// Generator handles the creation of test URLs based on strategies and mutation.
type Generator struct {
	MaxParams    int
	Payloads     []string // Your specific list: <b/cexss, cexss"", etc.
	MutationMode string   // "replace" or "suffix"
}

// NewGenerator creates a new instance.
func NewGenerator(maxParams int, payloads []string, mutationMode string) *Generator {
	return &Generator{
		MaxParams:    maxParams,
		Payloads:     payloads,
		MutationMode: mutationMode,
	}
}

// Generate applies the chosen strategy and iterates through ALL payloads.
func (g *Generator) Generate(baseURL string, existingParams []string, allParams []string, strategy string) []string {
	var generatedURLs []string

	u, err := url.Parse(baseURL)
	if err != nil {
		return generatedURLs
	}

	// 1. Determine which parameters to test based on Strategy
	var paramsToTest []string
	switch strategy {
	case "normal":
		paramsToTest = existingParams
	case "combine":
		paramSet := make(map[string]struct{})
		for _, p := range existingParams { paramSet[p] = struct{}{} }
		paramsToTest = append(paramsToTest, existingParams...)
		for _, p := range allParams {
			if _, exists := paramSet[p]; !exists {
				paramsToTest = append(paramsToTest, p)
				paramSet[p] = struct{}{}
			}
		}
	case "ignore":
		paramsToTest = allParams
		u.RawQuery = "" // Clear original query for 'ignore' mode
	case "all":
		// Recursively call for all strategies and merge results
		generatedURLs = append(generatedURLs, g.Generate(baseURL, existingParams, allParams, "normal")...)
		generatedURLs = append(generatedURLs, g.Generate(baseURL, existingParams, allParams, "combine")...)
		generatedURLs = append(generatedURLs, g.Generate(baseURL, existingParams, allParams, "ignore")...)
		return generatedURLs
	}

	if len(paramsToTest) == 0 && strategy != "ignore" {
		return generatedURLs
	}

	// 2. Chunk the parameters (Max 25 per URL)
	chunks := chunkSlice(paramsToTest, g.MaxParams)

	// 3. CRITICAL: Iterate through EVERY PAYLOAD
	for _, payload := range g.Payloads {
		
		if len(chunks) == 0 {
			continue
		}

		for _, chunk := range chunks {
			// Create a copy of the URL query for this specific generation
			q := u.Query()

			// Apply the payload to the parameters in this chunk
			for _, param := range chunk {
				// FIXED: Removed the unused 'isExisting' variable.
				// We just grab the original value if it exists. If it doesn't, originalVal stays "".
				originalVal := ""
				for _, ep := range existingParams {
					if param == ep {
						originalVal = u.Query().Get(ep)
						break
					}
				}

				// Apply Mutation (Replace or Suffix)
				var finalValue string
				if g.MutationMode == "replace" {
					finalValue = payload
				} else if g.MutationMode == "suffix" {
					finalValue = originalVal + payload
				} else {
					finalValue = payload // Default to replace
				}

				// Set the value. 
				// Note: url.Values.Set automatically URL-encodes special chars like " and <
				q.Set(param, finalValue)
			}

			// Rebuild URL
			u.RawQuery = q.Encode()
			generatedURLs = append(generatedURLs, u.String())
		}
	}

	return generatedURLs
}

// chunkSlice breaks a large slice into smaller chunks.
func chunkSlice(slice []string, chunkSize int) [][]string {
	var chunks [][]string
	for i := 0; i < len(slice); i += chunkSize {
		end := i + chunkSize
		if end > len(slice) { end = len(slice) }
		chunks = append(chunks, slice[i:end])
	}
	return chunks
}

// GetExistingParams extracts parameter names from a URL.
func GetExistingParams(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil { return nil }
	var params []string
	for key := range u.Query() { params = append(params, key) }
	return params
}
