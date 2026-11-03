package lid

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type PeerVerifier func(*net.UnixConn) error
type Server struct {
	Engine *Engine
	Verify PeerVerifier
	next   atomic.Uint64
}

// Serve checks identity before reading any bytes or touching the lease owner.
func (s *Server) Serve(ctx context.Context, l *net.UnixListener) error {
	if s.Verify == nil {
		return ErrUntrusted
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			l.Close()
		case <-done:
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				s.Engine.Tick()
			}
		}
	}()
	defer wg.Wait()
	for {
		c, err := l.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer c.Close()
			if s.Verify(c) != nil {
				return
			}
			owner := s.next.Add(1)
			defer s.Engine.Disconnect(owner)
			stop := context.AfterFunc(ctx, func() { c.Close() })
			defer stop()
			reader := bufio.NewReaderSize(c, MaxLine+1)
			for {
				c.SetReadDeadline(time.Now().Add(Lease))
				if _, err := reader.Peek(1); err != nil {
					return
				}
				c.SetReadDeadline(time.Now().Add(5 * time.Second))
				line, err := reader.ReadSlice('\n')
				if err != nil || len(line) > MaxLine {
					return
				}
				var req Request
				if json.Unmarshal(line, &req) != nil {
					writeResponse(c, Response{V: Protocol, Protocol: Protocol, Helper: Version, Code: "bad-request"})
					return
				}
				resp := s.Engine.Handle(owner, req)
				if writeResponse(c, resp) != nil {
					return
				}
				if resp.Code == "bad-request" || resp.Code == "protocol-mismatch" {
					return
				}
			}
		}()
	}
}
func writeResponse(c net.Conn, r Response) error {
	c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return json.NewEncoder(c).Encode(r)
}
func Listen(path string) (*net.UnixListener, error) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0666); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

type Client struct {
	mu     sync.Mutex
	conn   *net.UnixConn
	reader *bufio.Reader
	id     uint64
}

func Dial(path string, verify PeerVerifier) (*Client, error) {
	if verify == nil {
		return nil, ErrUntrusted
	}
	c, err := net.DialTimeout("unix", path, 3*time.Second)
	if err != nil {
		return nil, err
	}
	u := c.(*net.UnixConn)
	if err = verify(u); err != nil {
		u.Close()
		return nil, err
	}
	return &Client{conn: u, reader: bufio.NewReaderSize(u, MaxLine+1)}, nil
}
func (c *Client) Call(req Request) (Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return Response{}, net.ErrClosed
	}
	c.id++
	req.V = Protocol
	req.ID = time.Now().UTC().Format("20060102T150405") + "_" + stringID(c.id)
	timeout := 5 * time.Second
	if req.Op == "hello" {
		timeout = 2 * time.Second
	}
	c.conn.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(c.conn).Encode(req); err != nil {
		return Response{}, err
	}
	line, err := c.reader.ReadSlice('\n')
	if err != nil {
		return Response{}, err
	}
	if len(line) > MaxLine {
		return Response{}, io.ErrShortBuffer
	}
	var resp Response
	if err = json.Unmarshal(line, &resp); err != nil {
		return resp, err
	}
	if resp.V != Protocol || resp.Protocol != Protocol || resp.ID != req.ID {
		return resp, errors.New("protocol-mismatch")
	}
	return resp, nil
}
func stringID(n uint64) string {
	const digits = "0123456789abcdef"
	var b [16]byte
	for i := 15; i >= 0; i-- {
		b[i] = digits[n&15]
		n >>= 4
	}
	return string(b[:])
}
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}
