package storage

import (
	"os"
)

type fileLock struct {
	file *os.File
}

func acquireFileLock(lockPath string) (*fileLock, error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	if err := platformLock(f); err != nil {
		f.Close()
		return nil, err
	}

	return &fileLock{file: f}, nil
}

func (fl *fileLock) release() {
	platformUnlock(fl.file)
	fl.file.Close()
}
