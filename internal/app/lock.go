package app

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// SQLite persistence is shared by a single service owner. An OS-held lock dies
// with the process, preventing another replica from replaying external effects.
func (a *App) AcquireWorkerLock() (func(), error) {
	f, e := os.OpenFile(filepath.Join(a.dataDir, "service.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, errors.New("another ZenSync service owns this data directory")
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
