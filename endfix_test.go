package main

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// A server that authenticates, answers the command, and IGNORES the end marker.
//
// This is the shape the whole experiment is for, and it did not exist in the
// suite: every control here answers the sentinel, so the code path where it
// never comes back had never been executed by a test. The bug survived because
// nothing modelled the server that has it.
func startDeafControl(t *testing.T, password, body string) string {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	frame := func(id, typ int32, s string) []byte {
		b := make([]byte, 12+len(s)+2)
		binary.LittleEndian.PutUint32(b[0:], uint32(10+len(s)))
		binary.LittleEndian.PutUint32(b[4:], uint32(id))
		binary.LittleEndian.PutUint32(b[8:], uint32(typ))
		copy(b[12:], s)
		return b
	}

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 4096)

		// AUTH
		n, err := conn.Read(buf)
		if err != nil || n < 12 {
			return
		}
		got := string(buf[12 : n-2])
		if got != password {
			_, _ = conn.Write(frame(-1, 2, ""))
			return
		}
		_, _ = conn.Write(frame(0x5eec, 2, ""))

		// Everything after auth: reply to an EXECCOMMAND and to nothing else.
		// A RESPONSE_VALUE from the client is read and dropped on the floor,
		// which is exactly what we think ASA is doing.
		for {
			n, err := conn.Read(buf)
			if err != nil || n < 12 {
				return
			}
			typ := int32(binary.LittleEndian.Uint32(buf[8:]))
			if typ == 2 {
				_, _ = conn.Write(frame(0x5eed, 0, body))
			}
			// typ == 0 (our sentinel): deliberately no answer.
		}
	}()
	return ln.Addr().String()
}

func TestDefaultHangsOnAServerThatIgnoresTheMarker(t *testing.T) {
	// THE BUG, PINNED. Without the experiment the roster arrives, the marker
	// never does, and we return NOTHING after burning the whole deadline - the
	// eight seconds a twelve-server owner saw twelve times over.
	addr := startDeafControl(t, "pw", "No Players Connected")
	h, p := hostPort(t, addr)

	start := time.Now()
	text, shape, outcome := ListPlayersOpts(h, p, "pw", 1200*time.Millisecond, Opts{})
	if outcome != OutcomeTimeout {
		t.Fatalf("outcome = %s, want timeout - the default is supposed to still be broken here", outcome)
	}
	if text != "" {
		t.Fatal("the default returned text; this test is asserting the OLD behaviour")
	}
	// The evidence is in the shape even though the text is thrown away - which
	// is what --probe now prints on a failure.
	if shape.Frames == 0 {
		t.Fatal("frames = 0: the server's reply was not even counted, so --probe " +
			"still cannot tell this apart from a dead port")
	}
	if time.Since(start) < 900*time.Millisecond {
		t.Fatal("it did not actually wait out the deadline")
	}
}

func TestEndFixReturnsTheRosterWhenTheMarkerNeverComes(t *testing.T) {
	addr := startDeafControl(t, "pw", roster(4))
	h, p := hostPort(t, addr)

	start := time.Now()
	text, shape, outcome := ListPlayersOpts(h, p, "pw", 8*time.Second, Opts{
		SeparateSentinel: true,
		EndOnQuiet:       true,
		Quiet:            250 * time.Millisecond,
	})
	if outcome != OutcomeNoEndMarker {
		t.Fatalf("outcome = %s, want no-end-marker", outcome)
	}
	n, ok := CountPlayers(text)
	if !ok || n != 4 {
		t.Fatalf("count = %d ok=%v, want 4 - the roster did not survive", n, ok)
	}
	if shape.Frames != 1 {
		t.Errorf("frames = %d, want 1", shape.Frames)
	}
	// IT RETURNS ON THE QUIET PERIOD, NOT THE DEADLINE. Waiting out the full 8s
	// would leave a twelve-server run at 96 seconds, which is the cost that
	// started this.
	if time.Since(start) > 3*time.Second {
		t.Fatalf("took %v - it waited for the deadline rather than the quiet period",
			time.Since(start))
	}
}

