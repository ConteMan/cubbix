package lid

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type FileJournal struct{ Path string }

// normalizeJournalGroup repairs group inheritance only after the descriptor's
// owner and permissions have been checked. A root-created directory can inherit
// admin from its parent on macOS; its 0700 mode still protects the contents.
func normalizeJournalGroup(uid, gid, euid uint32, chown func() error) error {
	if uid != euid {
		return fmt.Errorf("unsafe journal owner")
	}
	if euid == 0 && gid != 0 {
		return chown()
	}
	return nil
}
func secureJournalFile(f *os.File, directory bool) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || info.IsDir() != directory || !directory && (!info.Mode().IsRegular() || st.Nlink != 1) || info.Mode().Perm() != mode {
		return fmt.Errorf("unsafe journal metadata")
	}
	return normalizeJournalGroup(st.Uid, st.Gid, uint32(os.Geteuid()), func() error { return f.Chown(-1, 0) })
}
func openJournalDir(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	if err = secureJournalFile(f, true); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}
func (j FileJournal) Load() (*Journal, error) {
	dir, err := openJournalDir(filepath.Dir(j.Path))
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	f, err := os.OpenFile(j.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = secureJournalFile(f, false); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return &Journal{}, err
	}
	var v Journal
	err = json.Unmarshal(data, &v)
	return &v, err
}
func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (j FileJournal) Save(v Journal) error {
	dir := filepath.Dir(j.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	d, err := openJournalDir(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if err = secureJournalFile(f, false); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), j.Path); err != nil {
		return err
	}
	return syncDir(dir)
}
func (j FileJournal) Remove() error {
	if err := os.Remove(j.Path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDir(filepath.Dir(j.Path))
}
