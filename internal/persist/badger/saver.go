// SPDX-License-Identifier: MIT

package badger

import (
	"github.com/dgraph-io/badger"
	"go.cryptoscope.co/margaret/internal/persist"
)

func (s ModernSaver) Put(key persist.Key, data []byte) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
}

func (s ModernSaver) Get(key persist.Key) ([]byte, error) {
	var data []byte
	err := s.db.View(func(txn *badger.Txn) error {
		it, err := txn.Get(key)
		if err != nil {
			return err
		}
		data, err = it.ValueCopy(nil)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, persist.ErrNotFound
	}

	if len(data) == 0 {
		return nil, persist.ErrNotFound
	}

	return data, nil
}

func (s ModernSaver) List() ([]persist.Key, error) {

	var keys []persist.Key

	err := s.db.Update(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iter.Close()

		for iter.Rewind(); iter.Valid(); iter.Next() {
			it := iter.Item()

			k := it.Key()
			keys = append(keys, persist.Key(k))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

func (s ModernSaver) Delete(rm persist.Key) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(rm)
	})
}
