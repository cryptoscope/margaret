// SPDX-License-Identifier: MIT

package test // import "go.cryptoscope.co/margaret/test"

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/margaret"
)

type namedLog interface {
	FileName() string
}

func LogTestGet(f NewLogFunc) func(*testing.T) {
	type testcase struct {
		tipe   interface{}
		values []interface{}
		result []interface{}
	}

	mkTest := func(tc testcase) func(*testing.T) {
		return func(t *testing.T) {
			a := assert.New(t)
			r := require.New(t)

			log, err := f(t.Name(), tc.tipe)
			r.NoError(err, "error creating log")
			r.NotNil(log, "returned log is nil")

			// TODO:
			// defer func() {
			// 	if namer, ok := log.(namedLog); ok {
			// 		r.NoError(os.RemoveAll(namer.FileName()), "error deleting log after test")
			// 	}
			// }()

			for i, v := range tc.values {
				seq, err := log.Append(v)
				r.NoError(err, "error appending to log")
				r.Equal(margaret.BaseSeq(i), seq, "sequence missmatch")
			}

			for i, wants := range tc.result {
				got, err := log.Get(margaret.BaseSeq(i))
				a.NoError(err, "error getting value at position", i)
				a.Equal(wants, got, "value mismatch at position", i)
			}
		}
	}

	tcs := []testcase{
		// {
		// 	tipe:   0,
		// 	values: []interface{}{1, 2, 3},
		// 	result: []interface{}{1, 2, 3},
		// },

		{
			tipe:   "strings",
			values: []interface{}{"a", "b", "c"},
			result: []interface{}{"a", "b", "c"},
		},

		{
			tipe:   "strings with newlines",
			values: []interface{}{"a", "hello\nworld", "n\nii\nic\nc\ne\neee"},
			result: []interface{}{"a", "hello\nworld", "n\nii\nic\nc\ne\neee"},
		},
	}

	return func(t *testing.T) {
		for i, tc := range tcs {
			t.Run(fmt.Sprintf("%d-type:%T", i, tc.tipe), mkTest(tc))
		}
	}
}
