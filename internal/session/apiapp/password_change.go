package apiapp

import (
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	passwordpolicy "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/apiconfig"
)

func loadPasswordAcceptance(c *apiconfig.Config) error {
	if c.Runtime.PasswordChange != nil {
		rules, err := readPasswordAcceptance(c.PasswordBlocklistFile, c.MinimumPasswordCharacters, c.Runtime.PasswordLimits.MaxPasswordBytes)
		if err != nil {
			return err
		}
		c.Runtime.PasswordChange.Acceptance = rules
	}
	if c.EmailBlocklistFile != "" {
		rules, err := readPasswordAcceptance(c.EmailBlocklistFile, c.EmailMinimumCharacters, c.Runtime.PasswordLimits.MaxPasswordBytes)
		if err != nil {
			return err
		}
		c.Runtime.EmailConfirmation = rules
	}
	return nil
}

func readPasswordAcceptance(path string, minimum int, maxBytes uint32) (*passwordpolicy.Acceptance, error) {
	body, err := checkedfile.ReadPublic(path, 1<<20)
	if err != nil {
		return nil, ErrConfiguration
	}
	defer clear(body)
	rules, err := passwordpolicy.ParseAcceptance(body, minimum, maxBytes)
	if err != nil {
		return nil, ErrConfiguration
	}
	return rules, nil
}
