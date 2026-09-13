package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Source RCON, far enough to count players and no further.
//
// ONE AUTH AND ONE READ-ONLY COMMAND. `ListPlayers` returns who is online and
// changes nothing. Nothing here can write, kick, ban, save or shut down, and
// there is no parameter through which a caller could ask it to - the command is
// a constant in this file.
//
// THE PASSWORD IS A []byte FOR ITS WHOLE LIFE HERE and is zeroed in a defer.
// What that buys is that the packet we built and the memory we control do not
// keep it after the call. What it does not buy is control over the string the
// JSON decoder made when it read the config file; Go strings are immutable and
// garbage-collected, so that copy cannot be wiped. Saying otherwise would be
// the kind of security theatre that stops people looking further.
//
// IT NEVER LEAVES THIS MACHINE. The password is read from a file next to the
// binary, used against 127.0.0.1, and is not part of anything sent to the
// endpoint - which carries a key and an integer and has no field that could
// hold it.

const (
	authType          = 3
	execType          = 2
	authResponseType  = 2
	responseValueType = 0
)

// Read-only, and a constant so no config value can choose it.
const harmlessCommand = "ListPlayers"

// ARK's reply to a RESPONSE_VALUE sent by a client. Our end-of-response marker.
const arkSentinel = "Server received, But no response!!"

// Source caps a response frame's body at 4096 bytes.
//
// THIS IS NOT THEORETICAL. A ListPlayers line is roughly 55 bytes, so the split
// lands near 74 players and ASA servers routinely carry 70-127. A client that
// returned on the first frame would undercount a full server and never say so -
// and worse, a truncation that happens to land on a newline parses CLEANLY, so
// the counter's own sanity check cannot catch it. Reassembly is load-bearing.
const chunkSize = 4096

// Outcome is every way a probe can end, each with its own words.
//
// A generic "verification failed" is worse here than anywhere: the owner has
// just put an admin password in a file. If they cannot tell "wrong password"
// from "port closed", the safe assumption is that the password leaked and they
// rotate it for no reason - a real cost imposed by being vague.
type Outcome string

const (
	OutcomeOK            Outcome = "ok"
	OutcomeBadCredential Outcome = "bad-credential"
	OutcomePortClosed    Outcome = "port-closed"
	OutcomeUnreachable   Outcome = "unreachable"
	OutcomeTimeout       Outcome = "timeout"
	OutcomeWrongProtocol Outcome = "wrong-protocol"
	OutcomeOurFault      Outcome = "our-fault"
	// The server answered in full and never sent an end marker. Only reachable
	// with --end-fix, and DELIBERATELY NOT OutcomeOK: we are returning a reply
	// we believe is complete without the proof we normally require, and the one
	// thing this counter may never do is report a number it cannot stand behind
	// as though it were certain.
	OutcomeNoEndMarker Outcome = "no-end-marker"
)

