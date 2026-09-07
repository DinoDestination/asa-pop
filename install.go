package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// The scheduled task.
//
// WHY THIS FLAG EXISTS AT ALL: walking somebody through the Task Scheduler GUI
// is where installs die. `schtasks` is already how this project runs its own
// ingest on Windows, so it is the same pattern one level out.
//
// EVERY FIVE MINUTES, AND THE TASK SURVIVES A REBOOT - but it only RUNS while
// the installing user is logged on, which is measured rather than assumed and
// is written out beside the arguments below. The site treats a count as live
// for 25 minutes - five missed reports - so a blip costs nothing and a machine
// that is genuinely off stops showing a number rather than freezing its last
// one.

const taskName = "Dino Destination population reporter"

func doInstall() int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Printf("Could not work out where this program is: %v\n", err)
		return 1
	}

	// A CONFIG CHECK BEFORE THE TASK, not after. A task installed against a
	// missing or unparseable config runs every five minutes and fails silently
	// into a log nobody opens - which is indistinguishable from working.
	cfg, path, err := loadConfig()
	if err != nil {
		fmt.Printf("Not installing: could not read %s\n  %v\n", path, err)
		fmt.Println("\nCreate the config first, then run --probe to check it, then --install.")
		return 1
	}
	if len(cfg.Servers) == 0 {
		fmt.Printf("Not installing: %s has no servers in it.\n", path)
		return 1
	}
	missing := 0
	for _, s := range cfg.Servers {
		if s.Key == "" || s.Password == "" {
			missing++
		}
	}
	if missing > 0 {
		fmt.Printf("Not installing: %d server(s) are missing a key or a password.\n", missing)
		fmt.Println("Fill those in, run --probe, then --install.")
		return 1
	}

	// /F replaces an existing task rather than failing, so re-running after
	// moving the binary fixes the path instead of leaving a task pointing at
	// somewhere the exe no longer is.
	args := []string{
		"/Create", "/F",
		"/TN", taskName,
		"/TR", `"` + exe + `"`,
		"/SC", "MINUTE",
		"/MO", "5",
		// LOGGED-ON ONLY, AND THAT IS MEASURED RATHER THAN INTENDED.
		//
		// This comment used to read "RUNS WHETHER OR NOT ANYBODY IS LOGGED IN".
		// It was false. A throwaway task created with exactly this argument
		// list reports Principal.LogonType = Interactive and UserId = the
		// installing user, so it does not run while nobody is logged on. After
		// a reboot with no login there are no reports and no log lines saying
		// why - the silent failure the log exists to make visible, wearing a
		// comment that said it could not happen.
		//
		// THE ARGUMENTS ARE STILL RIGHT FOR AN UNELEVATED INSTALL, which is why
		// the comment changed and they did not. Measured on Windows 11 without
		// elevation: "/RU <user> /NP" and "/RU SYSTEM" both fail with "Access
		// is denied", and "/NP" first prompts for a password on stdin - which
		// exec.Command does not supply, so it would hang rather than fail.
		// Run-whether-logged-on needs an Administrator prompt, and that is a
		// decision for whoever installs this rather than one to take on their
		// behalf.
		//
		// What it costs is printed after the install, because an owner who
		// leaves the box logged out has to know the reporting stops.
		"/RL", "LIMITED",
	}

	out, err := exec.Command("schtasks", args...).CombinedOutput()
	if err != nil {
		fmt.Printf("Could not create the scheduled task: %v\n%s\n", err, strings.TrimSpace(string(out)))
		fmt.Println("\nIf that says access is denied, run this from an Administrator command prompt.")
		return 1
	}

	fmt.Printf("Installed. \"%s\" now runs every 5 minutes.\n", taskName)
	fmt.Printf("  program : %s\n", exe)
	fmt.Printf("  config  : %s\n", path)
	fmt.Printf("  log     : %s\n", filepath.Join(filepath.Dir(exe), logName))
	fmt.Println("\nONE THING TO KNOW: the task runs as you, and only while you are")
	fmt.Println("logged on. If this machine reboots and nobody logs in, reporting")
	fmt.Println("stops until somebody does - the log will simply have no new lines.")
	fmt.Println("To have it run regardless, open Task Scheduler, find")
	fmt.Printf("  %q,\n", taskName)
	fmt.Println("and tick \"Run whether user is logged on or not\". Windows asks for")
	fmt.Println("your password to do that, which is why this program does not.")
	fmt.Println("\nRemove it any time with:  asa-pop.exe --uninstall")
	fmt.Println("Your listing shows a live count within about five minutes.")
	return 0
}

func doUninstall() int {
	out, err := exec.Command("schtasks", "/Delete", "/F", "/TN", taskName).CombinedOutput()
	if err != nil {
		txt := strings.TrimSpace(string(out))
		// A task that is already gone is the state the owner asked for, not an
		// error to make them read.
		if strings.Contains(strings.ToLower(txt), "cannot find") {
			fmt.Println("Nothing to remove - the scheduled task is not installed.")
			return 0
		}
		fmt.Printf("Could not remove the scheduled task: %v\n%s\n", err, txt)
		return 1
	}
	fmt.Printf("Removed. \"%s\" will not run again.\n", taskName)
	fmt.Println("Your listing stops showing a live count within 25 minutes, and says so.")
	fmt.Println("The average it already built stays - that is history, not a claim about now.")
	return 0
}
