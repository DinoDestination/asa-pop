package main

import "testing"

// THE ONE THAT MATTERS. A scheduled task has no console; if any combination of
// state could route it into a prompt, it would block on stdin that never
// arrives - every five minutes, forever, on somebody's game server - and the
// only symptom would be a listing that quietly stopped updating.
func TestNonInteractiveIsNeverGuided(t *testing.T) {
	for _, hasConfig := range []bool{false, true} {
		for _, usable := range []bool{false, true} {
			got := ChooseMode(Flags{}, hasConfig, usable, false)
			if got == ModeGuided {
				t.Fatalf("hasConfig=%v usable=%v routed a task with no console into the guided prompt",
					hasConfig, usable)
			}
			if got != ModeReport {
				t.Fatalf("hasConfig=%v usable=%v gave mode %v, want ModeReport", hasConfig, usable, got)
			}
		}
	}
}

func TestDoubleClickFirstRunIsGuided(t *testing.T) {
	// The state a first-time tester is in: they downloaded one file and
	// double-clicked it. No arguments, no config, a console because Explorer
	// made one.
	if got := ChooseMode(Flags{}, false, false, true); got != ModeGuided {
		t.Fatalf("got %v, want ModeGuided", got)
	}
}

func TestHalfFinishedConfigIsGuided(t *testing.T) {
	// A config that exists but cannot report - no key yet, or no password - is
	// somebody midway through setup, not somebody testing a working install.
	// Reporting would print a refusal they have no context for.
	if got := ChooseMode(Flags{}, true, false, true); got != ModeGuided {
		t.Fatalf("got %v, want ModeGuided", got)
	}
}

func TestWorkingConfigRunByHandReports(t *testing.T) {
	// An owner double-clicking a finished install is checking that it works.
	// Sending them back through setup would be wrong.
	if got := ChooseMode(Flags{}, true, true, true); got != ModeReport {
		t.Fatalf("got %v, want ModeReport", got)
	}
}

// AN EXPLICIT FLAG ALWAYS WINS, in every state. A technical owner who typed
// --probe must not be sent through a guided prompt because their config happens
// to be missing.
func TestFlagsBeatEveryState(t *testing.T) {
	cases := []struct {
		name string
		f    Flags
		want Mode
	}{
		{"probe", Flags{Probe: true}, ModeProbe},
		{"install", Flags{Install: true}, ModeInstall},
		{"uninstall", Flags{Uninstall: true}, ModeUninstall},
		{"version", Flags{Version: true}, ModeVersion},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, hasConfig := range []bool{false, true} {
				for _, usable := range []bool{false, true} {
					for _, interactive := range []bool{false, true} {
						if got := ChooseMode(c.f, hasConfig, usable, interactive); got != c.want {
							t.Fatalf("hasConfig=%v usable=%v interactive=%v: got %v want %v",
								hasConfig, usable, interactive, got, c.want)
						}
					}
				}
			}
		})
	}
}
