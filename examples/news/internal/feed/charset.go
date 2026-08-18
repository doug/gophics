package feed

import (
	"bytes"
	"io"

	"github.com/doug/gophics/examples/news/internal/textenc"
)

// charsetReader lets encoding/xml consume feeds served in legacy encodings.
// It never returns an error: refusing to parse a feed over a charset label
// would be a worse outcome than a few replacement characters.
func charsetReader(label string, input io.Reader) (io.Reader, error) {
	switch textenc.Normalize(label) {
	case "utf8", "ascii", "":
		return input, nil
	}
	b, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(textenc.ToUTF8(b, label)), nil
}
