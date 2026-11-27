package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSQLiteFixtureIsNotOpenedForInnoDBPreflight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "staging.db")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", "file:"+path)
	if err := check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("preflight touched SQLite fixture: %v", err)
	}
}

func TestPreflightDeadlineIncludesMySQLHandshake(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	// Existing MySQL deployments may rely on the API's default driver.
	t.Setenv("DB_DRIVER", "")
	t.Setenv("DB_DSN", "test:secret-sentinel@tcp("+listener.Addr().String()+")/preflight")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- check(ctx) }()
	// Accept the connection but withhold the server's initial handshake packet.
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	select {
	case err := <-done:
		if ctx.Err() != context.DeadlineExceeded || err == nil || strings.Contains(err.Error(), "secret-sentinel") {
			t.Fatalf("unexpected preflight result: deadline=%v err=%v", ctx.Err(), err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("preflight remained blocked in the first MySQL handshake")
	}
}
