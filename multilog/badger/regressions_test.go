// SPDX-License-Identifier: MIT

package badger

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/margaret/codec/msgpack"
)

func logPath(t *testing.T, path string) {
	t.Log("not deleting test data because test failed.")
	t.Log("test data path:", path)
}

func logOrRemoveTestData(t *testing.T, path string) {
	if t.Failed() {
		logPath(t, path)
	} else {
		if err := os.RemoveAll(path); err != nil {
			t.Errorf("error removing test data at %q: %s", path, err)
		}
	}
}

func TestIncrementRegression(t *testing.T) {
	r := require.New(t)

	name, err := ioutil.TempDir("/tmp", "TestMultiLogBadgerIncrementRegression")
	r.NoError(err, "error creating temp folder for database")
	defer logOrRemoveTestData(t, name)

	dbOpts := badger.DefaultOptions(name)

	db, err := badger.Open(dbOpts)
	r.NoError(err, "error opening database")

	mlog := New(db, msgpack.New(uint64(0)))

	slog, err := mlog.Get(librarian.Addr([]byte{2, 23, 255}))
	r.NoError(err, "error getting sublog")

	for ui := uint64(1); ui < 10; ui++ {
		_, err := slog.Append(ui)
		r.NoError(err, "error appending value", ui)
	}

	addrs, err := mlog.List()
	r.NoError(err, "error listing multilog")

	t.Log(addrs)
}
