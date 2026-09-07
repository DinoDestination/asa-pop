package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// The Dino Destination population reporter.
//
// WHAT IT DOES, IN ONE SENTENCE: every five minutes it asks YOUR server how
// many players are on, over RCON, on your own machine, and posts that number to
// dinodestination.com so your listing can show a live count.
//
// WHAT LEAVES THIS MACHINE: a key and an integer. Nothing else. Your RCON
// password is read from the file beside this program, used against 127.0.0.1,
// and is not part of the request - the endpoint has no field that could carry
// it, and neither does the database behind it.
//
// WHY THERE IS A BINARY AT ALL. This is one file of Go with no third-party
// dependencies, built by a public GitHub Actions workflow with `-trimpath` and
// a pinned toolchain, so the same source produces the same bytes. The SHA256 is
// printed beside the download. You are not asked to trust the binary - you are
// given a way to check it against the source.

// version is set at build time: -ldflags "-X main.version=<tag>".
// "dev" means somebody built this by hand, and it says so rather than claiming
// to be a release.
var version = "dev"

const userAgent = "dino-destination-reporter/"

// DEFAULTS THAT MATCH A STANDARD ASA SETUP. `RCONPort=27020` is what almost
// every guide sets and what most hosts already have, so the owner usually only
// has to paste a password and a key.
const (
	defaultPort     = 27020
	defaultHost     = "127.0.0.1"
	defaultEndpoint = "https://www.dinodestination.com/api/population/report"
	rconTimeout     = 8 * time.Second
	postTimeout     = 15 * time.Second
)

type ServerConf struct {
	// A label for the log, so an owner with twelve maps can tell which line is
	// which. Never sent.
	Name string `json:"name"`
	// 127.0.0.1 unless the server is on another box on the LAN. RCON does NOT
	// need to be exposed to the internet for this to work, which is the whole
	// reason the reporter runs here rather than us connecting in.
	Host string `json:"host"`
	Port int    `json:"port"`
	// STAYS IN THIS FILE. Not sent, not logged, not hashed and stored.
	Password string `json:"password"`
	// Issued at dinodestination.com/manage. One per server.
	Key string `json:"key"`
}

type Config struct {
	Endpoint string       `json:"endpoint"`
	Servers  []ServerConf `json:"servers"`
}

func configPath() (string, error) {
	// BESIDE THE BINARY, not in the working directory. A scheduled task runs
	// with a working directory the owner never chose (usually C:\Windows\
	// System32), so a relative path would work when double-clicked and fail
	// silently every five minutes afterwards.
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), logConfigName), nil
}

// The config's filename, named once so the log can refer to it by the same
// string the code opens.
const logConfigName = "asa-pop.json"

func loadConfig() (*Config, string, error) {
	path, err := configPath()
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, path, err
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, path, fmt.Errorf("%s is not valid JSON: %w", filepath.Base(path), err)
	}
	if c.Endpoint == "" {
		c.Endpoint = defaultEndpoint
	}
	for i := range c.Servers {
		if c.Servers[i].Host == "" {
			c.Servers[i].Host = defaultHost
		}
		if c.Servers[i].Port == 0 {
			c.Servers[i].Port = defaultPort
		}
	}
	return &c, path, nil
}

