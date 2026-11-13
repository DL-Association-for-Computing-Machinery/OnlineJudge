package emailintake_test

import (
	"context"
	"errors"
	"net/netip"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/emailtoken"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/emailintake"
)

const (
	oldEpoch = "0123456789abcdef0123456789abcdef"
	newEpoch = "fedcba9876543210fedcba9876543210"
)

func TestHandoffIsBoundedAndDrains(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		address := mailbox(t, "Fixture@example.invalid")
		source := netip.MustParseAddr("::ffff:192.0.2.1")
		allowSnapshot := make(chan struct{})
		admissionEntered, allowWork := make(chan struct{}), make(chan struct{})
		epoch := oldEpoch
		var snapshots, processed int
		i, err := emailintake.New(emailintake.Config{Capacity: 1, Workers: 1, MaxAge: time.Second},
			func(context.Context) (identity.RecoveryState, error) {
				snapshots++
				<-allowSnapshot
				return identity.RecoveryState{Enabled: true, Generation: epoch}, nil
			}, func(_ context.Context, ip netip.Addr, email identity.Email) error {
				if ip.String() != "192.0.2.1" || email != address {
					t.Error("admission did not receive the canonical source and original mailbox")
				}
				close(admissionEntered)
				<-allowWork
				return nil
			}, func(ctx context.Context, email identity.Email, purpose emailtoken.Purpose, generation string) {
				processed++
				if ctx.Err() != nil || email != address || purpose != emailtoken.Recovery || generation != oldEpoch {
					t.Error("processing lost the original job or inherited HTTP cancellation")
				}
			})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		accepted := make(chan error, 1)
		go func() { accepted <- i.Submit(ctx, source, emailtoken.Recovery, address) }()
		synctest.Wait()
		if err := i.Submit(t.Context(), source, emailtoken.Activation, address); !errors.Is(err, emailintake.ErrUnavailable) || snapshots != 1 {
			t.Fatal("full intake allowed another recovery lookup")
		}
		close(allowSnapshot)
		<-admissionEntered
		if err := <-accepted; err != nil {
			t.Fatal(err)
		}
		cancel()
		epoch = newEpoch
		drained := make(chan struct{})
		go func() { i.Drain(); close(drained) }()
		synctest.Wait()
		select {
		case <-drained:
			t.Fatal("drain returned while work was still active")
		default:
		}
		if err := i.Submit(t.Context(), source, emailtoken.Activation, address); !errors.Is(err, emailintake.ErrUnavailable) {
			t.Fatal("closing intake accepted another submission")
		}
		close(allowWork)
		<-drained
		i.Drain()
		if processed != 1 {
			t.Fatalf("processed %d requests, want one", processed)
		}
	})
}

func TestBackgroundDenialFailureAndQueueExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := netip.MustParseAddr("192.0.2.1")
		denied := mailbox(t, "denied@example.invalid")
		failed := mailbox(t, "failed@example.invalid")
		busy := mailbox(t, "busy@example.invalid")
		expired := mailbox(t, "expired@example.invalid")
		var admissions, processed []identity.Email
		i, err := emailintake.New(emailintake.Config{Capacity: 2, Workers: 1, MaxAge: time.Second}, openEpoch,
			func(_ context.Context, _ netip.Addr, email identity.Email) error {
				admissions = append(admissions, email)
				if email == denied || email == failed {
					return errors.New("admission refused or outcome unknown")
				}
				return nil
			}, func(ctx context.Context, email identity.Email, _ emailtoken.Purpose, _ string) {
				processed = append(processed, email)
				if email == busy {
					<-ctx.Done()
				}
			})
		if err != nil {
			t.Fatal(err)
		}
		for _, email := range []identity.Email{denied, failed, busy, expired} {
			if err := i.Submit(t.Context(), source, emailtoken.Activation, email); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
		}
		i.Drain()
		if len(admissions) != 3 || admissions[0] != denied || admissions[1] != failed || admissions[2] != busy ||
			len(processed) != 1 || processed[0] != busy {
			t.Fatal("intake retried, processed denied work, or attempted admission after the queue deadline")
		}
	})
}

