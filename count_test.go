package main

import (
	"fmt"
	"strings"
	"testing"
)

func line(i int) string {
	return fmt.Sprintf("%d. Survivor%03d, 0002%s", i, i, strings.Repeat("a1b2c3d4", 4))
}

func roster(n int) string {
	out := make([]string, n)
	for i := range out {
		out[i] = line(i)
	}
	return strings.Join(out, "\n")
}

func TestCountPlayers(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		want  int
		wanOK bool
	}{
		{"the empty answer is a FACT", "No Players Connected", 0, true},
		{"with a full stop", "No Players Connected.", 0, true},
		{"case does not matter", "no players connected", 0, true},
		{"silence is not zero", "", 0, false},
		{"whitespace is not zero", "   \n  ", 0, false},
		{"a non-roster reply is not zero", arkSentinel, 0, false},
		{"a roster counts", "0. A, 0002x\n1. B, 0002y", 2, true},
		{"one player", "0. Solo, 0002z", 1, true},
		{"trailing newline is not a player", "0. A, 0002x\n", 1, true},
		{"CRLF is not a player", "0. A, 0002x\r\n1. B, 0002y\r\n", 2, true},
		{"a half-understood answer is not a count", "0. A, 0002x\nsomething else", 0, false},
		{"an error string is not a count", "Command not recognised", 0, false},
		{"a name with a comma still parses", "0. Bob, the Builder, 0002x", 1, true},
		{"90 players", roster(90), 90, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := CountPlayers(c.in)
			if ok != c.wanOK {
				t.Fatalf("ok = %v, want %v", ok, c.wanOK)
			}
			if got != c.want {
				t.Fatalf("count = %d, want %d", got, c.want)
			}
		})
	}
}

// THE CASE THAT MAKES REASSEMBLY LOAD-BEARING, and the reason this file exists
// rather than trusting the counter to notice.
//
// A response truncated at the 4096-byte frame boundary usually ends mid-line,
// and the counter rejects that. But when the split happens to land ON a
// newline, the truncation parses PERFECTLY as a smaller roster - a confident,
// well-formed, wrong number that nothing downstream could detect.
//
// So the counter cannot be the thing that catches a short read. The sentinel
// and the reassembly in rcon.go are, and this test states plainly why.
func TestBoundaryTruncationParsesCleanlyAndIsWrong(t *testing.T) {
	full := roster(90)
	n, ok := CountPlayers(full)
	if !ok || n != 90 {
		t.Fatalf("full roster: %d %v", n, ok)
	}

	lines := strings.Split(full, "\n")
	cut := strings.Join(lines[:74], "\n")
	if len(cut) >= chunkSize {
		t.Fatalf("the cut is %d bytes, which is not under the frame boundary", len(cut))
	}

	got, ok := CountPlayers(cut)
	if !ok {
		t.Fatal("a boundary-aligned truncation was rejected - if this ever becomes true, " +
			"the comment on this test is wrong and reassembly is less critical than stated")
	}
	if got != 74 {
		t.Fatalf("truncated count = %d, want 74", got)
	}
	// The point: 74 != 90, it parsed cleanly, and no amount of care in the
	// counter could have known.
	if got == 90 {
		t.Fatal("truncation was somehow invisible")
	}
}

func TestRedactRemovesNamesAndIds(t *testing.T) {
	in := "0. SecretName, 0002deadbeefdeadbeefdeadbeefdeadbe\n1. Another, 0002cafe"
	out := Redact(in)
	for _, leaked := range []string{"SecretName", "Another", "deadbeef", "cafe"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("redaction leaked %q:\n%s", leaked, out)
		}
	}
	// It still shows the SHAPE, which is the whole point of a redacted probe.
	if !strings.Contains(out, "0.") || !strings.Contains(out, "<name redacted>") {
		t.Fatalf("redaction destroyed the shape:\n%s", out)
	}
	if len(strings.Split(out, "\n")) != 2 {
		t.Fatalf("redaction changed the line count:\n%s", out)
	}
}

func TestRedactLeavesNonRosterLinesAlone(t *testing.T) {
	// "No Players Connected" carries nothing private and is the single most
	// useful line to see in a pasted probe.
	if got := Redact("No Players Connected"); got != "No Players Connected" {
		t.Fatalf("got %q", got)
	}
}
