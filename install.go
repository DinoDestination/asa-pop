package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// The scheduled task.
//
// WHY THIS FLAG EXISTS AT ALL: walking somebody through the Task Scheduler GUI
// is where installs die. `schtasks` is already how this project runs its own
// ingest on Windows, so it is the same pattern one level out.
//
// EVERY FIVE MINUTES, FOREVER, AND IT SURVIVES A REBOOT. The site treats a
// count as live for 25 minutes - five missed reports - so a blip costs nothing
// and a machine that is genuinely off stops showing a number rather than
// freezing its last one.

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
		// RUNS WHETHER OR NOT ANYBODY IS LOGGED IN. A game server box is on
		// 24/7 with nobody at the console, and a task that only runs at logon
		// would report for the ten minutes after a reboot and then stop.
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
