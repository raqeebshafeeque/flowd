package utils

import (
	"strings"

	"github.com/oklog/ulid/v2"
)

type ID string

func NewID() ID {
	id := ulid.Make()
	return ID(id.String())
}

func NewIDWithPrefix(prefix string) ID {
	id := ulid.Make()
	return ID(strings.Join([]string{prefix, id.String()}, "_"))
}
