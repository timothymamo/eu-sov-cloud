package inventoryfile

import (
	"fmt"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/internal/jsondoc"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/inventory"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// walk reads the whole document with w, a walker at its start, and returns what
// it holds. The document is a JSON object whose exact schemaVersion key decode
// has found to be 1. jsondoc refuses an unknown, inexact or repeated key, a
// value of the wrong JSON type, a missing resources list and a string that
// escapes a lone surrogate. walk decides the null policy of ADR 0006: null is
// an absent value for every field but resources, and never an element of
// resources or of capabilities. It counts resources and capabilities as it
// reads them, and refuses a list at its first element past the cap, before
// that element is read.
func walk(w *jsondoc.Walker) (document, error) {
	var doc document

	err := w.Object(documentKeys, func(key string) error {
		var err error

		switch key {
		case keySchemaVersion:
			err = version(w)
		case keySource:
			doc.source, err = optionalText(w)
		case keyResources:
			doc.resources, err = resources(w)
		}

		return err
	})
	if err != nil {
		return document{}, err
	}

	return doc, nil
}

// version reads the value of the schemaVersion key. decode has found it to be
// 1, and jsondoc refuses a second schemaVersion key as repeated before its value
// is read; it is checked again all the same, so the walk never relies on it.
func version(w *jsondoc.Walker) error {
	raw, err := w.Raw()
	if err != nil {
		return err
	}

	return checkVersion(raw, "")
}

// optionalText reads the next value: a string, or null for an absent value,
// which is "".
func optionalText(w *jsondoc.Walker) (string, error) {
	if null, err := w.Null(); null || err != nil {
		return "", err
	}

	return w.String()
}

// resources reads the resources list, counting its elements as it goes: the
// list is refused at its first element past MaxResources. null is refused, as
// a value that is not an array.
func resources(w *jsondoc.Walker) ([]inventory.Resource, error) {
	list := []inventory.Resource{}

	err := w.Array("an array", func(i int) error {
		if i == MaxResources {
			return fmt.Errorf("%w: more than %d", ErrTooManyResources, MaxResources)
		}

		r, err := resource(w)
		list = append(list, r)

		return err
	})
	if err != nil {
		return nil, err
	}

	return list, nil
}

// resource reads the next value: a resource object. Its values are checked by
// inventory.New, not here.
func resource(w *jsondoc.Walker) (inventory.Resource, error) {
	var r inventory.Resource

	err := w.Object(resourceKeys, func(key string) error {
		var (
			s   string
			err error
		)

		switch key {
		case keyID:
			r.ID, err = optionalText(w)
		case keyRawType:
			r.RawType, err = optionalText(w)
		case keyCategory:
			s, err = optionalText(w)
			r.Category = taxonomy.Category(s)
		case keyRegion:
			r.Region, err = optionalText(w)
		case keyCapabilities:
			r.Capabilities, err = capabilities(w)
		}

		return err
	})
	if err != nil {
		return inventory.Resource{}, err
	}

	return r, nil
}

// capabilities reads the next value: an array of at most MaxCapabilities
// strings, or null for an absent list. The list is refused at its first element
// past the cap, before that element is read.
func capabilities(w *jsondoc.Walker) ([]taxonomy.Capability, error) {
	if null, err := w.Null(); null || err != nil {
		return nil, err
	}

	path := w.Path()
	list := []taxonomy.Capability{}

	err := w.Array("an array", func(j int) error {
		if j == MaxCapabilities {
			return fmt.Errorf("%w: %s lists more than %d capabilities, at %s",
				ErrDecode, text.Quote(path), MaxCapabilities, w.Position())
		}

		s, err := w.String()
		list = append(list, taxonomy.Capability(s))

		return err
	})
	if err != nil {
		return nil, err
	}

	return list, nil
}
