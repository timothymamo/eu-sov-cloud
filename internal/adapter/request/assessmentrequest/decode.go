package assessmentrequest

import (
	"errors"
	"fmt"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/internal/jsondoc"
)

// MaxBodySize is the largest request body Decode reads, in bytes: the inventory
// reader's cap (inventoryfile.MaxFileSize, 10 MiB) plus 64 KiB for the envelope
// and the target list. It is written out because an adapter imports no other
// adapter; a test pins it to that sum.
const MaxBodySize = 10<<20 + 64<<10

// ErrDecode is wrapped by every error Decode returns: a body that is not one
// well-formed request envelope.
var ErrDecode = errors.New("not a valid assessment request")

// The keys of the request envelope.
const (
	keyInventory = "inventory"
	keyTargets   = "targets"
)

// envelopeKeys lists the keys of the request object, both required. It is never
// modified.
var envelopeKeys = []jsondoc.Key{
	{Name: keyInventory, Required: true},
	{Name: keyTargets, Required: true},
}

// Decode strictly decodes body, the body of POST /v1/assessments, and returns
// the inventory object as the raw bytes the body holds and the target keys in
// the order they were sent. It refuses, with an error that wraps ErrDecode and
// returns neither: a body over MaxBodySize, anything jsondoc.Open refuses, an
// unknown, inexact or repeated key, a missing key, an inventory that is not an
// object, and targets that are not a non-empty array of strings. No target key
// is validated here.
func Decode(body []byte) (inventory []byte, targets []string, err error) {
	doc, err := jsondoc.Open(body, MaxBodySize, ErrDecode)
	if err != nil {
		return nil, nil, err
	}

	w := doc.Walker()

	err = w.Object(envelopeKeys, func(key string) error {
		var err error

		switch key {
		case keyInventory:
			inventory, err = object(w)
		case keyTargets:
			targets, err = targetKeys(w)
		}

		return err
	})
	if err != nil {
		return nil, nil, err
	}

	return inventory, targets, nil
}

// object reads the next value, which must be a JSON object, and returns its
// JSON text unchanged. Only its type is checked: its contents are the inventory
// reader's to decode.
func object(w *jsondoc.Walker) ([]byte, error) {
	at := w.Position()

	raw, err := w.Raw()
	if err != nil {
		return nil, err
	}

	if raw[0] != '{' {
		return nil, fmt.Errorf("%w: %s at %s is %s, want an object", ErrDecode, keyInventory, at, kindOf(raw))
	}

	return raw, nil
}

// kindOf names the JSON type of raw, with its article, never its value.
func kindOf(raw []byte) string {
	switch raw[0] {
	case '[':
		return "an array"
	case '"':
		return "a string"
	case 't', 'f':
		return "a boolean"
	case 'n':
		return "null"
	default:
		return "a number"
	}
}

// targetKeys reads the next value, which must be a non-empty array of strings,
// and returns its strings in order.
func targetKeys(w *jsondoc.Walker) ([]string, error) {
	at := w.Position()

	var keys []string

	err := w.Array("an array of strings", func(int) error {
		key, err := w.String()
		if err != nil {
			return err
		}

		keys = append(keys, key)

		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: %s at %s is empty, want at least one target", ErrDecode, keyTargets, at)
	}

	return keys, nil
}