func TestEndFixNeverGuessesOnAnEmptyReply(t *testing.T) {
	// THE ONE THING THE FALLBACK MUST NOT DO. A server that accepts the
	// connection and says nothing at all is a timeout, not an empty roster -
	// reporting "0 players" there would publish a wrong number as though it
	// were measured.
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
		time.Sleep(3 * time.Second)
	}()

	h, p := hostPort(t, ln.Addr().String())
	text, _, outcome := ListPlayersOpts(h, p, "pw", 900*time.Millisecond, Opts{
		SeparateSentinel: true, EndOnQuiet: true, Quiet: 200 * time.Millisecond,
	})
	if outcome != OutcomeTimeout {
		t.Fatalf("outcome = %s, want timeout", outcome)
	}
	if text != "" {
		t.Fatalf("returned %q on a server that said nothing", text)
	}
}

// recvBetweenExecAndMarker reports whether anything came back BETWEEN the
// command and the end marker.
//
// THE FIRST VERSION OF THESE TWO TESTS ASKED "was the marker sent before any
// RECV", which can never be true: authentication requires a reply before the
// command is sent at all, so the default failed its own test. Adjacency is the
// property that actually distinguishes the two modes.
func recvBetweenExecAndMarker(t *testing.T, w *Wire) bool {
	t.Helper()
	execAt, markerAt := -1, -1
	for i, e := range w.Events {
		if !e.Out || len(e.Raw) < 12 {
			continue
		}
		typ := int32(binary.LittleEndian.Uint32(e.Raw[8:]))
		if typ == 2 && execAt < 0 {
			execAt = i
		}
		if typ == 0 && markerAt < 0 {
			markerAt = i
		}
	}
	if execAt < 0 || markerAt < 0 {
		t.Fatalf("did not see both the command and the marker (exec=%d marker=%d)", execAt, markerAt)
	}
	for _, e := range w.Events[execAt+1 : markerAt] {
		if !e.Out {
			return true
		}
	}
	return false
}

func TestSeparateSentinelIsSentAfterTheReply(t *testing.T) {
	// THE MECHANISM, not the fallback: a response frame must arrive between the
	// command and the marker, so the two can never reach the server glued
	// together in one segment.
	addr := startControl(t, "pw", 2)
	h, p := hostPort(t, addr)
	w := NewWire()
	_, _, outcome := ListPlayersOpts(h, p, "pw", 4*time.Second, Opts{
		Wire: w, SeparateSentinel: true, EndOnQuiet: true, Quiet: 400 * time.Millisecond,
	})
	if outcome != OutcomeOK && outcome != OutcomeNoEndMarker {
		t.Fatalf("outcome = %s", outcome)
	}
	if !recvBetweenExecAndMarker(t, w) {
		t.Fatal("nothing came back between the command and the marker - the round trip is not separate")
	}
}

func TestTheDefaultStillSendsBothTogether(t *testing.T) {
	// BOTH DIRECTIONS. Asserting only the new behaviour passes on a build where
	// the flag does nothing because it is always on.
	addr := startControl(t, "pw", 1)
	h, p := hostPort(t, addr)
	w := NewWire()
	if _, _, o := ListPlayersOpts(h, p, "pw", 4*time.Second, Opts{Wire: w}); o != OutcomeOK {
		t.Fatalf("outcome = %s", o)
	}
	if recvBetweenExecAndMarker(t, w) {
		t.Fatal("the default waited for a reply before sending the marker - " +
			"--end-fix has become the default without anybody deciding to")
	}
}

func TestNoEndMarkerExplainsItselfAndBlamesNobody(t *testing.T) {
	msg := Explain(OutcomeNoEndMarker)
	if len(msg) < 30 {
		t.Fatal("stub message")
	}
	if strings.Contains(strings.ToLower(msg), "refused the password") {
		t.Fatal("it blames the credential")
	}
	// It must say the number is not confirmed, or an operator reads it as a
	// clean success with an odd name.
	if !strings.Contains(msg, "unconfirmed") {
		t.Errorf("does not say the count is unconfirmed: %q", msg)
	}
}
