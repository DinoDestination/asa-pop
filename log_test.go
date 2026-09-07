package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The log is the answer to "is this thing still running", so the tests are
// about what an owner will actually read: that a working run leaves a line, a
// broken one leaves a line that names the failure, and that neither line ever
// contains a player's name, a player's id, or a credential.

func TestLogLineRecordsASuccess(t *testing.T) {
	n := 7
	line := LogLine("2026-09-07 14:03:11", "The Island", "ok", &n, "reported")

	for _, want := range []string{"2026-09-07 14:03:11", "The Island", "ok", "count=7", "reported"} {
		if !strings.Contains(line, want) {
			t.Fatalf("a successful run's line is missing %q:\n%s", want, line)
		}
	}
}

func TestLogLineNamesTheFailure(t *testing.T) {
	// EVERY FAILING BRANCH, with the text `doReport` actually passes it. A test
	// that invented its own strings would pass while the real branches wrote
	// something else.
	cases := []struct {
		outcome string
		result  string
		want    string
	}{
		{"not-configured", "needs a key and a password in asa-pop.json", "asa-pop.json"},
		{string(OutcomeBadCredential), Explain(OutcomeBadCredential), "REFUSED the password"},
		{string(OutcomePortClosed), Explain(OutcomePortClosed), "nothing is listening"},
		{"not-understood", "reply not recognised: gibberish", "not-understood"},
		{"not-reported", "the report was refused: HTTP 401", "HTTP 401"},
	}
	for _, c := range cases {
		t.Run(c.outcome, func(t *testing.T) {
			line := LogLine("2026-09-07 14:03:11", "Ragnarok", c.outcome, nil, c.result)
			if !strings.Contains(line, c.want) {
				t.Fatalf("the failure is not named in the line:\nwant %q\ngot  %s", c.want, line)
			}
			// A COUNT THAT IS NOT KNOWN IS NOT ZERO. "Nobody is on" and "we
			// could not read the answer" are different states everywhere else
			// in this program, and the log is where somebody reads them.
			if strings.Contains(line, "count=0") {
				t.Fatalf("a failure logged count=0, which reads as an empty server:\n%s", line)
			}
			if !strings.Contains(line, "count=-") {
				t.Fatalf("a failure did not log an unknown count:\n%s", line)
			}
		})
	}
}

func TestLogNeverCarriesNamesOrIds(t *testing.T) {
	// THE REAL PATH: a roster goes through `Redact` in `doReport` before it
	// reaches `LogLine`. This drives that composition, because redaction
	// applied somewhere other than the log path would leave this passing while
	// the file filled up with player names.
	roster := "0. SecretName, 0002deadbeefdeadbeefdeadbeefdeadbe\n" +
		"1. AnotherPlayer, 0002cafebabecafebabecafebabecafeb"

	line := LogLine("2026-09-07 14:03:11", "Aberration", "not-understood", nil,
		"reply not recognised: "+Redact(roster))

	for _, leaked := range []string{
		"SecretName", "AnotherPlayer", "deadbeef", "cafebabe",
	} {
		if strings.Contains(line, leaked) {
			t.Fatalf("%q reached the log line:\n%s", leaked, line)
		}
	}
	if !strings.Contains(line, "redacted") {
		t.Fatalf("the roster was dropped rather than redacted - the shape is the diagnostic:\n%s", line)
	}
}

