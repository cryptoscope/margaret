// SPDX-License-Identifier: MIT

package badger

import (
	"github.com/dgraph-io/badger"
	"github.com/pkg/errors"
	"go.cryptoscope.co/margaret/internal/persist"
)

type ModernSaver struct {
	db *badger.DB
}

var _ persist.Saver = (*ModernSaver)(nil)

func (sl *ModernSaver) Close() error {
	return sl.db.Close()
}

func New(path string) (*ModernSaver, error) {
	var ms ModernSaver

	var err error

	o := badger.DefaultOptions(path)
	ms.db, err = badger.Open(o)
	if err != nil {

		return nil, errors.Wrapf(err, "failed to create KV %s", path)
	}

	return &ms, nil
}
