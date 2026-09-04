//go:build !windows

package main

import (
	"bufio"
	"fmt"
	"os"
)

// The non-Windows build, which exists so `go test ./...` compiles and runs on
// the Linux runner that produces the Windows binary. Linux is a deliberate
// minority for self-hosted ASA (it needs Wine, and few bother), so this is a
// working fallback rather than a supported target.
//
// IT SAYS SO WHEN IT CANNOT HIDE THE TYPING. A silent unmasked prompt on a
// platform we do not build for would be the same leak the Windows path exists
// to prevent, arriving where nobody looked.

func stdinIsConsole() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func readSecret(prompt string) (string, error) {
	fmt.Print(prompt)
	fmt.Print("\n  (typing will be visible on this platform)\n  ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return CleanSecret(line), nil
}
