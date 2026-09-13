package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// THE TWO FILES A NON-TECHNICAL OWNER DOUBLE-CLICKS.
//
// `--install` is the step that makes this tool do its job, and a flag cannot be
// passed by double-clicking a program - so the one action that mattered was the
// one action that needed a command prompt, in a tool whose entire premise is
// that an owner never opens one.
//
// They are tested for the same reason everything else here is: they ship to
// somebody who cannot debug them, and every failure asserted below is silent on
// their machine. A missing `pause` looks exactly like a program that did
// nothing; a missing `cd /d` looks exactly like a corrupt download.

const (
	batchOn  = "Turn on automatic reporting.bat"
	batchOff = "Turn off automatic reporting.bat"
)

func readBatch(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("%s is missing, and release.yml publishes it as an asset: %v", name, err)
	}
	return string(b)
}

func TestBatchFilesAreDoubleClickable(t *testing.T) {
	cases := []struct{ file, flag string }{
		{batchOn, "--install"},
		{batchOff, "--uninstall"},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			s := readBatch(t, c.file)

			// THE FOLDER THIS FILE IS IN, not whichever directory Windows
			// started us in. Without it, a run from a shortcut or from anywhere
			// but the folder itself looks for asa-pop.exe where it is not.
			if !strings.Contains(s, `cd /d "%~dp0"`) {
				t.Error(`no 'cd /d "%~dp0"': breaks whenever the working directory is not the folder`)
			}
			// PAUSE ON THE SUCCESS PATH, and that is not the same question as
			// "does this file contain the word pause".
			//
			// FOUND BY MUTATION, having been written the weak way first. The
			// not-found guard above has a pause of its own, so a plain
			// `Contains(s, "pause")` was satisfied by THAT one and survived
			// deleting the real one - the window would have flashed on every
			// successful install, which is the one failure this assertion exists
			// for, and the test would have stayed green. So the pause is looked
			// for AFTER the command, where it has to be to do anything.
			cmd := strings.Index(s, "asa-pop.exe "+c.flag)
			switch {
			case cmd < 0:
				t.Errorf("does not run 'asa-pop.exe %s'", c.flag)
			case !strings.Contains(s[cmd:], "pause"):
				t.Error("no pause after the command: the install prints the one warning that " +
					"matters - it runs only while you are logged on - and the window closes first")
			}
			// THE TWO-FILE DOWNLOAD PROBLEM. The exe and these are separate
			// release assets, so ending up in different folders is the expected
			// mistake rather than an unlikely one, and cmd.exe's own message for
			// it is "'asa-pop.exe' is not recognized", which reads as a broken
			// program rather than a misplaced file.
			if !strings.Contains(s, `if not exist "asa-pop.exe"`) {
				t.Error("no guard for asa-pop.exe sitting in a different folder")
			}
			// CRLF, AND THIS IS THE ONE THAT CAN FAIL NOWHERE ELSE.
			// .gitattributes forces it on checkout on every platform; cmd.exe
			// misreads the multi-line block above when the endings are LF, and
			// it does that on the owner's machine, never in CI.
			if !strings.Contains(s, "\r\n") {
				t.Error("LF line endings: cmd.exe needs CRLF - see the *.bat rule in .gitattributes")
			}
		})
	}
}

// THE FLAG EACH FILE PASSES IS A FLAG THE PROGRAM DECLARES.
//
// Read out of the batch file and checked against main.go, rather than both
// written down here: renaming a flag on either side fails, and nothing else in
// this suite connects the two. Without it, `--install` becoming `--schedule`
// leaves two double-clickable shortcuts to a usage error.
func TestBatchFilesPassFlagsTheProgramDeclares(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`asa-pop\.exe\s+--([a-z-]+)`)
	for _, name := range []string{batchOn, batchOff} {
		found := re.FindAllStringSubmatch(readBatch(t, name), -1)
		if len(found) == 0 {
			t.Fatalf("%s never runs asa-pop.exe with a flag", name)
		}
		for _, hit := range found {
			decl := `flag.Bool("` + hit[1] + `"`
			if !strings.Contains(string(src), decl) {
				t.Fatalf("%s passes --%s and main.go declares no %s", name, hit[1], decl)
			}
		}
	}
}
