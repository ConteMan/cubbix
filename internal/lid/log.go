package lid

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

const HelperLogPath = "/Library/Logs/Cubbix/helper.log"
const HelperLogLimit = 1 << 20

// HelperLog keeps at most two bounded files in a trusted directory. It never
// follows symlinks or appends to files owned by another user (root in production).
type HelperLog struct {
	mu    sync.Mutex
	path  string
	limit int64
}

func OpenHelperLog(path string) (*log.Logger, error) {
	w := &HelperLog{path: path, limit: HelperLogLimit}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	dir, err := os.OpenFile(filepath.Dir(path), os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return nil, errors.New("unsafe log directory")
	}
	if err = normalizeJournalGroup(st.Uid, st.Gid, uint32(os.Geteuid()), func() error { return dir.Chown(-1, 0) }); err != nil {
		return nil, err
	}
	if _, err := w.Write(nil); err != nil {
		return nil, err
	}
	return log.New(w, "cubbix-helper ", log.LstdFlags|log.LUTC), nil
}
func (w *HelperLog) open() (*os.File, error) {
	flags := os.O_APPEND | os.O_WRONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	f, err := os.OpenFile(w.path, flags|os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		if err = f.Chmod(0644); err != nil {
			f.Close()
			return nil, err
		}
	} else if os.IsExist(err) {
		f, err = os.OpenFile(w.path, flags, 0)
	}
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0644 || st.Nlink != 1 {
		f.Close()
		return nil, errors.New("unsafe log file")
	}
	if err = normalizeJournalGroup(st.Uid, st.Gid, uint32(os.Geteuid()), func() error { return f.Chown(-1, 0) }); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func (w *HelperLog) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if int64(len(p)) > w.limit {
		return 0, io.ErrShortWrite
	}
	f, err := w.open()
	if err != nil {
		return 0, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return 0, err
	}
	if info.Size()+int64(len(p)) > w.limit {
		f.Close()
		if err = os.Rename(w.path, w.path+".1"); err != nil {
			return 0, err
		}
		f, err = w.open()
		if err != nil {
			return 0, err
		}
	}
	n, err := f.Write(p)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return n, err
}

// LogResponse logs only protocol-controlled fields, never ids, client strings,
// request reasons, paths or command output.
func LogResponse(logf func(string, ...any), op string, r Response) {
	if logf == nil {
		return
	}
	switch op {
	case "hello", "status", "ping", "enable", "disable":
	default:
		op = "invalid"
	}
	value := -1
	if r.Value != nil {
		value = *r.Value
	}
	logf("request op=%s ok=%t code=%s state=%s value=%d detail=%s", op, r.OK, r.Code, r.State, value, r.Detail)
}
