// SPDX-License-Identifier: MIT

package test

import (
	"io/ioutil"

	"github.com/dgraph-io/badger/v2"
	"github.com/pkg/errors"

	"go.cryptoscope.co/margaret/codec/json"
	"go.cryptoscope.co/margaret/multilog"
	mlbadger "go.cryptoscope.co/margaret/multilog/badger"
	mltest "go.cryptoscope.co/margaret/multilog/test"
)

func init() {
	newMultiLog := func(name string, tipe interface{}, testDir string) (multilog.MultiLog, string, error) {
		if testDir == "" {
			var err error
			testDir, err = ioutil.TempDir("", "badger")
			if err != nil {
				return nil, "", errors.Wrap(err, "error creating tempdir")
			}
		}

		opts := badger.DefaultOptions(testDir)
		db, err := badger.Open(opts)
		if err != nil {
			return nil, "", errors.Wrap(err, "error opening database")
		}

		return mlbadger.New(db, json.New(tipe)), testDir, nil
	}

	mltest.Register("badger", newMultiLog)
}
