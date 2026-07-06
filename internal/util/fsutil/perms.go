package fsutil

import "os"

// DefaultDirPerm is the default permission for created directories.
const DefaultDirPerm os.FileMode = 0755

// DefaultFilePerm is the default permission for created files.
const DefaultFilePerm os.FileMode = 0644
