package main

import (
	"strconv"
	"strings"
)

// The pure half of the guided prompts, so the parsing is tested without a
// console. The platform-specific half is only the raw echo toggle.

// ParsePort reads a port answer, falling back to the default on an empty line.
//
// A BAD ANSWER IS REJECTED, NEVER COERCED. `strconv.Atoi` on "27020 " or
// "port 27020" would fail and a silent fallback to 27020 would then probe the
// wrong port and report "nothing is listening" - sending somebody to check a
// firewall because they typed a stray word.
func ParsePort(answer string, def int) (int, bool) {
	s := strings.TrimSpace(answer)
	if s == "" {
		return def, true
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, false
	}
	return n, true
}

// ParseYesNo reads a yes/no answer. An empty line takes the default, so the
// prompt can say [Y/n] and mean it.
func ParseYesNo(answer string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "":
		return def
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return def
	}
}

// CleanSecret trims the line ending a console read leaves behind, and NOTHING
// else.
//
// It deliberately does not trim spaces. An ARK ServerAdminPassword may
// legitimately begin or end with one, and silently removing it would produce a
// "the server refused the password" that is our fault and reads as theirs -
// which is the single most expensive wrong answer this program can give, since
// the owner's next move is to rotate a credential that was fine.
func CleanSecret(s string) string {
	return strings.TrimRight(s, "\r\n")
}
