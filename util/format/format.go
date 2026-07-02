package format

import "fmt"

// Bytes returns a human-readable size string (e.g. "2.5 MB").
// Negative sizes are formatted with a leading minus sign.
func Bytes(size int64) string {
	switch {
	case size < 0:
		return fmt.Sprintf("-%s", Bytes(-size))
	case size < 1024:
		return fmt.Sprintf("%d B", size)
	case size < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	case size < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
	}
}
