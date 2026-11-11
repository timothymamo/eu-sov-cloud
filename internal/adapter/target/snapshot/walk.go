package snapshot

import (
	"github.com/timothymamo/eu-sov-cloud/internal/adapter/internal/jsondoc"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/taxonomy"
)

// walk reads the whole document with w, a walker at its start, and returns what
// it holds. The document is a JSON object whose exact schemaVersion key decode
// has found to be 1. jsondoc refuses an unknown, inexact or repeated key, a
// value of the wrong JSON type, a missing key and a string that escapes a lone
// surrogate; walk decides the keys, all of them required, and reads every value
// without jsondoc.Walker.Null, so null is refused like any other value of the
// wrong type: the format has no optional field.
func walk(w *jsondoc.Walker) (document, error) {
	var doc document

	err := w.Object(documentKeys, func(key string) error {
		var err error

		switch key {
		case keySchemaVersion:
			// decode has found the version to be 1, and jsondoc refuses a
			// second schemaVersion key as repeated before its value is read;
			// the walk checks the value again all the same, so it never relies
			// on that.
			err = w.Version(schemaVersion, ErrSchemaVersion)
		case keyTarget:
			doc.target, err = w.String()
		case keySourceURL:
			doc.sourceURL, err = w.String()
		case keyRetrievedOn:
			doc.retrievedOn, err = w.String()
		case keyRegions:
			doc.regions, err = list[string](w)
		case keyOfferings:
			doc.offerings, err = offerings(w)
		}

		return err
	})
	if err != nil {
		return document{}, err
	}

	return doc, nil
}

// list reads the next value: an array of strings, each escaping no lone
// surrogate, as values of type T. An empty array is an empty, non-nil list.
func list[T ~string](w *jsondoc.Walker) ([]T, error) {
	values := []T{}

	err := w.Array("an array of strings", func(int) error {
		s, err := w.String()
		values = append(values, T(s))

		return err
	})
	if err != nil {
		return nil, err
	}

	return values, nil
}

// offerings reads the offerings list: an array of offering objects, each with
// every key of offeringKeys.
func offerings(w *jsondoc.Walker) ([]offering, error) {
	values := []offering{}

	err := w.Array("an array of objects", func(int) error {
		o, err := readOffering(w)
		values = append(values, o)

		return err
	})
	if err != nil {
		return nil, err
	}

	return values, nil
}

// readOffering reads the next value: an offering object.
func readOffering(w *jsondoc.Walker) (offering, error) {
	var o offering

	err := w.Object(offeringKeys, func(key string) error {
		var err error

		switch key {
		case keyName:
			o.name, err = w.String()
		case keyCategories:
			o.categories, err = list[taxonomy.Category](w)
		case keyCapabilities:
			o.capabilities, err = list[taxonomy.Capability](w)
		case keyRegions:
			o.regions, err = list[string](w)
		}

		return err
	})
	if err != nil {
		return offering{}, err
	}

	return o, nil
}