func TestLogLineCannotBeForgedByAReply(t *testing.T) {
	// A SERVER'S REPLY IS NOT OUR TEXT. `Redact` passes non-roster lines
	// through verbatim, which is right for its job; a reply carrying newlines
	// could otherwise write extra lines into this file that look exactly like
	// real entries, and a forged "reported 40" is worse than a missing one.
	hostile := "unknown command\n2026-01-01 00:00:00  Fake  ok  count=40  reported"

	line := LogLine("2026-09-07 14:03:11", "Genesis", "not-understood", nil,
		"reply not recognised: "+Redact(hostile))

	// THE PROPERTY IS "it cannot be a second LINE", and that is the whole of
	// what is defensible. Flattening cannot stop a reply containing the
	// characters "count=40" - and it must not try, because scrubbing arbitrary
	// substrings out of a server's answer would mangle exactly the diagnostic
	// somebody is reading the log to see.
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("a reply put a line break into the log:\n%s", line)
	}

	// SO THE COLUMNS STAY OURS. The stamp, the label, the outcome and the count
	// are written before the reply is, and a reader scanning the left edge is
	// reading fields this program produced. The first "count=" on the line is
	// the real one; anything the server said about a count is downstream of it,
	// visibly inside the result field.
	if !strings.HasPrefix(line, "2026-09-07 14:03:11  Genesis") {
		t.Fatalf("the line does not start with our own fields:\n%s", line)
	}
	first := strings.Index(line, "count=")
	if first < 0 || !strings.HasPrefix(line[first:], "count=-") {
		t.Fatalf("the first count on the line is not the one we wrote:\n%s", line)
	}
	if strings.Index(line, "not-understood") > first {
		t.Fatalf("the outcome column came after the count - the format moved:\n%s", line)
	}
}

func TestLogSafeFlattensAndTruncates(t *testing.T) {
	if got := logSafe("a\nb\tc  d", 100); got != "a b c d" {
		t.Fatalf("whitespace was not flattened: %q", got)
	}
	if got := logSafe("", 10); got != "-" {
		t.Fatalf("an empty field should read as a dash, got %q", got)
	}
	long := strings.Repeat("x", 300)
	if got := logSafe(long, 20); len([]rune(got)) != 21 {
		t.Fatalf("a long field was not truncated to 20 plus an ellipsis: %d runes", len([]rune(got)))
	}
}

func TestAppendWritesAndKeepsAppending(t *testing.T) {
	path := filepath.Join(t.TempDir(), logName)
	for i := 0; i < 3; i++ {
		if err := appendLogLine(path, fmt.Sprintf("line %d", i)); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(body), "\n"); got != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", got, body)
	}
}

func TestLogIsCappedAndSaysItTrimmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), logName)
	line := strings.Repeat("y", 200)
	// Enough to go past the cap with room to spare.
	for written := 0; written < logMaxBytes+(64<<10); written += len(line) + 1 {
		if err := appendLogLine(path, line); err != nil {
			t.Fatal(err)
		}
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() > logMaxBytes {
		t.Fatalf("the log grew past the cap: %d bytes", fi.Size())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// IT SAYS IT CUT. A log that silently begins mid-sentence reads like
	// corruption, and a corrupt-looking log is one nobody trusts.
	if !strings.Contains(string(body), "trimmed") {
		t.Fatalf("the log was trimmed without saying so")
	}
	// AND IT CUT AT A LINE BOUNDARY: every line after the header is whole.
	for _, l := range strings.Split(strings.TrimSpace(string(body)), "\n")[1:] {
		if l != line {
			t.Fatalf("a partial line survived the trim: %q", l)
		}
	}
}

func TestLogIsCreatedPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		// HONEST SKIP. Go maps the mode to little more than the read-only
		// attribute on Windows and the effective permission comes from the
		// directory's ACL, so asserting 0600 there would be asserting
		// something the platform does not do. It is asked for regardless,
		// because it means what it says everywhere else.
		t.Skip("file modes are not enforced this way on Windows")
	}
	path := filepath.Join(t.TempDir(), logName)
	if err := appendLogLine(path, "hello"); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("the log is %v, wanted 0600 - it sits beside a file holding an admin password", fi.Mode().Perm())
	}
}

func TestAFailingLogDoesNotStopAReport(t *testing.T) {
	// A DIRECTORY WHERE THE FILE SHOULD BE: the open fails, every time, for a
	// reason no retry fixes. `appendLogLine` must return the error rather than
	// panic, and `writeLog` - which is what `doReport` calls - must swallow it.
	dir := t.TempDir()
	blocked := filepath.Join(dir, logName)
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := appendLogLine(blocked, "anything"); err == nil {
		t.Fatal("writing into a directory should have failed")
	}

	// And the swallowing half: it complains once and returns, rather than
	// aborting the run that was about to report a count.
	complained := false
	writeLog("a line", &complained) // must not panic
}
