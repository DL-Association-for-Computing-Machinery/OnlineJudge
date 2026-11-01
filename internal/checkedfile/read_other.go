//go:build !linux && !darwin

package checkedfile

func read(string, int64, bool) ([]byte, error) { return nil, ErrUnavailable }
