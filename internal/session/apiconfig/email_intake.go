package apiconfig

import (
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/emailintake"
	sessionruntime "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/runtime"
)

func (w emailIntakeWire) apply(c *Config) error {
	intake := &sessionruntime.EmailIntakeConfig{
		Limits:         emailintake.Config{Capacity: w.Capacity, Workers: w.Workers},
		PayloadKeyFile: w.PayloadKeyFile,
	}
	if c.Runtime.EmailLimits == nil || c.EmailBlocklistFile == "" || w.BudgetDatabase == nil ||
		!validPath(w.PayloadKeyFile, false) || w.PayloadKeyFile == c.Runtime.SecretsFile ||
		!durations([]durationField{{w.MaxAge, &intake.Limits.MaxAge}}) || intake.Limits.Validate() != nil ||
		w.BudgetDatabase.apply(&intake.BudgetDatabase, &c.EmailBudgetAuth, &c.EmailBudgetTLS) != nil ||
		intake.BudgetDatabase.Database == c.Runtime.Database.Database {
		return ErrConfig
	}
	c.Runtime.EmailIntake = intake
	return nil
}
