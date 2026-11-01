package apiapp

import (
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	passwordpolicy "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/password"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/apiconfig"
)

func loadPasswordAcceptance(c *apiconfig.Config) error {
	if c.Runtime.PasswordChange == nil {
		return nil
	}
	body, err := checkedfile.ReadPublic(c.PasswordBlocklistFile, 1<<20)
	if err != nil {
		return ErrConfiguration
	}
	defer clear(body)
	rules, err := passwordpolicy.ParseAcceptance(body, c.MinimumPasswordCharacters, c.Runtime.PasswordLimits.MaxPasswordBytes)
	if err != nil {
		return ErrConfiguration
	}
	c.Runtime.PasswordChange.Acceptance = rules
	return nil
}
