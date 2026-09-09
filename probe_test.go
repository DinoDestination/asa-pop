package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// WHAT `--probe` ACTUALLY PRINTS.
//
// `Redact` had unit tests and `doProbe`'s USE of it had none, which is this
// repository's fourth rule word for word: a check must exercise the mechanism
// the page uses, not a nearby one. Deleting `shown = Redact(text)` from
// `doProbe` left all 34 tests green - on the one output an owner is told is
// safe to paste to a stranger.
//
// So these drive `doProbe` itself and read its stdout. Not `Redact`, not a
// helper that resembles it, and not the source: the bytes the owner copies out
// of his console are the only thing that answers the question.

// captureStdout runs fn with os.Stdout redirected and returns what it wrote.
//
// The reader runs CONCURRENTLY on purpose. A pipe holds about 64 KB before it
// blocks, a full roster probe prints less than that today, and a test that
// deadlocks the day the output grows is a worse failure than the one it is
// checking for.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()

	func() {
		defer func() {
			os.Stdout = old
			_ = w.Close()
		}()
		fn()
	}()

	out := <-done
	_ = r.Close()
	return out
}

// configBeside writes asa-pop.json where `configPath` looks for it: next to the
// running executable, which under `go test` is the test binary.
//
// THE REAL LOOKUP, NOT AN INJECTED PATH. `configPath` resolving beside the
// BINARY rather than the working directory is a deliberate decision - a
// scheduled task runs from C:\Windows\System32 - so a test that handed
// `doProbe` a path of its own choosing would be testing a lookup that does not
// exist.
func configBeside(t *testing.T, body string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(exe), logConfigName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

// noConfigBeside guarantees the state a first-time owner is in.
func noConfigBeside(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(exe), logConfigName)
	_ = os.Remove(path)
	return path
}

// ---------------------------------------------------------------------------
// no config
// ---------------------------------------------------------------------------

func TestProbeWithNoConfigExplainsItselfRatherThanErroring(t *testing.T) {
	// THE STATE EVERY FIRST RUN IS IN. `ChooseMode` gives an explicit `--probe`
	// priority over everything, so a technical owner who types it before there
	// is any config gets here - and what he sees is the whole of what he has to
	// go on.
	path := noConfigBeside(t)

	var code int
	out := captureStdout(t, func() { code = doProbe(false) })

	if code != 1 {
		t.Fatalf("exit code = %d, want 1 - a missing config is not a success", code)
	}

	// IT STILL SAYS WHAT A PROBE IS. He is being asked to run an unsigned
	// binary against his own game server; "nothing is reported to anybody" has
	// to survive the error path, because the error path is where a first run
	// lands.
	for _, want := range []string{
		"PROBE",
		"nothing is reported to anybody",
		"ListPlayers",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the no-config path dropped the reassurance %q\n%s", want, out)
		}
	}

	// AND IT SAYS WHAT TO DO, which is the difference between an explanation and
	// an error. The path it could not read, the instruction, and a config he can
	// paste - not a Go error string on its own.
	if !strings.Contains(out, filepath.Base(path)) {
		t.Errorf("it does not name the file it wanted (%s)\n%s", path, out)
	}
	for _, want := range []string{
		"Could not read",
		"Create it beside this program",
		`"servers"`,
		"27020",
		"ServerAdminPassword",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the no-config output is missing %q\n%s", want, out)
		}
	}

	// NOT A PANIC AND NOT A BARE STACK. Either would send an owner to us with
	// "it crashed", which is a different conversation from the one this output
	// is meant to start.
	for _, never := range []string{"panic:", "goroutine ", "runtime error"} {
		if strings.Contains(out, never) {
			t.Errorf("the no-config path leaked %q at the owner\n%s", never, out)
		}
	}
}

