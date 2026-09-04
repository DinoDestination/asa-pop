package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// THE DOUBLE-CLICK PATH.
//
// A command line is a wall. "Open cmd, cd to the folder, run it with a flag" is
// three things that can go wrong before anything is learned, and most owners
// bounce at the first. The people this is for install servers with SteamCMD and
// drop DLLs into Binaries\Win64 - they are not afraid of an .exe, they are just
// not going to type a flag.
//
// So: download, double-click, type the password, copy what it says.
//
// IT PAUSES AT THE END, ALWAYS. Double-clicking a console program on Windows
// opens a window that closes the instant the program exits - so without this
// the entire output flashes past and the owner has nothing to copy. That single
// missing line would make the whole flow useless while looking like it worked,
// which is why `TestGuidedAlwaysPauses` covers every exit from it.
//
// EVERY INPUT AND OUTPUT IS INJECTED, so the whole flow is driven by a test
// with a synthetic server rather than checked by eye. It cannot be driven by
// piping into the real binary - a pipe is not a console, and `stdinIsConsole`
// correctly refuses the guided path there, which is the guard that keeps the
// scheduled task out of a prompt.

type ui struct {
	in  *bufio.Reader
	out io.Writer
	// secret reads a line without echoing it. Injected because the real one
	// talks to the Windows console API.
	secret func(prompt string) (string, error)
}

func stdUI() *ui {
	return &ui{in: bufio.NewReader(os.Stdin), out: os.Stdout, secret: readSecret}
}

func (u *ui) say(format string, args ...any) { fmt.Fprintf(u.out, format+"\n", args...) }
func (u *ui) line()                          { fmt.Fprintln(u.out) }
func (u *ui) rule()                          { fmt.Fprintln(u.out, strings.Repeat("-", 64)) }

func (u *ui) ask(prompt string) string {
	fmt.Fprint(u.out, prompt)
	line, err := u.in.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}

// pause holds the window open until Enter. The message says why, because a
// window that stops for no visible reason reads as a hang.
func (u *ui) pause() {
	fmt.Fprint(u.out, "\nPress Enter to close this window.")
	_, _ = u.in.ReadString('\n')
}

func doGuided() int { return guided(stdUI()) }

func guided(u *ui) int {
	u.line()
	u.rule()
	u.say("  Dino Destination - server population check")
	u.rule()
	u.line()
	u.say("This checks whether we can read your player count from your own")
	u.say("server, so your listing can show a live number.")
	u.line()
	u.say("  * Nothing is sent anywhere. This is only a check.")
	u.say("  * Nothing on your server is changed. The only command used is")
	u.say("    ListPlayers, which just reads who is online.")
	u.say("  * Your password stays on this computer.")
	u.line()
	u.say("You need your ServerAdminPassword - the one in GameUserSettings.ini.")
	u.say("It is the same password you use for admin commands in game.")
	u.line()

	var servers []ServerConf
	for {
		s, ok := askOneServer(u, len(servers)+1)
		if ok {
			servers = append(servers, s)
		}
		u.line()
		// A CLUSTER IS THE NORMAL CASE, not the exception - twelve maps is an
		// ordinary ARK network. Asking beats making them hand-edit JSON.
		if !ParseYesNo(u.ask("Do you run another map on this computer? [y/N]: "), false) {
			break
		}
		u.line()
	}

	u.line()
	u.rule()
	if len(servers) == 0 {
		u.say("  Nothing answered.")
		u.rule()
		u.line()
		u.say("Copy everything above and send it to us - the reason it gives is")
		u.say("usually enough to say what to change.")
		u.line()
		u.say("The two things that are almost always it:")
		u.say("  * RCONEnabled=True must be in GameUserSettings.ini under [ServerSettings]")
		u.say("  * the port is RCONPort (usually 27020), NOT the port players join on")
		u.pause()
		return 1
	}

	u.say("  Worked on %d of your server(s).", len(servers))
	u.rule()
	u.line()
	u.say("SELECT ALL THE TEXT ABOVE, copy it, and send it to us.")
	u.say("Player names and IDs are already hidden, so it is safe to paste.")
	u.line()

	// SAVING IS OFFERED, NOT ASSUMED. It writes the password to a file beside
	// this program, and doing that unasked - on a machine somebody else may also
	// administer - is not ours to decide.
	if ParseYesNo(u.ask("Save these settings so you do not have to type them again? [Y/n]: "), true) {
		if path, err := saveConfig(servers); err != nil {
			u.line()
			u.say("  Could not save: %v", err)
		} else {
			u.line()
			u.say("  Saved to %s", path)
			u.say("  It contains your password, so keep it with your server files.")
			u.line()
			u.say("  Next: get your reporting key from dinodestination.com/manage,")
			u.say("  paste it into that file, then run this program again to start")
			u.say("  reporting every 5 minutes.")
		}
	}

	u.pause()
	return 0
}

