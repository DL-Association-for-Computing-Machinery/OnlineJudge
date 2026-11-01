//go:build !linux && !darwin

package redisstore

func readLoginSecrets(string) ([]byte, error) { return nil, ErrLoginSecrets }