func TestProbeWithAnEmptyServerListSaysSo(t *testing.T) {
	// A config that parses and holds nothing is the half-finished state - an
	// owner who created the file from the sample and has not filled it in. It
	// must not read as "your server did not answer".
	configBeside(t, `{"servers":[]}`)

	var code int
	out := captureStdout(t, func() { code = doProbe(false) })

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "no servers in it") {
		t.Errorf("an empty server list is not explained\n%s", out)
	}
	if !strings.Contains(out, `"servers"`) {
		t.Errorf("it does not show what a filled-in config looks like\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// redaction, on the output the owner pastes
// ---------------------------------------------------------------------------

func TestProbeRedactsNamesAndIdsInWhatItPrints(t *testing.T) {
	const password = "correct-horse"
	addr := startControl(t, password, 3)
	h, p := hostPort(t, addr)
	configBeside(t, fmt.Sprintf(
		`{"servers":[{"name":"The Island","host":%q,"port":%d,"password":%q,"key":"k"}]}`,
		h, p, password))

	var code int
	out := captureStdout(t, func() { code = doProbe(false) })

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 - the control answers correctly\n%s", code, out)
	}

	// THE COUNT IS STILL RIGHT. Redaction that also broke the counter would pass
	// every assertion below and be useless.
	if !strings.Contains(out, "count   : 3") {
		t.Errorf("the probe did not count the control's 3 players\n%s", out)
	}

	// NOT ONE NAME AND NOT ONE ID. `line(i)` builds `0. Survivor000, 0002<hex>`,
	// so both halves of a roster line are checked - a redactor that blanked the
	// name and printed the id would satisfy a looser test.
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("Survivor%03d", i)
		if strings.Contains(out, name) {
			t.Errorf("a player NAME reached the pasteable output: %s\n%s", name, out)
		}
	}
	if strings.Contains(out, "0002a1b2c3d4") {
		t.Errorf("a player ID reached the pasteable output\n%s", out)
	}
	if !strings.Contains(out, "<name redacted>") || !strings.Contains(out, "<id redacted>") {
		t.Errorf("the roster lines were not replaced with the redacted form\n%s", out)
	}

	// AND IT SAYS SO, because the sentence is what makes an owner willing to
	// paste it. A redacted output that did not claim to be redacted would get
	// retyped by hand or not sent at all.
	if !strings.Contains(out, "safe to paste") {
		t.Errorf("the probe does not tell the owner the output is safe to paste\n%s", out)
	}
}

func TestProbeRawShowsWhatRedactionHides(t *testing.T) {
	// THE POSITIVE CONTROL, and without it the test above is worthless: three
	// assertions that a name is ABSENT all pass on a probe that printed nothing
	// at all, or on one whose control server never answered.
	//
	// It is also the flag's contract. `--raw` exists so an owner can see the
	// shape ASA actually returned when the counter does not recognise it, and a
	// `--raw` that redacted anyway would make that impossible to diagnose.
	const password = "correct-horse"
	h, p := hostPort(t, startControl(t, password, 3))
	configBeside(t, fmt.Sprintf(
		`{"servers":[{"host":%q,"port":%d,"password":%q,"key":"k"}]}`, h, p, password))

	out := captureStdout(t, func() { _ = doProbe(true) })

	if !strings.Contains(out, "Survivor000") {
		t.Fatalf("--raw did not show a name, so the redaction test proves nothing\n%s", out)
	}
	if strings.Contains(out, "<name redacted>") {
		t.Errorf("--raw redacted anyway, which is what it exists not to do\n%s", out)
	}
	// AND IT DOES NOT CLAIM TO BE SAFE. The "safe to paste" line is printed only
	// under redaction; saying it here would be the exact wrong sentence beside
	// the exact wrong output.
	if strings.Contains(out, "safe to paste") {
		t.Errorf("--raw output calls itself safe to paste\n%s", out)
	}
}

func TestProbeSaysWhatIsWrongWithoutBlamingThePassword(t *testing.T) {
	// A probe against a port nothing is listening on. The outcome must not be
	// the credential one: telling an owner his password is wrong when the port
	// is closed costs him a rotation he did not need.
	configBeside(t, `{"servers":[{"host":"127.0.0.1","port":1,"password":"x","key":"k"}]}`)

	var code int
	out := captureStdout(t, func() { code = doProbe(false) })

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(out, "outcome :") {
		t.Errorf("no outcome was reported\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "password was refused") {
		t.Errorf("a closed port was reported as a credential problem\n%s", out)
	}
	if !strings.Contains(out, "Nothing was reported") {
		t.Errorf("a failed probe does not say that nothing was reported\n%s", out)
	}
}
