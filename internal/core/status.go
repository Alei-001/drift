package core

type StatusCode byte

const (
	Unmodified StatusCode = ' '
	Untracked  StatusCode = '?'
	Modified   StatusCode = 'M'
	Added      StatusCode = 'A'
	Deleted    StatusCode = 'D'
)

func (s StatusCode) String() string {
	return string(s)
}

type FileStatus struct {
	Staging  StatusCode
	Worktree StatusCode
}

type Status map[string]*FileStatus

func (s Status) File(path string) *FileStatus {
	if _, ok := s[path]; !ok {
		s[path] = &FileStatus{
			Staging:  Unmodified,
			Worktree: Unmodified,
		}
	}
	return s[path]
}

func (s Status) IsClean() bool {
	for _, fs := range s {
		if fs.Staging != Unmodified || fs.Worktree != Unmodified {
			return false
		}
	}
	return true
}
