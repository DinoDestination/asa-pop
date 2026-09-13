package main

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// The wire log: every packet in and out, annotated, with the password never in
// it.
//
// WHY IT EXISTS. This client was written against the Valve Source RCON spec and
// ASA's implementation is not identical to it. Four theories about one owner's
// twelve servers were wrong in a row, and the thing nobody could see was the
// bytes. A shape ("3 frames, 412 bytes") says a reply arrived; it says nothing
// about whether the packet we SENT is one a stricter server would accept.
//
// THE CREDENTIAL IS DROPPED AT CAPTURE, NOT AT RENDER, AND THAT DISTINCTION WAS
// PAID FOR. The first version kept whole packets and redacted the auth body in
// the renderer. A mutation removing one `continue` printed the password - and
// the leak test PASSED, because a hexdump wraps every 16 bytes, so a
// 32-character password is never contiguous in either the hex column or the
// ASCII column. Both halves of the check were defeated by line wrapping: a
// credential guard that could only report success.
//
// So a secret packet keeps its twelve-byte HEADER and nothing else. Everything
// diagnostic - the length prefix, the id, the type - lives there; the
// credential is all of the rest, and the rest is never copied. There is now no
// rendering choice that can leak it, which is the only kind of guarantee worth
// making about a password.

// WireEvent is one packet crossing the socket.
type WireEvent struct {
	At  time.Duration // since the connection opened
	Out bool          // true = we sent it
	// For a secret packet this is the HEADER ONLY - see the note above.
	Raw []byte
	// The real body length, kept so the log can say how many bytes were
	// dropped without having kept them.
	BodyLen int
	Secret  bool
}

// Wire collects events for one connection.
type Wire struct {
	start  time.Time
	Events []WireEvent
}

func NewWire() *Wire { return &Wire{start: time.Now()} }

func (w *Wire) add(out bool, raw []byte, secret bool) {
	if w == nil {
		return
	}
	bodyLen := 0
	if len(raw) > 12 {
		bodyLen = len(raw) - 12
	}
	keep := raw
	if secret {
		cut := 12
		if len(raw) < cut {
			cut = len(raw)
		}
		keep = raw[:cut]
	}
	cp := make([]byte, len(keep))
	copy(cp, keep)
	w.Events = append(w.Events, WireEvent{
		At: time.Since(w.start), Out: out, Raw: cp, BodyLen: bodyLen, Secret: secret,
	})
}

func typeName(out bool, typ int32) string {
	switch {
	case out && typ == 3:
		return "SERVERDATA_AUTH"
	case out && typ == 2:
		return "SERVERDATA_EXECCOMMAND"
	case out && typ == 0:
		return "SERVERDATA_RESPONSE_VALUE (client-sent sentinel - NOT in the spec)"
	case !out && typ == 2:
		return "SERVERDATA_AUTH_RESPONSE"
	case !out && typ == 0:
		return "SERVERDATA_RESPONSE_VALUE"
	}
	return fmt.Sprintf("type %d - unknown", typ)
}

func hexdump(b []byte, indent string) string {
	var sb strings.Builder
	for i := 0; i < len(b); i += 16 {
		end := i + 16
		if end > len(b) {
			end = len(b)
		}
		sb.WriteString(fmt.Sprintf("%s%04x  ", indent, i))
		for j := i; j < i+16; j++ {
			if j < end {
				sb.WriteString(fmt.Sprintf("%02x ", b[j]))
			} else {
				sb.WriteString("   ")
			}
		}
		sb.WriteString(" |")
		for j := i; j < end; j++ {
			c := b[j]
			if c < 0x20 || c > 0x7e {
				c = '.'
			}
			sb.WriteByte(c)
		}
		sb.WriteString("|\n")
	}
	return sb.String()
}

// Render writes the annotated log. `redactBodies` passes RECEIVED bodies
// through Redact first, so a roster does not end up in something pasted into a
// support channel. It has no bearing on the credential, which is already gone:
// two different secrets, two different mechanisms, and one flag governing both
// is how the password would end up in a paste.
func (w *Wire) Render(redactBodies bool) string {
	if w == nil || len(w.Events) == 0 {
		return "    (no packets crossed the socket)\n"
	}
	var sb strings.Builder
	for _, e := range w.Events {
		arrow := "<-  RECV"
		if e.Out {
			arrow = "->  SENT"
		}
		total := len(e.Raw)
		if e.Secret {
			total += e.BodyLen
		}
		sb.WriteString(fmt.Sprintf("    %s  %6.1fms  %d bytes\n",
			arrow, float64(e.At.Microseconds())/1000, total))

		// A SHORT READ IS ITSELF THE FINDING, so it is described rather than
		// skipped: fewer than twelve bytes cannot carry a header, and means the
		// peer sent something that is not a frame.
		if len(e.Raw) < 12 {
			sb.WriteString("          (too short to be an RCON header - shown raw)\n")
			sb.WriteString(hexdump(e.Raw, "          "))
			continue
		}
		size := int32(binary.LittleEndian.Uint32(e.Raw[0:]))
		id := int32(binary.LittleEndian.Uint32(e.Raw[4:]))
		typ := int32(binary.LittleEndian.Uint32(e.Raw[8:]))
		body := e.Raw[12:]

		sb.WriteString(fmt.Sprintf("          size=%d  id=0x%x (%d)  type=%d  %s\n",
			size, id, id, typ, typeName(e.Out, typ)))
		// THE SPEC'S OWN ARITHMETIC, CHECKED OUT LOUD. size counts everything
		// after the length prefix: 4 (id) + 4 (type) + body + 1 + 1.
		want := int32(total - 4)
		if size != want {
			sb.WriteString(fmt.Sprintf("          *** size says %d, the packet is %d after the prefix ***\n",
				size, want))
		}
		if !e.Secret && len(body) >= 2 && (body[len(body)-1] != 0 || body[len(body)-2] != 0) {
			sb.WriteString("          *** does not end in two null bytes ***\n")
		}

		if e.Secret {
			// There is nothing left to print; this says how much was dropped.
			sb.WriteString(fmt.Sprintf("          body=<%d bytes redacted - this packet carries the password>\n",
				e.BodyLen))
			continue
		}
		shown := body
		if redactBodies && !e.Out {
			shown = []byte(Redact(string(body)))
		}
		sb.WriteString(hexdump(shown, "          "))
	}
	return sb.String()
}
