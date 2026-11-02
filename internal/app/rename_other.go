//go:build !linux && !darwin

package app

import "errors"

func renameNoReplace(from, to string) error {
	return errors.New("Safe relocation is unsupported on this platform")
}
