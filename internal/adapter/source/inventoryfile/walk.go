package inventoryfile

import (
	"fmt"
	"strconv"

	"github.com/timothymamo/shift2eu/internal/adapter/internal/jsondoc"
	"github.com/timothymamo/shift2eu/internal/domain/inventory"
	"github.com/timothymamo/shift2eu/internal/domain/taxonomy"
	"github.com/timothymamo/shift2eu/internal/domain/text"
)

// walk reads the whole document with w, a walker at its start, and returns what
// it holds. The document is a JSON object whose exact schemaVersion key decode
// has found to be version, 1 or 2; a resource of version 1 has none of the keys
// version 2 adds. jsondoc refuses an unknown, inexact or repeated key, a
// value of the wrong JSON type, a missing resources list and a string that
// escapes a lone surrogate. walk decides the null policy of ADR 0006: null is
// an absent value for every field but resources, at every level of a version 2
// object too, and never an element of resources, capabilities or relations. It
// counts resources, capabilities and relations as it reads them, and refuses a
// list at its first element past the cap, before that element is read.
func walk(w *jsondoc.Walker, version int) (document, error) {
	var doc document

	err := w.Object(documentKeys, func(key string) error {
		var err error

		switch key {
		case keySchemaVersion:
			// decode has found the version, and jsondoc refuses a second
			// schemaVersion key as repeated before its value is read; the
			// walk checks the value again all the same, so it never relies on
			// that.
			err = sameVersion(w, version)
		case keySource:
			doc.source, err = optionalText(w)
		case keyResources:
			doc.resources, err = resources(w, version)
		}

		return err
	})
	if err != nil {
		return document{}, err
	}

	return doc, nil
}

// sameVersion reads the next value, the schemaVersion, and checks that it is
// version, the version decode found.
func sameVersion(w *jsondoc.Walker, version int) error {
	raw, err := w.Raw()
	if err != nil {
		return err
	}

	got, err := checkVersion(raw, "")
	if err != nil {
		return err
	}

	if got != version {
		return fmt.Errorf("%w: %s is %d, not the version %d read first", ErrSchemaVersion, keySchemaVersion, got, version)
	}

	return nil
}

// optionalText reads the next value: a string, or null for an absent value,
// which is "".
func optionalText(w *jsondoc.Walker) (string, error) {
	if null, err := w.Null(); null || err != nil {
		return "", err
	}

	return w.String()
}

// resourceChunk is how many resources resources gathers in one slice before it
// starts the next.
const resourceChunk = 1024

// resources reads the resources list, counting its elements as it goes: the
// list is refused at its first element past MaxResources. null is refused, as
// a value that is not an array.
//
// The resources are gathered in slices of resourceChunk and joined once at the
// end, so a long list is copied once, not at every growth of one slice, which
// keeps the memory a document of many resources costs in proportion to the
// file. Each is decoded straight into its place.
func resources(w *jsondoc.Walker, version int) ([]inventory.Resource, error) {
	var (
		full  [][]inventory.Resource
		chunk []inventory.Resource
		total int
	)

	err := w.Array("an array", func(i int) error {
		if i == MaxResources {
			return fmt.Errorf("%w: more than %d", ErrTooManyResources, MaxResources)
		}

		if len(chunk) == cap(chunk) {
			if chunk != nil {
				full = append(full, chunk)
			}

			chunk = make([]inventory.Resource, 0, resourceChunk)
		}

		chunk = append(chunk, inventory.Resource{})
		total++

		return resource(w, version, &chunk[len(chunk)-1])
	})
	if err != nil {
		return nil, err
	}

	list := make([]inventory.Resource, 0, total)
	for _, c := range full {
		list = append(list, c...)
	}

	return append(list, chunk...), nil
}

// resource reads the next value into r: a resource object of version, with the
// keys that version defines. Its values are checked by inventory.New, not here.
func resource(w *jsondoc.Walker, version int, r *inventory.Resource) error {
	keys := resourceKeys
	if version == schemaVersion2 {
		keys = resourceKeysV2
	}

	return w.Object(keys, func(key string) error {
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
		case keySize:
			r.Size, err = size(w)
		case keyUsage:
			r.Usage, err = usage(w)
		case keyWorkload:
			r.Workload, err = optionalText(w)
		case keyRelations:
			r.Relations, err = relations(w)
		case keyMonthlyCost:
			r.MonthlyCost, err = cost(w)
		}

		return err
	})
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

// size reads the next value: a size object, or null for a size not given. A
// member that is null is not given either. The values are checked by
// inventory.New, which reads a size that gives nothing as no size.
func size(w *jsondoc.Walker) (*inventory.Size, error) {
	if null, err := w.Null(); null || err != nil {
		return nil, err
	}

	var s inventory.Size

	err := w.Object(sizeKeys, func(key string) error {
		var err error

		switch key {
		case keyVCPUs:
			s.VCPUs, err = number(w)
		case keyMemoryGiB:
			s.MemoryGiB, err = number(w)
		case keyStorageGiB:
			s.StorageGiB, err = number(w)
		case keyCount:
			s.Count, err = integer(w)
		}

		return err
	})
	if err != nil {
		return nil, err
	}

	return &s, nil
}

