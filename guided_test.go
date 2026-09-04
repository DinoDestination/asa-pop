package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// The guided flow, driven end to end against a real synthetic RCON server.
//
// It cannot be tested by piping into the built binary: a pipe is not a console,
// so `stdinIsConsole` refuses the guided path there - which is the guard that
// keeps the scheduled task out of a prompt and is therefore working correctly.
// So the flow takes its reader, its writer and its secret-reader as fields, and
// the test supplies all three.

func drive(t *testing.T, answers []string, password string, port int) (string, int) {
	t.Helper()
	var out bytes.Buffer
	u := &ui{
		in:  bufio.NewReader(strings.NewReader(strings.Join(answers, "\n") + "\n")),
		out: &out,
		secret: func(string) (string, error) {
			return password, nil
		},
	}
	code := guided(u)
	return out.String(), code
}

func TestGuidedHappyPath(t *testing.T) {
	addr := startControl(t, "hunter2", 12)
	_, port := hostPort(t, addr)

	// port, (password injected), map name, another? no, save? no, pause
	out, code := drive(t, []string{fmt.Sprint(port), "The Island", "n", "n", ""}, "hunter2", port)

	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{
		"WORKED. 12 player(s) online right now",
		"Worked on 1 of your server(s)",
		"SELECT ALL THE TEXT ABOVE",
		"safe to paste",
		"Press Enter to close this window",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output is missing %q:\n%s", want, out)
		}
	}
	// THE PASSWORD IS NEVER IN THE OUTPUT. The whole point of this flow is that
	// the owner copies what it shows and sends it to us.
	if strings.Contains(out, "hunter2") {
		t.Fatalf("the password appears in output the owner is told to paste:\n%s", out)
	}
}

func TestGuidedReportsAFullServerAndSaysItReassembled(t *testing.T) {
	// 90 players crosses the 4096 boundary. The owner is told, because "2
	// packets" is the difference between a number we trust and one we do not.
	addr := startControl(t, "hunter2", 90)
	_, port := hostPort(t, addr)
	out, code := drive(t, []string{fmt.Sprint(port), "", "n", "n", ""}, "hunter2", port)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "WORKED. 90 player(s)") {
		t.Fatalf("did not count 90:\n%s", out)
	}
	if !strings.Contains(out, "reassembled across a packet boundary") {
		t.Fatalf("did not say it reassembled:\n%s", out)
	}
}

func TestGuidedEmptyServerIsASuccess(t *testing.T) {
	// Nobody online is a real answer and must read as one - an owner testing at
	// 4am must not conclude their setup is broken.
	addr := startControl(t, "hunter2", 0)
	_, port := hostPort(t, addr)
	out, code := drive(t, []string{fmt.Sprint(port), "", "n", "n", ""}, "hunter2", port)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "WORKED. 0 player(s)") {
		t.Fatalf("an empty server did not read as working:\n%s", out)
	}
}

func TestGuidedWrongPasswordExplainsItself(t *testing.T) {
	addr := startControl(t, "hunter2", 3)
	_, port := hostPort(t, addr)
	out, code := drive(t, []string{fmt.Sprint(port), "n", ""}, "wrong-password", port)

	if code != 1 {
		t.Fatalf("a total failure exited %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "REFUSED the password") {
		t.Fatalf("did not say the password was refused:\n%s", out)
	}
	// AND IT STILL TELLS THEM WHAT TO DO NEXT, rather than ending on a failure.
	if !strings.Contains(out, "RCONEnabled=True") {
		t.Fatalf("a failed run gave no next step:\n%s", out)
	}
	if strings.Contains(out, "wrong-password") {
		t.Fatalf("the attempted password is in the output:\n%s", out)
	}
}

// EVERY EXIT FROM THE GUIDED FLOW PAUSES. Double-clicking opens a window that
// closes the instant the program exits, so a path that returns without pausing
// shows the owner nothing at all - and looks, from the outside, exactly like a
// program that ran fine.
func TestGuidedAlwaysPauses(t *testing.T) {
	addr := startControl(t, "hunter2", 5)
	_, port := hostPort(t, addr)

	cases := []struct {
		name     string
		answers  []string
		password string
	}{
		{"success", []string{fmt.Sprint(port), "", "n", "n", ""}, "hunter2"},
		{"wrong password", []string{fmt.Sprint(port), "n", ""}, "nope"},
		{"no password typed", []string{fmt.Sprint(port), "n", ""}, ""},
		{"closed port", []string{"27099", "n", ""}, "hunter2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, _ := drive(t, c.answers, c.password, port)
			if !strings.Contains(out, "Press Enter to close this window") {
				t.Fatalf("this path does not pause - the window would vanish:\n%s", out)
			}
		})
	}
}

func TestGuidedRejectsANonsensePortRatherThanGuessing(t *testing.T) {
	addr := startControl(t, "hunter2", 4)
	_, port := hostPort(t, addr)
	// "abc" must be refused and re-asked, not silently coerced to 27020 - which
	// would probe the wrong port and send them to check a firewall.
	out, code := drive(t, []string{"abc", fmt.Sprint(port), "", "n", "n", ""}, "hunter2", port)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "That is not a port number") {
		t.Fatalf("a nonsense port was accepted:\n%s", out)
	}
	if !strings.Contains(out, "WORKED. 4 player(s)") {
		t.Fatalf("did not recover after the bad answer:\n%s", out)
	}
}

func TestParsePortAndYesNo(t *testing.T) {
	if p, ok := ParsePort("", 27020); !ok || p != 27020 {
		t.Fatal("an empty answer should take the default")
	}
	if p, ok := ParsePort(" 27021 ", 27020); !ok || p != 27021 {
		t.Fatal("surrounding spaces should be tolerated")
	}
	for _, bad := range []string{"abc", "0", "70000", "27020x", "-1"} {
		if _, ok := ParsePort(bad, 27020); ok {
			t.Fatalf("%q was accepted as a port", bad)
		}
	}
	if !ParseYesNo("", true) || ParseYesNo("", false) {
		t.Fatal("an empty answer should take the default")
	}
	if !ParseYesNo("Y", false) || !ParseYesNo("yes", false) {
		t.Fatal("yes was not read as yes")
	}
	if ParseYesNo("n", true) || ParseYesNo("NO", true) {
		t.Fatal("no was not read as no")
	}
}

// A password may legitimately begin or end with a space, and trimming it would
// produce a refusal that is our fault and reads as theirs - sending the owner
// to rotate a credential that was fine.
func TestCleanSecretKeepsSpaces(t *testing.T) {
	if got := CleanSecret("  spaced pass  \r\n"); got != "  spaced pass  " {
		t.Fatalf("got %q", got)
	}
}