// Explain returns the sentence an owner reads in a log they will open exactly
// once - when it is not working.
func Explain(o Outcome) string {
	switch o {
	case OutcomeOK:
		return "connected, authenticated and read the player list"
	case OutcomeBadCredential:
		return "the server answered and REFUSED the password. The host and port are right. " +
			"Check ServerAdminPassword in GameUserSettings.ini."
	case OutcomePortClosed:
		// THE ENABLE HINT BELONGS HERE, NOT ONLY IN THE GUIDED SUMMARY. That
		// summary prints the RCONEnabled line only when ZERO servers answered, so
		// an owner running two maps with RCON off on one of them got this message
		// with no hint at all - and "check RCONPort" sends them to look at a
		// setting that is usually already correct. The common cause is that RCON
		// was never switched on, or was switched on without restarting the server:
		// ARK reads this file at boot and rewrites it on shutdown, so an edit made
		// while the server is up is discarded.
		//
		// "Nothing received the password" IS TRUE HERE, which is the whole reason
		// it is said here and not below: DialTimeout failed, so no byte was
		// written. See the wrong-protocol case for the same sentence being false.
		return "nothing is listening on that port. Nothing received the password. " +
			"Check that RCONEnabled=True is under [ServerSettings] in GameUserSettings.ini, " +
			"that RCONPort matches (usually 27020), and that the server was RESTARTED after " +
			"the edit - ARK rewrites that file on shutdown, so an edit made while it is " +
			"running is thrown away. It is not the port players connect on."
	case OutcomeUnreachable:
		return "could not reach that address at all. If the server is on this machine, use 127.0.0.1."
	case OutcomeTimeout:
		return "something is listening but did not finish the exchange. A firewall that drops " +
			"rather than refuses looks exactly like this."
	case OutcomeNoEndMarker:
		return "the server answered and never sent an end-of-response marker, so the reply " +
			"was taken as complete when the socket went quiet. The count is probably right; " +
			"it is reported as unconfirmed because a reply cut mid-list would still parse."
	case OutcomeWrongProtocol:
		// THIS SAID "Your password was NOT sent anywhere" AND THAT WAS FALSE.
		//
		// `ListPlayers` dials and then writes the auth packet immediately: Source
		// RCON is client-initiates, so there is no greeting to inspect and nothing
		// to validate before speaking. By the time anything can be recognised as
		// not-RCON, the password has already gone to whatever is listening. The
		// message claimed the opposite in the ONE case where it mattered, which is
		// worse than saying nothing - an owner who reads it does not rotate a
		// credential they should.
		//
		// THE ALTERNATIVE WAS REJECTED ON MEASUREMENT, not on preference. Probing
		// first - one non-auth packet, read the reply, only then authenticate -
		// would make the old sentence true. It also depends on how an
		// unauthenticated ARK server answers a pre-auth RESPONSE_VALUE, which is
		// unspecified; if it ignores it we block to the deadline and report
		// `timeout` on a HEALTHY server. That trades a false sentence in a rare
		// case for a broken install in the common one.
		//
		// So it says what happened, and what it costs. Usually nothing: the port
		// was the owner's own game or query port, on their own machine, and the
		// bytes went to their own server process. The case worth acting on is a
		// port that was not theirs, and only they can tell those apart.
		return "something answered on that port, but not the way RCON does - usually the game " +
			"port or the query port by mistake. Use RCONPort (usually 27020). The password WAS " +
			"sent to whatever answered before it could be recognised: if that was your own " +
			"server on this machine it went no further, but if the port was not yours, change " +
			"ServerAdminPassword."
	default:
		return "the connection failed in a way we did not expect. This one is on us, not your password."
	}
}

type frame struct {
	id   int32
	typ  int32
	body string
}

func packet(id, typ int32, body []byte) []byte {
	buf := make([]byte, 0, 14+len(body))
	var hdr [12]byte
	binary.LittleEndian.PutUint32(hdr[0:], uint32(8+len(body)+2))
	binary.LittleEndian.PutUint32(hdr[4:], uint32(id))
	binary.LittleEndian.PutUint32(hdr[8:], uint32(typ))
	buf = append(buf, hdr[:]...)
	buf = append(buf, body...)
	buf = append(buf, 0, 0)
	return buf
}

var errNotRCON = errors.New("not an RCON server")

// drain pulls whole frames out of a stream, tolerating both split and coalesced
// packets.
func drain(buf []byte) ([]frame, []byte, error) {
	var frames []frame
	at := 0
	for len(buf)-at >= 4 {
		size := int32(binary.LittleEndian.Uint32(buf[at:]))
		// A LENGTH THAT MAKES NO SENSE IS A DIFFERENT PROTOCOL, not a short
		// read. The game port answers with something whose first four bytes are
		// not a plausible frame length, and waiting for 2GB of it is how this
		// would hang until the deadline instead of saying "wrong protocol".
		if size < 10 || size > 8192 {
			return nil, nil, errNotRCON
		}
		if int32(len(buf)-at-4) < size {
			break
		}
		id := int32(binary.LittleEndian.Uint32(buf[at+4:]))
		typ := int32(binary.LittleEndian.Uint32(buf[at+8:]))
		body := string(buf[at+12 : at+4+int(size)-2])
		frames = append(frames, frame{id: id, typ: typ, body: body})
		at += 4 + int(size)
	}
	return frames, bytes.Clone(buf[at:]), nil
}

