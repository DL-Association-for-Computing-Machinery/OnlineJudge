package sandbox_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func relayPair(t *testing.T) (*faultRelay, net.Conn, net.Conn, net.Listener) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() }) // Private fixture listener only.
	r, err := newFaultRelay(context.Background(), l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !r.Close() {
			t.Error("relay did not join its workers")
		}
	})
	client, err := net.DialTimeout("tcp", strings.TrimPrefix(r.endpoint(), "https://"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() }) // Operation assertions precede cleanup.
	if tcp, ok := l.(*net.TCPListener); ok {
		if err := tcp.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	server, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() }) // Operation assertions precede cleanup.
	return r, client, server, l
}

func relayRead(t *testing.T, c net.Conn, want []byte) {
	t.Helper()
	if err := c.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(want))
	if _, err := io.ReadFull(c, got); err != nil || !bytes.Equal(got, want) {
		t.Fatal("relay changed or lost stream bytes")
	}
}

func relayWrite(t *testing.T, c net.Conn, data []byte) {
	t.Helper()
	if err := c.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, err := c.Write(data); err != nil || n != len(data) {
		t.Fatal("fixture stream write failed")
	}
}

func waitRelay(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("relay did not observe fault traffic")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFaultRelayPreservesBytes(t *testing.T) {
	r, client, server, _ := relayPair(t)
	data := []byte(strings.Repeat("\x00\xff\r\nopaque-wire", 5000))
	sent := make(chan error, 1)
	go func() { _, err := client.Write(data); sent <- err }()
	relayRead(t, server, data)
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	go func() { _, err := server.Write(data); sent <- err }()
	relayRead(t, client, data)
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	if r.upHeld.Load() != 0 || r.downHeld.Load() != 0 {
		t.Fatal("healthy streams were marked blocked")
	}
}

func TestFaultRelayDirectionsAndResume(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode relayDirection
	}{{"downstream", relayDown}, {"both", relayUp | relayDown}} {
		t.Run(tc.name, func(t *testing.T) {
			r, client, server, upstream := relayPair(t)
			r.Block(tc.mode)
			relayWrite(t, client, []byte("request"))
			if tc.mode&relayUp == 0 {
				relayRead(t, server, []byte("request"))
			} else {
				waitRelay(t, func() bool { return r.upHeld.Load() > 0 })
				relayTimesOut(t, server)
			}
			relayWrite(t, server, []byte("response"))
			waitRelay(t, func() bool { return r.downHeld.Load() > 0 })
			relayTimesOut(t, client)
			var b [1]byte
			r.Resume()
			if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			n, err := client.Read(b[:])
			var timed net.Error
			if n != 0 || err == nil || (errors.As(err, &timed) && timed.Timeout()) {
				t.Fatal("resume replayed old TLS stream bytes")
			}
			fresh, err := net.DialTimeout("tcp", strings.TrimPrefix(r.endpoint(), "https://"), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fresh.Close() }) // Private fixture connection.
			peer, err := upstream.Accept()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = peer.Close() }) // Private fixture connection.
			relayWrite(t, fresh, []byte("new request"))
			relayRead(t, peer, []byte("new request"))
			relayWrite(t, peer, []byte("new response"))
			relayRead(t, fresh, []byte("new response"))
		})
	}
}

func TestFaultRelayCloseBlockedStreams(t *testing.T) {
	r, client, server, _ := relayPair(t)
	r.Block(relayUp | relayDown)
	relayWrite(t, client, []byte("pending request"))
	relayWrite(t, server, []byte("pending response"))
	waitRelay(t, func() bool { return r.upHeld.Load() > 0 && r.downHeld.Load() > 0 })
	results := make(chan bool, 2)
	for range 2 {
		go func() { results <- r.Close() }()
	}
	for range 2 {
		if !<-results {
			t.Fatal("close left a blocked copy or accept goroutine")
		}
	}
}

func relayTimesOut(t *testing.T, c net.Conn) {
	t.Helper()
	if err := c.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	n, err := c.Read(b[:])
	var timeout net.Error
	if n != 0 || !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatal("blocked direction did not remain open without delivering bytes")
	}
}
