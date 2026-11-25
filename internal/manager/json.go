package manager

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
)

// DecodeUniqueJSON rejects duplicate object keys, including case-insensitive
// aliases that encoding/json would otherwise match to the same struct field.
func DecodeUniqueJSON(data []byte, out any) error {
	check := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueValue(check, 0); err != nil {
		return err
	}
	if _, err := check.Token(); err != io.EOF {
		return errors.New("trailing JSON value")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}

func uniqueValue(dec *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("JSON nesting limit exceeded")
	}
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	switch tok {
	case json.Delim('{'):
		keys := make(map[string]struct{})
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid JSON object key")
			}
			folded := foldJSONKey(key)
			if _, exists := keys[folded]; exists {
				return errors.New("duplicate JSON field")
			}
			keys[folded] = struct{}{}
			if err := uniqueValue(dec, depth+1); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case json.Delim('['):
		for dec.More() {
			if err := uniqueValue(dec, depth+1); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	default:
		return nil
	}
}

func foldJSONKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		lowest := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < lowest {
				lowest = next
			}
		}
		b.WriteRune(lowest)
	}
	return b.String()
}
