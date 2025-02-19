package test

import (
	"context"

	"github.com/OpenSlides/openslides-autoupdate-service/pkg/datastore"
)

// KeysBuilder is a mock that impelements the autoupdate.KeysBuilder interface.
type KeysBuilder struct {
	K []string
}

// Update does nothing.
func (m KeysBuilder) Update(context.Context, datastore.Getter) error {
	return nil
}

// Keys returns the keys from the mock.
func (m KeysBuilder) Keys() []string {
	return m.K
}
