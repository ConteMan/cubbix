package lid

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type FileJournal struct{ Path string }

func (j FileJournal) Load() (*Journal, error) {
	dir, err := os.Lstat(filepath.Dir(j.Path))
	if err != nil {
		return nil, err
	}
	if !dir.IsDir() || dir.Mode().Perm() != 0700 || !journalOwner(dir) {
		return nil, fmt.Errorf("unsafe journal directory")
	}
	info, err := os.Lstat(j.Path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !journalOwner(info) {
		return nil, fmt.Errorf("unsafe journal")
	}
	data, err := os.ReadFile(j.Path)
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
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 || !journalOwner(info) {
		return fmt.Errorf("unsafe journal directory")
	}
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

func journalOwner(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uint32(os.Geteuid()) && (os.Geteuid() != 0 || st.Gid == 0)
}
