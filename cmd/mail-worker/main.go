// Command mail-worker delivers account email independently of the judge.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/db"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailmessage"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/maildelivery"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/mailtransport"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, get func(string) string) error {
	dsn := get("OJ_DATABASE_URL")
	if dsn == "" {
		return errors.New("mail worker database is not configured")
	}
	protector, err := emailtoken.LoadProtector(get("OJ_EMAIL_PAYLOAD_KEYS_FILE"))
	if err != nil {
		return err
	}
	renderer, err := emailmessage.New(get("OJ_ACCOUNT_PUBLIC_ORIGIN"))
	if err != nil {
		return err
	}
	smtp, err := sender(get)
	if err != nil {
		return err
	}
	start, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	pool, err := postgres.Open(start, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.CheckSchema(start, pool); err != nil {
		return err
	}
	worker, err := maildelivery.NewWorker(start, pool, protector, renderer, smtp)
	if err != nil {
		return err
	}
	cancel()
	return worker.Run(ctx)
}

func sender(get func(string) string) (*mailtransport.Sender, error) {
	bad := errors.New("invalid mail worker SMTP configuration")
	var mode mailtransport.TLSMode
	switch get("OJ_SMTP_TLS") {
	case "implicit":
		mode = mailtransport.ImplicitTLS
	case "starttls":
		mode = mailtransport.RequireSTARTTLS
	default:
		return nil, bad
	}
	port, err := strconv.Atoi(get("OJ_SMTP_PORT"))
	if err != nil {
		return nil, bad
	}
	from, err := identity.ParseEmail(get("OJ_SMTP_FROM"))
	if err != nil {
		return nil, bad
	}
	var password []byte
	if path := get("OJ_SMTP_PASSWORD_FILE"); path != "" {
		password, err = checkedfile.ReadSecret(path, 4096)
		if err != nil || len(password) == 0 {
			return nil, bad
		}
		defer clear(password)
	}
	return mailtransport.New(mailtransport.Config{
		Host: get("OJ_SMTP_HOST"), Port: port, TLS: mode, From: from, Timeout: 20 * time.Second,
	}, get("OJ_SMTP_USER"), string(password))
}
