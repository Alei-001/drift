package core

import "fmt"

type ProgressFunc func(current, total int, description string)

func NullProgress(current, total int, description string) {}

func ConsoleProgress(current, total int, description string) {
	if total == 0 {
		return
	}
	if current == 1 || current == total || current%10 == 0 {
		fmt.Printf("\r%s: %d/%d", description, current, total)
	}
	if current == total {
		fmt.Println()
	}
}
