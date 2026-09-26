package main

import (
	"bufio"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("RUN_SERVER_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func waitDial(t *testing.T, addr string, wantUp bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
		}
		if (err == nil) == wantUp {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("listener on %s never reached up=%v", addr, wantUp)
}

func TestSIGTERMDrainsInFlightRequest(t *testing.T) {
	addr := freeAddr(t)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(),
		"RUN_SERVER_MAIN=1",
		"ADDR="+addr,
		"YT_API_KEY=test",
		"STATIC_DIR="+t.TempDir(),
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	waitDial(t, addr, true)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	body := `{"input":""}`
	if _, err := conn.Write([]byte("POST /api/playlists HTTP/1.1\r\nHost: test\r\nContent-Type: application/json\r\nExpect: 100-continue\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	cont, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cont.StatusCode != http.StatusContinue {
		t.Fatalf("status = %d, want %d", cont.StatusCode, http.StatusContinue)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waitDial(t, addr, false)

	if _, err := conn.Write([]byte(body)); err != nil {
		t.Fatalf("writing body after SIGTERM: %v", err)
	}
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("in-flight request was dropped: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatalf("server exit after drain: %v", err)
	}
}
