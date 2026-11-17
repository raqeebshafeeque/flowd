package domain

import (
	"strings"

	"github.com/oklog/ulid/v2"
)

// ID identifies a day or an item. It never changes and is never reused.
type ID string

// NewID returns a fresh ID such as "it_01JD...". IDs made later sort after
// earlier ones, which keeps ties in ordering stable.
func NewID(prefix string) ID {
	return ID(strings.Join([]string{prefix, ulid.Make().String()}, "_"))
}