// Shape is what --probe reports: structure, never content.
type Shape struct {
	Frames int
	Bytes  int
	Split  bool
}

// Opts is how a run differs from the default, so the next experiment adds a
// field rather than a positional parameter - `doProbe` grew one of those and
// broke five call sites that vet had to find.
type Opts struct {
	Wire *Wire
	// SeparateSentinel sends the end marker only AFTER the first response frame
	// has arrived, instead of immediately behind the command.
	//
	// THE DEFAULT WRITES BOTH BACK TO BACK WITH NoDelay, so they routinely reach
	// the server coalesced in one TCP segment - and a server that reads one
	// packet per recv handles the command, replies, and never sees the marker.
	// A full round trip between the two makes that impossible. It costs one RTT
	// against a loopback address, which is microseconds.
	SeparateSentinel bool
	// EndOnQuiet returns what has arrived if the marker never comes and the
	// socket then stays silent for Quiet.
	//
	// THE BACKSTOP, NOT THE MECHANISM, and the distinction matters. A pause
	// between frames of a split reply is indistinguishable from the end of one,
	// so this is the only design here that can undercount - and a truncation
	// landing on a newline parses cleanly, which is why the counter cannot spot
	// it afterwards. It returns OutcomeNoEndMarker so the number is never passed
	// off as confirmed.
	EndOnQuiet bool
	Quiet      time.Duration
}

// ListPlayers authenticates, runs one read-only command, and returns the WHOLE
// response.
func ListPlayers(host string, port int, password string, timeout time.Duration) (string, Shape, Outcome) {
	return ListPlayersOpts(host, port, password, timeout, Opts{})
}

// ListPlayersWire is ListPlayers with a wire log and nothing else changed.
func ListPlayersWire(
	host string, port int, password string, timeout time.Duration, w *Wire,
) (string, Shape, Outcome) {
	return ListPlayersOpts(host, port, password, timeout, Opts{Wire: w})
}

