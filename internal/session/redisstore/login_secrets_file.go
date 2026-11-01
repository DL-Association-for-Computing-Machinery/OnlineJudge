package redisstore

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"

func readLoginSecrets(path string) ([]byte, error) {
	body, err := checkedfile.ReadSecret(path, maxLoginSecretsBytes)
	if err != nil {
		return nil, ErrLoginSecrets
	}
	return body, nil
}
