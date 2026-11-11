package problem

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const maxReferenceCatalog = 64 << 10

// The source lock authenticates these bytes. Independently derive the complete
// sequence rather than trusting a re-versioned draft's declared subset/order.
func validateReferenceCatalog(d document, info, rawHashes []byte, maximum int) error {
	expected, hashes, err := referenceCatalog(info, rawHashes, maximum)
	if err != nil || !validDocumentCaseMapping(d) || len(d.Provenance.Mapping) != len(expected) {
		return ErrReferenceUnsupported
	}
	for i, c := range expected {
		point := d.TestCases[i]
		if d.Provenance.Mapping[i].UpstreamCase != c.name || point.Sample != c.sample || point.Input.SHA256 != hashes[c.name+".in"] || point.Answer.SHA256 != hashes[c.name+".out"] {
			return ErrReferenceUnsupported
		}
	}
	return nil
}

type referenceCatalogCase struct {
	name   string
	sample bool
}

func referenceCatalog(info, rawHashes []byte, maximum int) ([]referenceCatalogCase, map[string]string, error) {
	var config struct {
		Tests []struct {
			Name   string `toml:"name"`
			Number int    `toml:"number"`
		} `toml:"tests"`
	}
	if toml.Unmarshal(info, &config) != nil || len(config.Tests) == 0 || len(config.Tests) > maximum {
		return nil, nil, ErrReferenceUnsupported
	}
	var expected []referenceCatalogCase
	seen := map[string]bool{}
	for _, group := range config.Tests {
		if !validPath(group.Name) || path.Base(group.Name) != group.Name || group.Number < 1 || group.Number > 100 || len(expected)+group.Number > maximum {
			return nil, nil, ErrReferenceUnsupported
		}
		prefix := strings.TrimSuffix(group.Name, path.Ext(group.Name))
		if prefix == "" {
			return nil, nil, ErrReferenceUnsupported
		}
		for i := 0; i < group.Number; i++ {
			name := fmt.Sprintf("%s_%02d", prefix, i)
			if seen[name] {
				return nil, nil, ErrReferenceUnsupported
			}
			seen[name] = true
			expected = append(expected, referenceCatalogCase{name: name, sample: group.Name == "example.in"})
		}
	}
	hashes, err := referenceHashes(rawHashes, 2*maximum)
	if err != nil || len(hashes) != 2*len(expected) {
		return nil, nil, ErrReferenceUnsupported
	}
	return expected, hashes, nil
}

// Decode the narrow hash dictionary with explicit duplicate-key rejection.
func referenceHashes(raw []byte, maximum int) (map[string]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrReferenceUnsupported
	}
	hashes := map[string]string{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, ErrReferenceUnsupported
		}
		key, ok := token.(string)
		if !ok || len(hashes) >= maximum {
			return nil, ErrReferenceUnsupported
		}
		if _, exists := hashes[key]; exists {
			return nil, ErrReferenceUnsupported
		}
		var value string
		if decoder.Decode(&value) != nil || !validHash(value) {
			return nil, ErrReferenceUnsupported
		}
		hashes[key] = value
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrReferenceUnsupported
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, ErrReferenceUnsupported
	}
	return hashes, nil
}
