package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ConteMan/cubbix/internal/cli"
)

// translateStub answers with a JSON line followed by a fixed-length body
// frame, just like the app's writeTranslationResult.
func translateStub(t *testing.T, path string, handle func(cli.Request, []byte) (*cli.Response, []byte)) {
	t.Helper()
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.AcceptUnix()
			if err != nil {
				return
			}
			go func(conn *net.UnixConn) {
				defer conn.Close()
				reader := bufio.NewReaderSize(conn, cli.MaxLine+1)
				line, err := reader.ReadBytes('\n')
				if err != nil {
					return
				}
				var req cli.Request
				if err := json.Unmarshal(line, &req); err != nil {
					return
				}
				body := make([]byte, req.Bytes)
				if req.Bytes > 0 {
					if _, err := io.ReadFull(reader, body); err != nil {
						return
					}
				}
				response, out := handle(req, body)
				if response == nil {
					return
				}
				response.V, response.ID = cli.Protocol, req.ID
				data, err := cli.EncodeLine(*response)
				if err != nil {
					return
				}
				if _, err := conn.Write(data); err != nil {
					return
				}
				if response.Bytes > 0 && len(out) >= response.Bytes {
					_, _ = conn.Write(out[:response.Bytes])
				}
			}(conn)
		}
	}()
}

func TestIntegrationTranslateFrames(t *testing.T) {
	path := tempSocket(t)
	translation := "第一行\n第二行"
	type seen struct {
		req  cli.Request
		body string
	}
	requests := make(chan seen, 4)
	translateStub(t, path, func(req cli.Request, body []byte) (*cli.Response, []byte) {
		requests <- seen{req, string(body)}
		return &cli.Response{OK: true, Bytes: len(translation),
			State:     &cli.State{Version: "0.6.1", Tiles: cli.Tiles{Awake: true}},
			Translate: &cli.TranslateResult{Source: "auto", Target: "zh-Hans", Model: "m"}}, []byte(translation)
	})
	for _, scenario := range []struct {
		args []string
		json bool
	}{
		{[]string{"translate", "Hello iOS", "--to", "en", "--socket", path}, false},
		{[]string{"translate", "Hello iOS", "--to", "en", "--json", "--socket", path}, true},
	} {
		args := scenario.args
		stdout, stderr, code := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v exit = %d (%q)", args, code, stderr)
		}
		if scenario.json {
			line := strings.TrimSuffix(stdout, "\n")
			if strings.Contains(line, "\n") {
				t.Fatalf("--json printed more than one line: %q", stdout)
			}
			var result cli.Result
			if err := json.Unmarshal([]byte(line), &result); err != nil {
				t.Fatalf("%q: %v", line, err)
			}
			if result.Translate == nil || result.Translate.Text != translation || result.Bytes != len(translation) || result.State == nil {
				t.Fatalf("--json = %+v", result)
			}
		} else if stdout != translation {
			t.Fatalf("human output = %q, want %q", stdout, translation)
		}
		select {
		case got := <-requests:
			if got.req.Action != cli.ActionTranslate || got.req.To != "en" ||
				got.req.Bytes != len("Hello iOS") || got.body != "Hello iOS" || got.req.Notify {
				t.Fatalf("request = %+v body=%q", got.req, got.body)
			}
		default:
			t.Fatal("the tool did not send a request")
		}
	}
}

func TestIntegrationTranslateLongBody(t *testing.T) {
	path := tempSocket(t)
	translation := strings.Repeat("译", 4000)
	translateStub(t, path, func(cli.Request, []byte) (*cli.Response, []byte) {
		return &cli.Response{OK: true, Bytes: len(translation),
			State: &cli.State{Version: "0.6.1", Tiles: cli.Tiles{Awake: true, JSON: true},
				JSON: cli.JSONState{Windows: 2}},
			Translate: &cli.TranslateResult{Source: "auto", Target: "zh-Hans", Model: "group/model"}}, []byte(translation)
	})
	stdout, stderr, code := runCLI(t, "translate", "--text", "hello", "--json", "--socket", path)
	if code != 0 {
		t.Fatalf("exit = %d (%q)", code, stderr)
	}
	var result cli.Result
	if err := json.Unmarshal([]byte(strings.TrimSuffix(stdout, "\n")), &result); err != nil {
		t.Fatalf("%q: %v", stdout, err)
	}
	if result.Translate == nil || result.Translate.Text != translation || result.Bytes != len(translation) || result.State == nil {
		got := 0
		if result.Translate != nil {
			got = len(result.Translate.Text)
		}
		t.Fatalf("4000 characters did not arrive: %d bytes of text", got)
	}
}

func TestIntegrationTranslateEmptyTextIsUsage(t *testing.T) {
	path := tempSocket(t)
	var called atomic.Bool
	translateStub(t, path, func(cli.Request, []byte) (*cli.Response, []byte) {
		called.Store(true)
		return &cli.Response{OK: true}, nil
	})
	_, stderr, code := runCLI(t, "translate", "--text=", "--socket", path)
	if code != 2 || !strings.Contains(stderr, "needs text") {
		t.Fatalf("empty text = %d %q", code, stderr)
	}
	if called.Load() {
		t.Fatal("an empty translation reached the app")
	}
}
