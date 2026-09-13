package main

import (
	"encoding/binary"
	"encoding/hex"
	"net"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The wire log exists to be pasted into a support channel, which is why its
// redaction is not optional and why the check below had to be rebuilt.

const wireSentinel = "sentinel-password-never-print-me"

// THE FIRST VERSION OF THIS CHECK COULD NOT FAIL, and a mutation proved it: a
// `continue` was removed so the auth body was hexdumped, the password appeared
// in the output, and the test passed anyway. A hexdump wraps every 16 bytes, so
// a 32-character password is never contiguous in the ASCII column OR in the hex
// run - and both halves of the check were plain substring searches.
//
// So the dump is DECODED back to bytes before searching, which is the only way
// to look for a secret in a hexdump without knowing how it wraps.
var dumpLine = regexp.MustCompile(`(?m)^\s+[0-9a-f]{4}  ((?:[0-9a-f]{2} |   )+) \|`)

func decodeDump(out string) []byte {
	var all []byte
	for _, m := range dumpLine.FindAllStringSubmatch(out, -1) {
		for _, pair := range strings.Fields(m[1]) {
			b, err := hex.DecodeString(pair)
			if err == nil {
				all = append(all, b...)
			}
		}
	}
	return all
}

func TestWireLogNeverContainsThePassword(t *testing.T) {
	addr := startControl(t, wireSentinel, 3)
	h, p := hostPort(t, addr)

	w := NewWire()
	_, _, outcome := ListPlayersWire(h, p, wireSentinel, 4*time.Second, w)
	if outcome != OutcomeOK {
		t.Fatalf("control server did not answer: %s", outcome)
	}

	// THE STRUCTURAL ASSERTION, and the one that cannot be defeated by
	// formatting: the secret packet's BODY was never copied into the event, so
	// no renderer can print what is not there.
	var sawSecret bool
	for _, e := range w.Events {
		if !e.Secret {
			continue
		}
		sawSecret = true
		if len(e.Raw) != 12 {
			t.Fatalf("the auth event kept %d bytes; the header is 12 and the rest is the password",
				len(e.Raw))
		}
		if e.BodyLen == 0 {
			t.Fatal("BodyLen is 0 - the log cannot say how much it dropped")
		}
	}
	if !sawSecret {
		t.Fatal("no packet was marked secret - is the auth write being traced at all?")
	}

	// AND THE RENDERED OUTPUT, decoded rather than scanned as text. `--raw`
	// governs ROSTER redaction and must never govern the credential, so both
	// renderings are checked.
	for _, redactBodies := range []bool{true, false} {
		out := w.Render(redactBodies)
		if strings.Contains(out, wireSentinel) {
			t.Fatalf("the password is in the wire log as text (redactBodies=%v)", redactBodies)
		}
		if strings.Contains(string(decodeDump(out)), wireSentinel) {
			t.Fatalf("the password is in the wire log's HEX (redactBodies=%v)", redactBodies)
		}
		if !strings.Contains(out, "bytes redacted") {
			t.Fatal("the auth packet is not marked redacted")
		}
	}
}

func TestTheLeakCheckCanActuallySeeALeak(t *testing.T) {
	// THE POSITIVE CONTROL. The previous check passed against a real leak, so
	// this one proves the decoder finds a password that IS present - otherwise
	// the test above is once again a check that can only report success.
	var w Wire
	w.start = time.Now()
	pkt := packet(0x5eec, 3, []byte(wireSentinel))
	// Traced as NOT secret, i.e. exactly what a future mis-call would do.
	w.add(true, pkt, false)

	out := w.Render(true)
	if !strings.Contains(string(decodeDump(out)), wireSentinel) {
		t.Fatal("the decoder cannot find a password that is definitely in the dump - " +
			"the leak check above proves nothing")
	}
}

func TestWireLogKeepsWhatMakesItDiagnostic(t *testing.T) {
	// Redacting the body must not cost the header. Size, id, type and the packet
	// names are what a comparison against a working client is made of.
	addr := startControl(t, wireSentinel, 2)
	h, p := hostPort(t, addr)
	w := NewWire()
	if _, _, o := ListPlayersWire(h, p, wireSentinel, 4*time.Second, w); o != OutcomeOK {
		t.Fatalf("outcome = %s", o)
	}
	out := w.Render(true)

	for _, want := range []string{
		"SERVERDATA_AUTH",
		"SERVERDATA_EXECCOMMAND",
		"SERVERDATA_RESPONSE_VALUE",
		"SENT",
		"RECV",
		"size=",
		"id=0x",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the wire log does not mention %q", want)
		}
	}
	// THE SENTINEL IS FLAGGED AS OFF-SPEC, because it is the most likely thing a
	// stricter server disagrees with us about and the reader should not have to
	// already know that.
	if !strings.Contains(out, "NOT in the spec") {
		t.Error("the client-sent RESPONSE_VALUE is not called out as off-spec")
	}
	// The auth packet's own length must still be reported in full, or the
	// redaction has cost the one number worth comparing.
	if !strings.Contains(out, "bytes redacted") {
		t.Error("the auth packet does not say how many bytes it dropped")
	}
}

func TestWireLogFlagsAMalformedLengthPrefix(t *testing.T) {
	// A server whose frames disagree with the spec's arithmetic is exactly what
	// this is for, so the log has to SAY so rather than leaving somebody to
	// count hex by hand.
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 512)
		_, _ = conn.Read(buf)
		ok := make([]byte, 14)
		binary.LittleEndian.PutUint32(ok[0:], 10)
		binary.LittleEndian.PutUint32(ok[4:], 0x5eec)
		binary.LittleEndian.PutUint32(ok[8:], 2)
		_, _ = conn.Write(ok)
		time.Sleep(50 * time.Millisecond)
		bad := make([]byte, 16)
		binary.LittleEndian.PutUint32(bad[0:], 99) // says 99, is 12
		binary.LittleEndian.PutUint32(bad[4:], 0x5eed)
		binary.LittleEndian.PutUint32(bad[8:], 0)
		_, _ = conn.Write(bad)
	}()

	h, p := hostPort(t, ln.Addr().String())
	w := NewWire()
	ListPlayersWire(h, p, "anything", 1500*time.Millisecond, w)
	out := w.Render(true)
	if !strings.Contains(out, "size says") {
		t.Errorf("a lying length prefix was not flagged:\n%s", out)
	}
}
