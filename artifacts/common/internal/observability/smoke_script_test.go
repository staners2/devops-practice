package observability

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCISmokeRunsBothMetricsListenersAndShutsDown(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	workdir := t.TempDir()
	if err := os.Mkdir(filepath.Join(workdir, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"api", "worker", "migrate", "smoke"} {
		wrapper := fmt.Sprintf("#!/bin/sh\nexec '%s' -test.run=TestSmokeProcess -- %s\n", executable, role)
		if err := os.WriteFile(filepath.Join(workdir, "bin", role), []byte(wrapper), 0700); err != nil {
			t.Fatal(err)
		}
	}
	// The script's dependency health check is outside the listener lifecycle under test.
	if err := os.WriteFile(filepath.Join(workdir, "bin", "curl"), []byte("#!/bin/sh\nprintf '%s' '{\"postgres\":\"ok\",\"redis\":\"ok\",\"kafka\":\"ok\"}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../scripts/ci/smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), "OBS_SMOKE_CHILD=1", "PATH="+filepath.Join(workdir, "bin")+":"+os.Getenv("PATH"), "METRICS_ADDR=127.0.0.1:9090")
	if output, err := cmd.CombinedOutput(); err != nil {
		apiLog, _ := os.ReadFile(filepath.Join(workdir, "api.log"))
		workerLog, _ := os.ReadFile(filepath.Join(workdir, "worker.log"))
		t.Fatalf("CI smoke failed: %v\n%s\napi: %s\nworker: %s", err, output, apiLog, workerLog)
	}
	for _, role := range []string{"api", "worker"} {
		addr, err := os.ReadFile(filepath.Join(workdir, role+".addr"))
		if err != nil {
			t.Fatal(err)
		}
		connection, err := net.DialTimeout("tcp", string(addr), time.Second)
		if err == nil {
			_ = connection.Close()
			t.Fatalf("%s listener still open after cleanup", role)
		}
	}
}

func TestSmokeProcess(t *testing.T) {
	if os.Getenv("OBS_SMOKE_CHILD") != "1" {
		return
	}
	role := os.Args[len(os.Args)-1]
	if role == "migrate" {
		os.Exit(0)
	}
	if role == "smoke" {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			ready := 0
			for _, component := range []string{"api", "worker"} {
				addr, err := os.ReadFile(component + ".addr")
				if err != nil {
					continue
				}
				client := &http.Client{Timeout: time.Second}
				response, err := client.Get("http://" + string(addr) + "/metrics")
				if err != nil {
					continue
				}
				body, _ := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if response.StatusCode == 200 && strings.Contains(string(body), "go_goroutines") {
					ready++
				}
			}
			if ready == 2 {
				os.Exit(0)
			}
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Fprintln(os.Stderr, "both metrics listeners must be available")
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", os.Getenv("METRICS_ADDR"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(role+".addr", []byte(listener.Addr().String()), 0600); err != nil {
		os.Exit(1)
	}
	server := Server(listener.Addr().String(), New(role))
	go func() { _ = server.Serve(listener) }()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(shutdown); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
