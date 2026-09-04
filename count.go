package main

import (
	"regexp"
	"strings"
)

// THE COUNTER. One definition, used by the reporter and by --probe.
//
// A probe that exercised a different parser from the one the scheduled task
// runs would prove nothing about the scheduled task - which is the mistake this
// project has recorded four times under "a check must exercise the MECHANISM
// the page uses, not a nearby one". So there is exactly one of these.
//
// ASA's `ListPlayers` answers one of two ways:
//
//	"No Players Connected"                        -> 0, and that is a FACT
//	"0. Name, 0002abc...\n1. Name, 0002def...\n"  -> one line per player
//
// ANYTHING ELSE IS ok=false, NEVER 0. "The server said nobody is on" and "we
// could not read the answer" are different states, and collapsing them prints a
// confident zero on a server that might be full. A count we did not understand
// is not reported at all, the last report goes stale, and the site falls back
// to saying so - which is the whole design.

// A roster entry: an index, a name, and an identity after a comma. ASA sends an
// EOS id where ASE sent a Steam64; neither is parsed, because counting lines
// does not require reading who is on them - and a roster is none of our
// business once it has been counted.
var entryRe = regexp.MustCompile(`^\d+\.\s+.+,\s*\S+$`)

var emptyRe = regexp.MustCompile(`(?i)^no players connected\.?$`)

// CountPlayers returns the number of players and whether the response was
// understood. ok=false means "do not report", never "report zero".
func CountPlayers(text string) (n int, ok bool) {
	body := strings.TrimSpace(text)
	if body == "" {
		return 0, false
	}
	if emptyRe.MatchString(body) {
		return 0, true
	}

	var lines []string
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(strings.TrimSuffix(l, "\r"))
		if l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return 0, false
	}

	entries := 0
	for _, l := range lines {
		if entryRe.MatchString(l) {
			entries++
		}
	}
	if entries == 0 {
		return 0, false
	}
	// EVERY line must be an entry. A response that is half a roster and half
	// something we do not recognise is a response we did not understand, and
	// counting the half we did would undercount by exactly the half we did not.
	if entries != len(lines) {
		return 0, false
	}
	return entries, true
}

// Redact replaces the name and identity on every roster line.
//
// The owner runs --probe on their own machine and pastes the output to us. A
// player roster is not part of confirming that a number can be counted, so it
// is removed by DEFAULT and --raw is the deliberate opt-out for reading it
// locally.
func Redact(text string) string {
	out := make([]string, 0, 16)
	for _, l := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(l, "\r"))
		if entryRe.MatchString(trimmed) {
			idx := trimmed[:strings.Index(trimmed, ".")+1]
			out = append(out, idx+" <name redacted>, <id redacted>")
			continue
		}
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}
