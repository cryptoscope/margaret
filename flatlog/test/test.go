// SPDX-License-Identifier: MIT

package test

import (
	"go.cryptoscope.co/margaret"

	"go.cryptoscope.co/margaret/flatlog"
	mtest "go.cryptoscope.co/margaret/test"
)

func init() {
	buildNewLogFunc := func() mtest.NewLogFunc {
		return func(name string, tipe interface{}) (margaret.Log, error) {
			return flatlog.Open(name)
		}
	}

	mtest.Register("flatlog", buildNewLogFunc())
}
