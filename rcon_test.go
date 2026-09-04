package main

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// A SYNTHETIC SOURCE RCON SERVER, the control.
//
// A run against a real ASA server that returns a number proves that server. It
// does NOT prove this client, its reassembly or its counter - and a counter
// that silently undercounts is worse than none, because the number renders as a
// fact. So the client is proved end to end against a server whose true player
// count we chose, including the 4096-byte split a real full server crosses.

func startControl(t *testing.T, password string, players int) string {
	t.Helper()

	body := "No Players Connected"
	if players > 0 {
		body = roster(players)
	}

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveControl(conn, password, body)
		}
	}()
	return ln.Addr().String()
}

func serveControl(conn net.Conn, password, body string) {
	defer conn.Close()
	var buf []byte
	authed := false
	read := make([]byte, 4096)
	for {
		n, err := conn.Read(read)
		if n > 0 {
			buf = append(buf, read[:n]...)
			frames, rest, derr := drain(buf)
			if derr != nil {
				return
			}
			buf = rest
			for _, f := range frames {
				switch {
				case f.typ == authType:
					// Valve sends an empty RESPONSE_VALUE before the verdict.
					_, _ = conn.Write(packet(f.id, responseValueType, nil))
					if f.body == password {
						authed = true
						_, _ = conn.Write(packet(f.id, authResponseType, nil))
					} else {
						_, _ = conn.Write(packet(-1, authResponseType, nil))
					}
				case f.typ == execType && authed:
					// SPLIT AT 4096, exactly as Source does.
					b := []byte(body)
					for at := 0; at < len(b); at += chunkSize {
						end := at + chunkSize
						if end > len(b) {
							end = len(b)
						}
						_, _ = conn.Write(packet(f.id, responseValueType, b[at:end]))
					}
				case f.typ == responseValueType && authed:
					// ARK answers a client-sent RESPONSE_VALUE with this.
					_, _ = conn.Write(packet(f.id, responseValueType, []byte(arkSentinel)))
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func hostPort(t *testing.T, addr string) (string, int) {
	t.Helper()
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	for _, r := range p {
		port = port*10 + int(r-'0')
	}
	return h, port
}

func TestListPlayersSmallRoster(t *testing.T) {
	h, p := hostPort(t, startControl(t, "correct-horse", 3))
	text, shape, outcome := ListPlayers(h, p, "correct-horse", 3*time.Second)
	if outcome != OutcomeOK {
		t.Fatalf("outcome = %s", outcome)
	}
	n, ok := CountPlayers(text)
	if !ok || n != 3 {
		t.Fatalf("count = %d %v", n, ok)
	}
	if shape.Split {
		t.Fatal("a 3-player roster should not have crossed the split")
	}
}

func TestListPlayersEmptyServer(t *testing.T) {
	h, p := hostPort(t, startControl(t, "correct-horse", 0))
	text, _, outcome := ListPlayers(h, p, "correct-horse", 3*time.Second)
	if outcome != OutcomeOK {
		t.Fatalf("outcome = %s", outcome)
	}
	n, ok := CountPlayers(text)
	// A FACT, and it must be 0-with-ok rather than not-understood: an empty
	// server is a real observation and the site should show it.
	if !ok || n != 0 {
		t.Fatalf("count = %d %v, want 0 true", n, ok)
	}
}

// THE ONE THAT MATTERS. 90 players is about 5KB, so the control splits it and
// the client must reassemble. Without the sentinel this returns 74 and nothing
// downstream can tell.
func TestListPlayersCrossesTheSplit(t *testing.T) {
	h, p := hostPort(t, startControl(t, "correct-horse", 90))
	text, shape, outcome := ListPlayers(h, p, "correct-horse", 5*time.Second)
	if outcome != OutcomeOK {
		t.Fatalf("outcome = %s", outcome)
	}
	if !shape.Split || shape.Frames < 2 {
		t.Fatalf("the split was not exercised: frames=%d split=%v - this test proves nothing",
			shape.Frames, shape.Split)
	}
	n, ok := CountPlayers(text)
	if !ok || n != 90 {
		t.Fatalf("count = %d %v, want 90 - the reply was not reassembled", n, ok)
	}
}

func TestWrongPasswordIsItsOwnOutcome(t *testing.T) {
	h, p := hostPort(t, startControl(t, "correct-horse", 3))
	_, _, outcome := ListPlayers(h, p, "wrong", 3*time.Second)
	if outcome != OutcomeBadCredential {
		t.Fatalf("outcome = %s, want bad-credential", outcome)
	}
}

func TestClosedPortIsNotACredentialProblem(t *testing.T) {
	// Bind and immediately close, so the port is definitely nobody's.
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h, p := hostPort(t, ln.Addr().String())
	_ = ln.Close()

	_, _, outcome := ListPlayers(h, p, "anything", 2*time.Second)
	if outcome == OutcomeBadCredential {
		t.Fatal("a closed port was reported as a refused password - that costs the owner " +
			"a credential rotation they did not need")
	}
	if outcome != OutcomePortClosed && outcome != OutcomeUnreachable {
		t.Fatalf("outcome = %s", outcome)
	}
}

// A GAME PORT, which accepts TCP and speaks something else entirely. The
// password must NOT have been sent, and the message must say so.
func TestWrongProtocolDoesNotBlameThePassword(t *testing.T) {
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
		// Four bytes that are not a plausible frame length.
		var junk [8]byte
		binary.LittleEndian.PutUint32(junk[0:], 0xDEADBEEF)
		_, _ = conn.Write(junk[:])
		_ = conn.Close()
	}()

	h, p := hostPort(t, ln.Addr().String())
	_, _, outcome := ListPlayers(h, p, "anything", 2*time.Second)
	if outcome != OutcomeWrongProtocol {
		t.Fatalf("outcome = %s, want wrong-protocol", outcome)
	}
	if !strings.Contains(Explain(outcome), "NOT sent") {
		t.Fatal("the wrong-protocol message does not tell the owner their password was not sent")
	}
}

// EVERY OUTCOME HAS ITS OWN WORDS. A generic failure message is what makes an
// owner rotate a password that was never the problem.
func TestEveryOutcomeExplainsItself(t *testing.T) {
	all := []Outcome{
		OutcomeOK, OutcomeBadCredential, OutcomePortClosed,
		OutcomeUnreachable, OutcomeTimeout, OutcomeWrongProtocol, OutcomeOurFault,
	}
	seen := map[string]Outcome{}
	for _, o := range all {
		msg := Explain(o)
		if len(msg) < 30 {
			t.Fatalf("%s has a stub message: %q", o, msg)
		}
		if prev, dup := seen[msg]; dup {
			t.Fatalf("%s and %s share a message - they are different problems with "+
				"different next steps", prev, o)
		}
		seen[msg] = o
	}
	// ONLY ONE OF THEM BLAMES THE PASSWORD.
	blames := 0
	for _, o := range all {
		if strings.Contains(strings.ToLower(Explain(o)), "refused the password") {
			blames++
		}
	}
	if blames != 1 {
		t.Fatalf("%d outcomes blame the password, want exactly 1", blames)
	}
}
