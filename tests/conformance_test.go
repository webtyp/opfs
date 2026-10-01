//go:build wasm

package tests

import (
	"fmt"
	"testing"
	"time"

	"webtyp.com/files"
	"webtyp.com/files/conformance"
	"webtyp.com/opfs"
)

// opfs passes the files contract's suite: any code written against files works on it.
func TestConformance(t *testing.T) {
	n := 0
	conformance.Run(t, conformance.Factory{
		Name: "opfs",
		New: func(t *testing.T) files.ReadWriter {
			n++
			fs, err := opfs.Open(fmt.Sprintf("conformance/%d-%d", time.Now().UnixNano(), n))
			if err != nil {
				t.Fatal(err)
			}
			return fs
		},
	})
}