// usage reads the next value: a usage object, or null for a usage not given.
func usage(w *jsondoc.Walker) (*inventory.Usage, error) {
	if null, err := w.Null(); null || err != nil {
		return nil, err
	}

	var u inventory.Usage

	err := w.Object(usageKeys, func(string) error {
		var err error

		u.HoursPerMonth, err = number(w)

		return err
	})
	if err != nil {
		return nil, err
	}

	return &u, nil
}

// cost reads the next value: a monthly cost object with both its keys, or null
// for a cost not given. A null amount or currency reaches inventory.New as not
// given, which refuses it.
func cost(w *jsondoc.Walker) (*inventory.Cost, error) {
	if null, err := w.Null(); null || err != nil {
		return nil, err
	}

	var c inventory.Cost

	err := w.Object(costKeys, func(key string) error {
		var err error

		switch key {
		case keyAmount:
			c.Amount, err = number(w)
		case keyCurrency:
			c.Currency, err = optionalText(w)
		}

		return err
	})
	if err != nil {
		return nil, err
	}

	return &c, nil
}

// relations reads the next value: an array of at most MaxRelations relation
// objects, duplicates included, or null for an absent list. The list is refused at its first element
// past the cap, before that element is read; a null element is refused, as a
// value that is not an object.
func relations(w *jsondoc.Walker) ([]inventory.Relation, error) {
	if null, err := w.Null(); null || err != nil {
		return nil, err
	}

	// The cap names the resource the list belongs to, resources[1]: the list's
	// path without its key. It is built here, never for a resource without
	// relations, so a document of many resources pays nothing for it.
	path := w.Path()
	at := path[:len(path)-len("."+keyRelations)]

	list := []inventory.Relation{}

	err := w.Array("an array", func(j int) error {
		if j == MaxRelations {
			return fmt.Errorf("%w: %s lists more than %d %s, at %s",
				ErrDecode, text.Quote(at), MaxRelations, keyRelations, w.Position())
		}

		var r inventory.Relation

		err := w.Object(relationKeys, func(key string) error {
			var (
				s   string
				err error
			)

			switch key {
			case keyType:
				s, err = optionalText(w)
				r.Type = inventory.RelationType(s)
			case keyTarget:
				r.Target, err = optionalText(w)
			}

			return err
		})
		list = append(list, r)

		return err
	})
	if err != nil {
		return nil, err
	}

	return list, nil
}

// number reads the next value: a JSON number that a float64 holds, or null for
// a number not given. A value of another JSON type, and a number too large for
// a float64 (1e999), are refused at their place, never by their value.
func number(w *jsondoc.Walker) (*float64, error) {
	raw, at, err := numberText(w)
	if raw == nil || err != nil {
		return nil, err
	}

	n, err := strconv.ParseFloat(string(raw), 64)
	if err != nil {
		return nil, fmt.Errorf("%w: %s is a number too large to read, at %s", ErrDecode, text.Quote(w.Path()), at)
	}

	return &n, nil
}

// integer reads the next value: a JSON number written as an integer, with no
// fraction or exponent, as schemaVersion is, that an int holds; or null for a
// number not given. Anything else is refused at its place, never by its value.
func integer(w *jsondoc.Walker) (*int, error) {
	raw, at, err := numberText(w)
	if raw == nil || err != nil {
		return nil, err
	}

	digits := raw
	if digits[0] == '-' {
		digits = digits[1:]
	}

	for _, b := range digits {
		if b < '0' || b > '9' {
			return nil, fmt.Errorf("%w: %s is a number that is not an integer, want an integer, at %s",
				ErrDecode, text.Quote(w.Path()), at)
		}
	}

	n, err := strconv.Atoi(string(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %s is an integer too large to read, at %s", ErrDecode, text.Quote(w.Path()), at)
	}

	return &n, nil
}

// numberText reads the next value and returns its JSON text if it is a number,
// with where it starts; nil when it is null. A value of another JSON type is
// refused, named by its type as jsondoc names one.
func numberText(w *jsondoc.Walker) ([]byte, string, error) {
	if null, err := w.Null(); null || err != nil {
		return nil, "", err
	}

	at := w.Position()

	raw, err := w.Raw()
	if err != nil {
		return nil, "", err
	}

	if kind, isNumber := jsonKind(raw[0]); !isNumber {
		return nil, "", fmt.Errorf("%w: %s is %s, want a number, at %s", ErrDecode, text.Quote(w.Path()), kind, at)
	}

	return raw, at, nil
}
