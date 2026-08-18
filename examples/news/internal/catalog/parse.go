package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Parse reads a catalog from memory and validates it. The reader app embeds its
// suggestion catalog in the binary, so there is no file to read; Load is the
// same operation against a path.
func Parse(data []byte) (*Catalog, error) {
	var c Catalog
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("catalog: %w", err)
	}
	return &c, nil
}