func TestDrainRacesWithSnapshotAndCanceledIntake(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		address := mailbox(t, "fixture@example.invalid")
		source := netip.MustParseAddr("192.0.2.1")
		var calls atomic.Int32
		mayFinish := make(chan struct{})
		i, err := emailintake.New(emailintake.Config{Capacity: 1, Workers: 1, MaxAge: time.Second},
			func(ctx context.Context) (identity.RecoveryState, error) {
				if calls.Add(1) == 1 {
					<-ctx.Done()
				} else {
					<-mayFinish
				}
				return openEpoch(ctx)
			}, func(context.Context, netip.Addr, identity.Email) error { return nil },
			func(context.Context, identity.Email, emailtoken.Purpose, string) { calls.Add(10) })
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		result := make(chan error, 1)
		go func() { result <- i.Submit(ctx, source, emailtoken.Activation, address) }()
		synctest.Wait()
		cancel()
		if err := <-result; !errors.Is(err, emailintake.ErrUnavailable) || calls.Load() != 1 {
			t.Fatal("canceled intake retained capacity or handed work off")
		}
		go func() { result <- i.Submit(t.Context(), source, emailtoken.Activation, address) }()
		synctest.Wait()
		drained := make(chan struct{})
		go func() { i.Drain(); close(drained) }()
		synctest.Wait()
		close(mayFinish)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		<-drained
		if calls.Load() != 12 {
			t.Fatal("drain did not complete the already reserved handoff exactly once")
		}
	})
}

func TestParallelSubmissionsRespectCapacityAndWorkerCount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		address := mailbox(t, "fixture@example.invalid")
		source := netip.MustParseAddr("192.0.2.1")
		var snapshots atomic.Int32
		started := make(chan struct{}, 4)
		release := make(chan struct{})
		i, err := emailintake.New(emailintake.Config{Capacity: 4, Workers: 2, MaxAge: time.Second},
			func(ctx context.Context) (identity.RecoveryState, error) {
				snapshots.Add(1)
				return openEpoch(ctx)
			}, func(context.Context, netip.Addr, identity.Email) error { return nil },
			func(context.Context, identity.Email, emailtoken.Purpose, string) {
				started <- struct{}{}
				<-release
			})
		if err != nil {
			t.Fatal(err)
		}
		results := make(chan error, 12)
		for range cap(results) {
			go func() { results <- i.Submit(t.Context(), source, emailtoken.Activation, address) }()
		}
		synctest.Wait()
		accepted := 0
		for range cap(results) {
			if err := <-results; err == nil {
				accepted++
			} else if !errors.Is(err, emailintake.ErrUnavailable) {
				t.Errorf("unexpected result: %v", err)
			}
		}
		if accepted != 4 || snapshots.Load() != 4 || len(started) != 2 {
			t.Errorf("accepted %d, read %d snapshots, and started %d workers; want 4, 4, and 2", accepted, snapshots.Load(), len(started))
		}
		close(release)
		i.Drain()
		if len(started) != 4 {
			t.Fatalf("drained %d accepted requests, want 4", len(started))
		}
	})
}

func TestUnavailableRecoveryReleasesCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		address := mailbox(t, "fixture@example.invalid")
		source := netip.MustParseAddr("192.0.2.1")
		snapshots, admissions, processed := 0, 0, 0
		i, err := emailintake.New(emailintake.Config{Capacity: 1, Workers: 1, MaxAge: time.Second},
			func(ctx context.Context) (identity.RecoveryState, error) {
				snapshots++
				switch snapshots {
				case 1:
					return identity.RecoveryState{}, errors.New("recovery lookup failed")
				case 2:
					return identity.RecoveryState{Generation: oldEpoch}, nil
				case 3:
					return identity.RecoveryState{Enabled: true, Generation: "invalid"}, nil
				case 4:
					<-ctx.Done()
				}
				return openEpoch(ctx)
			}, func(ctx context.Context, _ netip.Addr, _ identity.Email) error {
				admissions++
				if admissions == 1 {
					<-ctx.Done()
				}
				return nil
			}, func(context.Context, identity.Email, emailtoken.Purpose, string) { processed++ })
		if err != nil {
			t.Fatal(err)
		}
		for n := 1; n <= 4; n++ {
			if err := i.Submit(t.Context(), source, emailtoken.Recovery, address); !errors.Is(err, emailintake.ErrUnavailable) || snapshots != n {
				t.Fatalf("failed snapshot %d accepted work or did not free capacity: %v", n, err)
			}
		}
		if err := i.Submit(t.Context(), source, emailtoken.Recovery, address); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second + time.Nanosecond)
		synctest.Wait()
		if err := i.Submit(t.Context(), source, emailtoken.Recovery, address); err != nil {
			t.Fatal(err)
		}
		i.Drain()
		if admissions != 2 || processed != 1 {
			t.Fatalf("admitted %d and processed %d, want 2 and 1", admissions, processed)
		}
	})
}

func openEpoch(context.Context) (identity.RecoveryState, error) {
	return identity.RecoveryState{Enabled: true, Generation: oldEpoch}, nil
}

func mailbox(t *testing.T, s string) identity.Email {
	t.Helper()
	email, err := identity.ParseEmail(s)
	if err != nil {
		t.Fatal(err)
	}
	return email
}