// ListPlayersOpts is the real one. Everything above is a thin wrapper so a new
// experiment never rewrites existing call sites.
func ListPlayersOpts(
	host string, port int, password string, timeout time.Duration, o Opts,
) (string, Shape, Outcome) {
	w := o.Wire
	// ONE COPY, AND WE OWN IT.
	secret := []byte(password)
	defer func() {
		for i := range secret {
			secret[i] = 0
		}
	}()

	var shape Shape
	addr := net.JoinHostPort(host, fmt.Sprint(port))

	conn, err := net.DialTimeout("tcp4", addr, timeout)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return "", shape, OutcomeTimeout
		}
		if strings.Contains(err.Error(), "refused") {
			return "", shape, OutcomePortClosed
		}
		return "", shape, OutcomeUnreachable
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	_ = conn.SetDeadline(deadline)
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
	}

	const authID, execID, sentinelID = 0x5eec, 0x5eed, 0x5eee

	authPkt := packet(authID, authType, secret)
	// `true` - THIS PACKET CARRIES THE CREDENTIAL. The tracer renders a length
	// and never the bytes, so there is no path from here to a printed password.
	w.add(true, authPkt, true)
	if _, err := conn.Write(authPkt); err != nil {
		return "", shape, OutcomeOurFault
	}

	var buf []byte
	var parts []string
	authed := false
	sentinelSent := false
	read := make([]byte, 8192)

	for {
		n, err := conn.Read(read)
		if n > 0 {
			// THE RAW READ, not the frames drained from it. Whether two packets
			// arrived coalesced in one segment, or one arrived split across
			// two, is exactly the kind of thing a stricter server disagrees
			// with us about - and draining first would hide it.
			w.add(false, read[:n], false)
			buf = append(buf, read[:n]...)
			var frames []frame
			frames, buf, err = drain(buf)
			if err != nil {
				return "", shape, OutcomeWrongProtocol
			}

			for _, f := range frames {
				if !authed {
					// The empty RESPONSE_VALUE Valve sends before the verdict.
					if f.typ == responseValueType && f.id != -1 {
						continue
					}
					if f.typ != authResponseType {
						continue
					}
					// -1 IS THE REFUSAL, and the only thing that means the
					// password was wrong. Everything else has its own outcome
					// precisely so this one is unambiguous.
					if f.id == -1 {
						return "", shape, OutcomeBadCredential
					}
					authed = true
					execPkt := packet(execID, execType, []byte(harmlessCommand))
					w.add(true, execPkt, false)
					if _, err := conn.Write(execPkt); err != nil {
						return "", shape, OutcomeOurFault
					}
					// THE SENTINEL, SENT IMMEDIATELY AFTER. Source has no "this
					// is the last frame" bit, so the only reliable end marker is
					// a packet we sent ourselves coming back after the real ones.
					// HELD BACK until a response frame proves the server has
					// finished reading the command, when asked to. Sending it
					// now is what may be getting it swallowed.
					if !o.SeparateSentinel {
						if !writeSentinel(conn, w, sentinelID) {
							return "", shape, OutcomeOurFault
						}
						sentinelSent = true
					}
					continue
				}

				if f.typ != responseValueType {
					continue
				}
				// END OF RESPONSE: our sentinel's id, or ARK's reply to it.
				if f.id == sentinelID || strings.HasPrefix(f.body, arkSentinel) {
					shape.Split = shape.Frames > 1
					return strings.Join(parts, ""), shape, OutcomeOK
				}
				shape.Frames++
				shape.Bytes += len(f.body)
				parts = append(parts, f.body)

				// THE SEPARATE ROUND TRIP. The first response frame is proof the
				// server consumed the command as its own packet, so the marker
				// cannot now arrive glued to it.
				if o.SeparateSentinel && !sentinelSent {
					if !writeSentinel(conn, w, sentinelID) {
						return "", shape, OutcomeOurFault
					}
					sentinelSent = true
				}
				// A SHORT LEASH ONCE THERE IS SOMETHING TO LOSE. Without this the
				// fallback would wait out the whole 8s deadline before deciding
				// the marker is not coming, which on twelve servers is the 96
				// seconds that started all of this.
				if o.EndOnQuiet && o.Quiet > 0 {
					_ = conn.SetDeadline(time.Now().Add(o.Quiet))
				}
			}
		}

		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				shape.Split = shape.Frames > 1
				// NEVER ON AN EMPTY READ. With nothing in hand this is the
				// timeout it always was; the fallback exists for the case where
				// the roster arrived and only the marker did not.
				if o.EndOnQuiet && len(parts) > 0 {
					return strings.Join(parts, ""), shape, OutcomeNoEndMarker
				}
				return "", shape, OutcomeTimeout
			}
			// CLOSED WITHOUT AN ANSWER. A game port typically accepts the TCP
			// connection and then drops it, which is not a refusal of the
			// credential.
			if authed {
				return "", shape, OutcomeOurFault
			}
			return "", shape, OutcomeWrongProtocol
		}
	}
}

// writeSentinel builds and sends the end marker. One definition, because the
// two places that send it differ only in WHEN - and a second hand-written
// packet is how they would come to differ in what.
func writeSentinel(conn net.Conn, w *Wire, sentinelID int32) bool {
	pkt := packet(sentinelID, responseValueType, nil)
	w.add(true, pkt, false)
	_, err := conn.Write(pkt)
	return err == nil
}
