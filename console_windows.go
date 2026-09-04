//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"syscall"
)

// THE PASSWORD IS NOT ECHOED, AND THAT IS A REQUIREMENT RATHER THAN A POLISH.
//
// The guided flow exists so a non-technical owner can run a check and PASTE US
// WHAT IT SHOWS. If the password were echoed, it would be sitting in the same
// console window as the output they are about to select, copy and send - or
// screenshot. The very change that made this usable would have introduced a
// credential leak into the support channel, on a tool whose entire pitch is
// that we never receive the password.
//
// So it is masked, with the Windows console API through Go's own `syscall`
// package - no third-party dependency, because the binary containing no code
// but ours and the standard library is the trust story.

const enableEchoInput = 0x0004

// `syscall` exposes GetConsoleMode and NOT SetConsoleMode, so the setter is
// bound from kernel32 by hand. STILL NO DEPENDENCY: `golang.org/x/term` does
// exactly this and would do it better, and it is not worth the binary
// containing code that is not ours or the standard library's - which is the
// whole argument for anybody trusting this download.
var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleMd = kernel32.NewProc("SetConsoleMode")
)

func setConsoleMode(h syscall.Handle, mode uint32) error {
	r, _, err := procSetConsoleMd.Call(uintptr(h), uintptr(mode))
	if r == 0 {
		return err
	}
	return nil
}

// stdinIsConsole reports whether a human is watching.
//
// This is what keeps the scheduled task out of the guided path (see mode.go).
// A task started by schtasks has no console, so the handle call fails and this
// is false - which is the safe direction: the worst case is a first-time owner
// being shown a plain error instead of a prompt, rather than a background task
// blocking on input forever.
func stdinIsConsole() bool {
	var mode uint32
	handle := syscall.Handle(os.Stdin.Fd())
	if err := syscall.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	return true
}

// readSecret reads a line with echo off, and restores the console mode however
// it returns.
func readSecret(prompt string) (string, error) {
	fmt.Print(prompt)

	handle := syscall.Handle(os.Stdin.Fd())
	var original uint32
	restore := func() {}
	if err := syscall.GetConsoleMode(handle, &original); err == nil {
		// ECHO OFF. If this fails we still read the line - a password typed in
		// the clear is bad, and refusing to let somebody set the tool up at all
		// is worse. The prompt says which happened.
		if err := setConsoleMode(handle, original&^enableEchoInput); err == nil {
			restore = func() { _ = setConsoleMode(handle, original) }
		} else {
			fmt.Print("\n  (this console will not hide typing - it will be visible)\n  ")
		}
	}
	// RESTORED ON EVERY PATH. A console left with echo disabled is one where the
	// owner types into an apparently dead window for the rest of the session.
	defer restore()

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	// The newline they typed was swallowed with the echo, so the next thing
	// printed would otherwise land on the prompt line.
	fmt.Println()
	if err != nil {
		return "", err
	}
	return CleanSecret(line), nil
}