// askOneServer prompts for one server and probes it immediately, so a mistake
// is caught while the owner is still looking at the thing they mistyped rather
// than after they have entered all twelve.
func askOneServer(u *ui, n int) (ServerConf, bool) {
	s := ServerConf{Host: defaultHost, Name: fmt.Sprintf("server %d", n)}

	for {
		answer := u.ask(fmt.Sprintf("RCON port [%d]: ", defaultPort))
		port, ok := ParsePort(answer, defaultPort)
		if ok {
			s.Port = port
			break
		}
		u.say("  That is not a port number. Just digits, or press Enter for %d.", defaultPort)
	}

	secret, err := u.secret("ServerAdminPassword (typing is hidden): ")
	if err != nil || secret == "" {
		u.say("  No password entered - skipping this one.")
		return s, false
	}
	s.Password = secret

	u.line()
	u.say("Checking 127.0.0.1:%d ...", s.Port)

	text, shape, outcome := ListPlayers(s.Host, s.Port, s.Password, rconTimeout)
	if outcome != OutcomeOK {
		u.say("  DID NOT WORK: %s", Explain(outcome))
		return s, false
	}

	count, understood := CountPlayers(text)
	if !understood {
		// THE ONE RESULT WORTH SENDING BACK. The server answered and the reply
		// is not a shape the counter knows - which is precisely what a probe is
		// for, and it must not read as the owner's mistake.
		u.say("  Connected and read a reply, but it is not in a form we recognise.")
		u.say("  This is OUR side to fix, not yours. The reply looked like this:")
		for i, l := range splitLines(Redact(text)) {
			if i >= 6 {
				u.say("      ...")
				break
			}
			u.say("      %s", l)
		}
		return s, false
	}

	u.say("  WORKED. %d player(s) online right now.", count)
	extra := ""
	if shape.Split {
		extra = ", reassembled across a packet boundary"
	}
	u.say("  (reply came back in %d packet(s), %d bytes%s)", shape.Frames, shape.Bytes, extra)

	// The name is cosmetic and never sent, so it is asked last and an empty
	// answer is fine.
	if name := u.ask("  What is this map called? (optional): "); name != "" {
		s.Name = name
	}
	return s, true
}

func saveConfig(servers []ServerConf) (string, error) {
	path, err := configPath()
	if err != nil {
		return "", err
	}
	// PRESERVE AN EXISTING KEY. Somebody re-running the check after pasting
	// their key in must not have it wiped by the save - that would look exactly
	// like the key being rejected.
	if existing, _, err := loadConfig(); err == nil {
		for i := range servers {
			for _, old := range existing.Servers {
				if old.Port == servers[i].Port && old.Key != "" {
					servers[i].Key = old.Key
				}
			}
		}
	}

	body, err := json.MarshalIndent(Config{Servers: servers}, "", "  ")
	if err != nil {
		return "", err
	}
	// 0600: it holds an admin password.
	if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