// report posts one count. The body is exactly {key, count}.
func report(endpoint, key string, count int) error {
	body, err := json.Marshal(map[string]any{"key": key, "count": count})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	// THE VERSION RIDES IN THE USER-AGENT, not in the payload. It is genuinely
	// useful to know which builds are in the field, and putting it in the body
	// would widen a request whose narrowness is the security claim.
	req.Header.Set("User-Agent", userAgent+version)

	client := &http.Client{Timeout: postTimeout}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode/100 == 2 {
		return nil
	}
	// The endpoint says WHY, and the reason is what an owner needs. It never
	// contains the key.
	var out struct {
		Why string `json:"why"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	if out.Why != "" {
		return fmt.Errorf("%s (HTTP %d)", out.Why, res.StatusCode)
	}
	return fmt.Errorf("HTTP %d", res.StatusCode)
}

func stamp() string { return time.Now().Format("2006-01-02 15:04:05") }

func main() {
	var (
		probe     = flag.Bool("probe", false, "check RCON and print the shape of the reply, without reporting anything")
		raw       = flag.Bool("raw", false, "with --probe, show player names and ids (local only - do not paste this)")
		install   = flag.Bool("install", false, "create a scheduled task that runs this every 5 minutes")
		uninstall = flag.Bool("uninstall", false, "remove that scheduled task")
		showVer   = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	// THE STATE, GATHERED ONCE, and the decision made by a pure function that
	// is tested without any of it. `configUsable` is the question that matters:
	// a config file that exists but cannot actually report is somebody midway
	// through setup, not somebody with a working install.
	cfg, _, cfgErr := loadConfig()
	hasConfig := cfgErr == nil
	configUsable := false
	if hasConfig {
		for _, s := range cfg.Servers {
			if s.Key != "" && s.Password != "" {
				configUsable = true
				break
			}
		}
	}

	switch ChooseMode(
		Flags{Probe: *probe, Raw: *raw, Install: *install, Uninstall: *uninstall, Version: *showVer},
		hasConfig, configUsable, stdinIsConsole(),
	) {
	case ModeVersion:
		fmt.Println(userAgent + version)
	case ModeInstall:
		os.Exit(doInstall())
	case ModeUninstall:
		os.Exit(doUninstall())
	case ModeProbe:
		os.Exit(doProbe(*raw))
	case ModeGuided:
		os.Exit(doGuided())
	default:
		os.Exit(doReport())
	}
}

// ---------------------------------------------------------------------------
// --probe
// ---------------------------------------------------------------------------
//
// THE SAME RCON PATH AND THE SAME COUNTER the scheduled run uses. A probe that
// exercised a nearby code path would prove nothing about what runs every five
// minutes - the failure this project has written down four times.
//
// It reports the SHAPE: how many frames came back, how many bytes, whether the
// 4096-byte split was crossed, and what the counter made of it. Names and ids
// are redacted unless --raw, because this output is meant to be pasted to us.
func doProbe(raw bool) int {
	fmt.Printf("\nDino Destination population reporter %s\n", version)
	fmt.Println("PROBE - nothing is reported to anybody, and nothing is changed on your server.")
	fmt.Println("The only command sent is `ListPlayers`, which is read-only.")
	fmt.Println()

	cfg, path, err := loadConfig()
	if err != nil {
		fmt.Printf("Could not read %s\n  %v\n\n", path, err)
		fmt.Println("Create it beside this program. The smallest version that works:")
		fmt.Println(sampleConfig)
		return 1
	}
	if len(cfg.Servers) == 0 {
		fmt.Printf("%s has no servers in it.\n%s\n", path, sampleConfig)
		return 1
	}

	fmt.Printf("Config: %s\n", path)
	fmt.Printf("%d server(s) configured.\n\n", len(cfg.Servers))

	bad := 0
	for i, s := range cfg.Servers {
		label := s.Name
		if label == "" {
			label = fmt.Sprintf("server %d", i+1)
		}
		fmt.Printf("  %s  (%s:%d)\n", label, s.Host, s.Port)

		if s.Password == "" {
			fmt.Printf("    no password set in the config - nothing to try\n\n")
			bad++
			continue
		}

		text, shape, outcome := ListPlayers(s.Host, s.Port, s.Password, rconTimeout)
		fmt.Printf("    outcome : %s - %s\n", outcome, Explain(outcome))
		if outcome != OutcomeOK {
			bad++
			fmt.Println()
			continue
		}

		fmt.Printf("    frames  : %d\n", shape.Frames)
		fmt.Printf("    bytes   : %d\n", shape.Bytes)
		fmt.Printf("    split   : %v", shape.Split)
		if shape.Split {
			fmt.Printf("  (the reply crossed the 4096-byte boundary and was reassembled)")
		}
		fmt.Println()

		n, ok := CountPlayers(text)
		if ok {
			fmt.Printf("    count   : %d\n", n)
		} else {
			// NOT A ZERO. This is the one result worth sending back to us: it
			// means ASA answered in a shape the counter does not recognise, and
			// the raw form below is what fixes it.
			fmt.Printf("    count   : NOT UNDERSTOOD - this would be reported as nothing, never as 0\n")
			bad++
		}

		shown := text
		if !raw {
			shown = Redact(text)
		}
		fmt.Println("    reply   :")
		for j, line := range splitLines(shown) {
			if j >= 12 {
				fmt.Printf("      ... and %d more lines\n", len(splitLines(shown))-12)
				break
			}
			fmt.Printf("      %s\n", line)
		}
		if !raw {
			fmt.Println("    (names and ids redacted - this output is safe to paste to us)")
		}
		fmt.Println()
	}

	if bad == 0 {
		fmt.Println("All good. Run `asa-pop.exe --install` to report every 5 minutes.")
		return 0
	}
	fmt.Printf("%d server(s) did not answer as expected. Nothing was reported.\n", bad)
	return 1
}

func splitLines(s string) []string {
	out := []string{}
	cur := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, cur)
			cur = ""
			continue
		}
		if r == '\r' {
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// ---------------------------------------------------------------------------
// the default action: read the config, report once, exit
// ---------------------------------------------------------------------------
//
// ONE SHOT, NOT A DAEMON. A crashed run is recovered by the next one five
// minutes later, there is no process to supervise or leak, and it survives a
// reboot because the scheduled task does.
//
// A SERVER THAT IS DOWN REPORTS NOTHING. It does not report 0 - that would put
// a live "0 online" badge on a machine that is off, which is the flat-line-
// across-an-outage failure the site is built to avoid. No report means the
// count goes stale after 25 minutes and the listing says so.
func doReport() int {
	cfg, path, err := loadConfig()
	if err != nil {
		fmt.Printf("%s  could not read %s: %v\n", stamp(), path, err)
		return 1
	}

	failed := 0
	// Said at most once per run, however many servers fail to log. See
	// `writeLog`: a broken log must not bury the lines it exists to record.
	logComplained := false

	for i, s := range cfg.Servers {
		label := s.Name
		if label == "" {
			label = fmt.Sprintf("server %d", i+1)
		}

		// EVERY BRANCH BELOW WRITES EXACTLY ONE LINE, success and failure
		// alike. A log that only recorded failures could not tell "working"
		// from "not running at all" - and "not running at all" is the likely
		// one here, because the scheduled task only runs while somebody is
		// logged in. See install.go.
		if s.Key == "" || s.Password == "" {
			fmt.Printf("%s  %s: not configured (needs a key and a password)\n", stamp(), label)
			writeLog(LogLine(stamp(), label, "not-configured", nil,
				"needs a key and a password in "+logConfigName), &logComplained)
			failed++
			continue
		}

		text, _, outcome := ListPlayers(s.Host, s.Port, s.Password, rconTimeout)
		if outcome != OutcomeOK {
			fmt.Printf("%s  %s: %s - %s\n", stamp(), label, outcome, Explain(outcome))
			// `Explain` is our own text and quotes nothing that was sent - the
			// outcome codes exist so a failure can be described without
			// repeating the credential that produced it.
			writeLog(LogLine(stamp(), label, string(outcome), nil, Explain(outcome)), &logComplained)
			failed++
			continue
		}

		n, ok := CountPlayers(text)
		if !ok {
			fmt.Printf("%s  %s: the reply was not a player list we understood - reporting nothing. "+
				"Run --probe and send us the output.\n", stamp(), label)
			// THE ONE PLACE SERVER TEXT REACHES THE LOG, and the only place it
			// is any use: this branch means ASA answered in a shape the counter
			// does not recognise, and the shape IS the diagnostic.
			//
			// Through `Redact` first, so a roster becomes a count of redacted
			// lines rather than a list of who was playing. Through `logSafe`
			// after, so a reply containing newlines cannot forge entries in
			// this file.
			writeLog(LogLine(stamp(), label, "not-understood", nil,
				"reply not recognised: "+Redact(text)), &logComplained)
			failed++
			continue
		}

		if err := report(cfg.Endpoint, s.Key, n); err != nil {
			// THE KEY IS NEVER IN THIS LINE. A log is the one place people
			// paste into a support thread.
			fmt.Printf("%s  %s: counted %d, but the report was refused: %v\n", stamp(), label, n, err)
			writeLog(LogLine(stamp(), label, "not-reported", &n,
				"the report was refused: "+err.Error()), &logComplained)
			failed++
			continue
		}
		fmt.Printf("%s  %s: reported %d\n", stamp(), label, n)
		writeLog(LogLine(stamp(), label, "ok", &n, "reported"), &logComplained)
	}

	if failed > 0 {
		return 1
	}
	return 0
}

const sampleConfig = `{
  "servers": [
    {
      "name": "The Island",
      "host": "127.0.0.1",
      "port": 27020,
      "password": "your ServerAdminPassword",
      "key": "the key from dinodestination.com/manage"
    }
  ]
}`
