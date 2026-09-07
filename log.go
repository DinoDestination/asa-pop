package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// The log.
//
// WHY IT EXISTS: the scheduled task's stdout goes nowhere. `schtasks /TR` runs
// the binary with no redirection, so every diagnostic this program carefully
// writes - "counted 3, but the report was refused", "the reply was not a player
// list we understood" - was written to a stream nobody would ever read. The
// README said failures land "in the log". There was no log.
//
// The only surviving signal was the task's Last Run Result, which is an
// integer, in a GUI, that nobody opens until they already suspect something.
//
// SUCCESS IS LOGGED TOO, AND THAT IS THE POINT. A file that only records
// failures cannot tell "working" from "not running at all" - and "not running
// at all" is the likely failure here, because the scheduled task only runs
// while somebody is logged in (see install.go). The last timestamp is the
// answer to "is this thing still alive", and it only exists if success writes
// one.
const (
	logName = "asa-pop.log"

	// A CAP, NOT A ROTATION SCHEME. One file that trims its own head is
	// something an owner can open in Notepad and paste from; a directory of
	// asa-pop.log.1..9 is a thing they have to understand first. At roughly 90
	// bytes a line and one line per server per five minutes, 256 KB is about
	// three weeks of history for a twelve-map cluster - far more than anybody
	// needs to answer "when did it stop".
	logMaxBytes  = 256 << 10
	logKeepBytes = 128 << 10
)

func logPath() (string, error) {
	// BESIDE THE BINARY, for the reason `configPath` gives: a scheduled task
	// runs from a working directory the owner never chose, so a relative path
	// would put the log somewhere in System32 - or fail - every five minutes.
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), logName), nil
}

// logSafe makes one field safe to put on a line of a log.
//
// THE REPLY FROM AN RCON SERVER IS NOT OUR TEXT. `Redact` removes names and ids
// from roster lines and passes everything else through verbatim, which is
// correct for its job and not sufficient for this one: a server that answered
// with embedded newlines could otherwise write extra lines into this file that
// look exactly like real entries. A forged "reported 40" is a worse outcome
// than a missing one.
//
// So every field is flattened to a single line before it is written, control
// characters become spaces, runs of whitespace collapse, and the result is
// truncated. The owner's own server name goes through the same path - not
// because it is dangerous, but because a name with a line break in it would
// corrupt the format just as effectively by accident.
func logSafe(s string, max int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || unicode.IsControl(r) || r == ' ' {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := b.String()
	if out == "" {
		return "-"
	}
	if len(out) > max {
		return out[:max] + "…"
	}
	return out
}

// LogLine formats one entry. Pure, so the shape is tested without a filesystem.
//
// `count` is a pointer rather than an int because "the server said nobody is
// on" and "we could not read the answer" must not both print 0 - the same
// distinction `CountPlayers` exists to preserve, carried through to the one
// place an owner will actually read it.
func LogLine(stamp, label, outcome string, count *int, result string) string {
	c := "-"
	if count != nil {
		c = fmt.Sprintf("%d", *count)
	}
	return fmt.Sprintf(
		"%s  %-24s  %-14s  count=%-5s  %s",
		stamp,
		logSafe(label, 24),
		logSafe(outcome, 14),
		c,
		logSafe(result, 160),
	)
}

// trimLog drops the head of the file when it grows past the cap.
//
// It cuts at a LINE BOUNDARY and says it cut. A log that silently begins
// mid-sentence reads like corruption, and the first thing somebody does with a
// corrupt-looking log is stop trusting it.
func trimLog(path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(body) <= logKeepBytes {
		return nil
	}
	tail := body[len(body)-logKeepBytes:]
	if i := bytes.IndexByte(tail, '\n'); i >= 0 {
		tail = tail[i+1:]
	}
	header := []byte("--- earlier entries trimmed to keep this file small ---\n")
	return os.WriteFile(path, append(header, tail...), 0o600)
}

// appendLogLine writes one line, trimming the file first if it has grown too
// big. Exported-ish shape (a path argument) so the tests drive it against a
// temp directory rather than against whatever is beside the test binary.
//
// 0600 IS ASKED FOR AND IS NOT ENFORCED EVERYWHERE. On Linux it means what it
// says. On Windows - where this actually runs - Go maps the mode to little more
// than the read-only attribute, and the effective permission comes from the
// ACL the containing directory hands down. The log holds no credential, so this
// is a belt rather than the trousers; `asa-pop.json`, which does hold one, has
// exactly the same limitation and it is worth being straight about that rather
// than implying the mode is a guarantee.
func appendLogLine(path, line string) error {
	if fi, err := os.Stat(path); err == nil && fi.Size() > logMaxBytes {
		if err := trimLog(path); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

// writeLog appends one line and never fails the caller.
//
// A LOG THAT CANNOT BE WRITTEN MUST NOT STOP A REPORT. The count is the
// product; the log is how somebody checks on it. A read-only directory, a
// locked file, a full disk - none of those are reasons to stop telling the site
// how many people are playing.
//
// IT IS NOT SILENT ABOUT IT EITHER, and the distinction matters: the failure
// goes to stdout, which is where it would have gone anyway, and it is said once
// per run rather than once per server so a broken log cannot bury the lines it
// was supposed to be recording.
func writeLog(line string, complained *bool) {
	path, err := logPath()
	if err == nil {
		err = appendLogLine(path, line)
	}
	if err != nil && !*complained {
		*complained = true
		fmt.Printf("%s  (could not write %s: %v - reporting continues)\n", stamp(), logName, err)
	}
}
