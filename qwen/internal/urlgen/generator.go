package urlgen

import (
	"net/url"

	"github.com/salarrbl/cexss/internal/mutation"
)

type Generator struct {
	MaxParams int
	Payloads  []string
	Mutator   *mutation.Mutator
}

func NewGenerator(maxParams int, payloads []string, mutator *mutation.Mutator) *Generator {
	return &Generator{
		MaxParams: maxParams,
		Payloads:  payloads,
		Mutator:   mutator,
	}
}

func (g *Generator) Generate(baseURL string, existingParams []string, allParams []string, strategy string) []string {
	var generated []string

	u, err := url.Parse(baseURL)
	if err != nil {
		return generated
	}

	paramsToTest := g.selectParams(u, existingParams, allParams, strategy)

	if strategy == "all" {
		generated = append(generated, g.Generate(baseURL, existingParams, allParams, "normal")...)
		generated = append(generated, g.Generate(baseURL, existingParams, allParams, "combine")...)
		generated = append(generated, g.Generate(baseURL, existingParams, allParams, "ignore")...)
		return generated
	}

	if strategy == "ignore" {
		u.RawQuery = ""
	}

	if len(paramsToTest) == 0 && strategy != "ignore" {
		return generated
	}

	chunks := chunkSlice(paramsToTest, g.MaxParams)

	for _, payload := range g.Payloads {
		for _, chunk := range chunks {
			q := u.Query()

			for _, param := range chunk {
				origVal := q.Get(param)
				val := g.Mutator.Apply(origVal, payload)
				q.Set(param, val)
			}

			u.RawQuery = q.Encode()
			generated = append(generated, u.String())
		}
	}

	return generated
}

func (g *Generator) selectParams(u *url.URL, existingParams, allParams []string, strategy string) []string {
	switch strategy {
	case "normal":
		return existingParams
	case "combine":
		paramSet := map[string]struct{}{}
		for _, p := range existingParams {
			paramSet[p] = struct{}{}
		}
		combined := make([]string, len(existingParams))
		copy(combined, existingParams)
		for _, p := range allParams {
			if _, exists := paramSet[p]; !exists {
				paramSet[p] = struct{}{}
				combined = append(combined, p)
			}
		}
		return combined
	case "ignore":
		return allParams
	default:
		return existingParams
	}
}

func chunkSlice(slice []string, chunkSize int) [][]string {
	if chunkSize < 1 {
		chunkSize = 1
	}
	var chunks [][]string
	for i := 0; i < len(slice); i += chunkSize {
		end := i + chunkSize
		if end > len(slice) {
			end = len(slice)
		}
		chunks = append(chunks, slice[i:end])
	}
	return chunks
}

func GetExistingParams(rawURL string) []string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	var params []string
	for key := range u.Query() {
		params = append(params, key)
	}
	return params
}
