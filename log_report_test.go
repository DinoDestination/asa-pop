package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// THE LOG PATH ITSELF, not a composition of its parts.
//
// log_test.go tests LogLine and Redact side by side. That is necessary and it
// is NOT the guard the redaction needs: deleting Redact( from doReport's
// not-understood branch leaves every one of those tests green while the file
// fills up with player names. A check aimed one object to the left of the thing
// that breaks is this repository's most-repeated defect, so these two run the
// REAL entry point against a REAL RCON server and read the REAL file.

// beside puts a file where configPath and logPath look for one - beside the
// running binary, which under `go test` is the test binary. It restores
// whatever was there first, so a developer who happens to have a real
// asa-pop.json next to their test binary does not lose it.
func beside(t *testing.T, name, content string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(exe), name)

	prior, priorErr := os.ReadFile(path)
	t.Cleanup(func() {
		if priorErr == nil {
			_ = os.WriteFile(path, prior, 0o600)
			return
		}
		_ = os.Remove(path)
	})

	if content == "" {
		_ = os.Remove(path)
		return path
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		// SAID OUT LOUD, never passed quietly. A test that skips silently
		// because it could not set itself up reports success for a guard that
		// did not run.
		t.Skipf("cannot write beside the test binary (%v) - this check needs that", err)
	}
	return path
}

func TestDoReportRedactsWhatItLogs(t *testing.T) {
	// A REPLY THE COUNTER REJECTS AND THE REDACTOR CAN STILL WORK ON: a banner
	// line plus roster lines. CountPlayers refuses a response that is only
	// partly a roster - undercounting by the half it did not recognise is the
	// failure it exists to prevent - so this lands in the not-understood
	// branch, the one branch where server text reaches the log at all.
	const reply = "Command returned unexpectedly\n" +
		"0. SecretName, 0002deadbeefdeadbeefdeadbeefdeadbe\n" +
		"1. AnotherPlayer, 0002cafebabecafebabecafebabecafeb"

	addr := startControlBody(t, "hunter2", reply)
	host, port := hostPort(t, addr)

	cfg := fmt.Sprintf(`{"endpoint":"http://127.0.0.1:1/never-called",
	  "servers":[{"name":"The Island","host":%q,"port":%d,
	  "password":"hunter2","key":"test-key"}]}`, host, port)

	beside(t, logConfigName, cfg)
	logFile := beside(t, logName, "")

	if code := doReport(); code == 0 {
		t.Fatal("doReport succeeded - it was supposed to reach the not-understood branch")
	}

	body, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("doReport wrote no log at all: %v", err)
	}
	got := string(body)

	if !strings.Contains(got, "not-understood") || !strings.Contains(got, "The Island") {
		t.Fatalf("the failure is not in the log:\n%s", got)
	}
	if strings.Contains(got, "hunter2") {
		t.Fatalf("the RCON password reached the log:\n%s", got)
	}

	// THE MUTATION TARGET. Remove Redact( from that branch and these fire.
	for _, leaked := range []string{"SecretName", "AnotherPlayer", "deadbeef", "cafebabe"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("%q reached the log file:\n%s", leaked, got)
		}
	}
	// AND IT WAS REDACTED RATHER THAN DROPPED. The shape of an unrecognised
	// reply is the diagnostic; a branch that logged nothing would also pass
	// every assertion above.
	if !strings.Contains(got, "redacted") {
		t.Fatalf("the roster was not redacted into the log - it was dropped or never written:\n%s", got)
	}
}

func TestDoReportLogsARunThatCounted(t *testing.T) {
	// SUCCESS LEAVES A LINE, which is the half that lets this file tell
	// "working" from "not running at all". The report is pointed at a closed
	// port, so this lands on not-reported - a run that COUNTED and could not
	// deliver. What is asserted is the count: known, 3, and in the file, which
	// a line reading count=- would not be.
	addr := startControl(t, "hunter2", 3)
	host, port := hostPort(t, addr)

	cfg := fmt.Sprintf(`{"endpoint":"http://127.0.0.1:1/refused",
	  "servers":[{"name":"Ragnarok","host":%q,"port":%d,
	  "password":"hunter2","key":"test-key"}]}`, host, port)

	beside(t, logConfigName, cfg)
	logFile := beside(t, logName, "")

	doReport()

	body, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("doReport wrote no log at all: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "count=3") {
		t.Fatalf("the count it read is not in the log:\n%s", got)
	}
	// A LOG IS THE ONE FILE PEOPLE PASTE INTO A SUPPORT THREAD.
	if strings.Contains(got, "test-key") {
		t.Fatalf("the reporting key reached the log:\n%s", got)
	}
	if strings.Contains(got, "hunter2") {
		t.Fatalf("the RCON password reached the log:\n%s", got)
	}
}
