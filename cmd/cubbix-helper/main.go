package main

import (
	"context"
	"github.com/ConteMan/cubbix/internal/lid"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if os.Geteuid() != 0 || len(os.Args) != 1 {
		log.Fatal("helper requires system launch")
	}
	logger, err := lid.OpenHelperLog(lid.HelperLogPath)
	if err != nil {
		// Logging failure must not prevent journal recovery.
		log.Print("helper file log unavailable; using stderr")
	} else {
		log.SetOutput(logger.Writer())
	}
	log.Printf("helper start revision=%s", lid.Version)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	stopWatchdog := helperExitWatchdog(ctx, 12*time.Second, func() {
		log.Print("helper shutdown timed out; journal retained")
		os.Exit(1)
	})
	defer stopWatchdog()
	// Production write capability is constructed here only. Tests inject a fake.
	sleep := lid.PMSet{Logf: log.Printf, Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).Output()
	}}
	engine := lid.NewEngine(&sleep, lid.SystemPower{}, lid.FileJournal{Path: lid.JournalPath})
	engine.Logf = log.Printf
	if err := engine.Start(); err != nil {
		log.Fatal("recovery initialization failed")
	}
	listener, err := lid.Listen(lid.SocketPath)
	if err != nil {
		_ = engine.Shutdown()
		log.Fatal("listener unavailable")
	}
	defer listener.Close()
	server := lid.Server{Engine: engine, Verify: func(c *net.UnixConn) error { return lid.VerifyPeer(c, lid.ClientRequirement) }}
	if err = server.Serve(ctx, listener); err != nil {
		log.Print("listener stopped with error; recovery may be pending")
	}
}

// The watchdog is a last resort for a stuck verifier, framework or filesystem.
// Ordinary shutdown waits for handlers and confirmed restoration first.
func helperExitWatchdog(ctx context.Context, timeout time.Duration, exit func()) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-done:
			return
		}
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-timer.C:
			exit()
		case <-done:
		}
	}()
	return func() { close(done) }
}
