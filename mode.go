package main

// WHICH MODE A RUN IS IN, as a pure function.
//
// This is the only genuinely dangerous piece of routing in the program, and it
// is separated out so it can be tested without a console, a config file or a
// server.
//
// THE FAILURE TO AVOID: the scheduled task runs `asa-pop.exe` with NO ARGUMENTS
// every five minutes, which is exactly the same command line a first-time owner
// produces by double-clicking. If those two were told apart by the arguments
// alone, a config that went missing would put the scheduled task into an
// interactive prompt - waiting on stdin that will never arrive, forever, every
// five minutes, accumulating processes on somebody's game server.
//
// So they are told apart by whether a HUMAN IS THERE: `interactive` is whether
// stdin is a console. A scheduled task has no console, so it can never take the
// guided path however broken its config is - it reports, fails loudly, and
// exits. `TestNonInteractiveIsNeverGuided` asserts that across every
// combination, because it is the one that would be silent.

type Mode int

const (
	ModeReport Mode = iota
	ModeGuided
	ModeProbe
	ModeInstall
	ModeUninstall
	ModeVersion
)

type Flags struct {
	Probe     bool
	Raw       bool
	// --wire implies --probe (main.go ORs them), so this never routes on its
	// own and ChooseMode needs no new branch.
	Wire      bool
	Install   bool
	Uninstall bool
	Version   bool
}

// ChooseMode decides what a run does.
//
//	hasConfig      asa-pop.json exists and parsed
//	configUsable   it has at least one server with a key AND a password
//	interactive    stdin is a console, i.e. somebody is watching
func ChooseMode(f Flags, hasConfig, configUsable, interactive bool) Mode {
	// AN EXPLICIT FLAG ALWAYS WINS. A technical owner who typed `--probe` gets
	// the probe, config or no config, console or no console.
	switch {
	case f.Version:
		return ModeVersion
	case f.Install:
		return ModeInstall
	case f.Uninstall:
		return ModeUninstall
	case f.Probe:
		return ModeProbe
	}

	// NO ARGUMENTS, AND NOBODY WATCHING: this is the scheduled task. It reports,
	// and if the config is broken it says so in the log and exits non-zero.
	// There is no branch from here into a prompt.
	if !interactive {
		return ModeReport
	}

	// NO ARGUMENTS, SOMEBODY WATCHING. A config that can actually report is the
	// owner testing their install by hand, so honour it. Anything else is a
	// first run - or a half-finished one - and gets the guided probe.
	if hasConfig && configUsable {
		return ModeReport
	}
	return ModeGuided
}
