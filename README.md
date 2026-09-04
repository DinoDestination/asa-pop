# Population reporter

A self-hosted ARK: Survival Ascended cluster has no player count anywhere we can
reach. It is absent from Wildcard's published server list by definition, EOS
matchmaking answers `policy_missing_action` for the public client, and ASA does
not reply to A2S on any port — measured against five live servers on three ports
each, fifteen queries, fifteen nulls.

So the count has to come from the owner. This runs on **their** machine, asks
**their** server over RCON on `127.0.0.1`, and posts the number.

## What leaves the machine

`{"key": "...", "count": 7}`. That is the whole request.

The RCON password is read from `asa-pop.json` beside the binary, used against
the loopback address, and zeroed after use. It is not in the payload, not in the
logs, and not storable at the other end — `record_population_report` takes a key
and an integer and has no parameter that could carry a credential, which a
schema test asserts against the function's own signature.

**RCON does not need to be exposed to the internet for this to work.** That is
the point of the reporter running there rather than us connecting in, and it
makes this strictly safer than the RCON *claim* flow, which does ask an owner to
open a port to us.

## Usage

```
asa-pop.exe --probe        check RCON, print the shape of the reply, report nothing
asa-pop.exe --probe --raw  the same, without redacting names and ids (local only)
asa-pop.exe                read the config, report once, exit
asa-pop.exe --install      scheduled task, every 5 minutes, survives a reboot
asa-pop.exe --uninstall    remove it
asa-pop.exe --version
```

`asa-pop.json`, beside the binary:

```json
{
  "servers": [
    {
      "name": "The Island",
      "host": "127.0.0.1",
      "port": 27020,
      "password": "your ServerAdminPassword",
      "key": "issued at dinodestination.com/manage"
    }
  ]
}
```

`host` and `port` default to `127.0.0.1:27020`. One entry per server; one key
per server, so revoking one does not touch the others.

## Three decisions worth knowing about

**`--probe` uses the same RCON client and the same counter as the scheduled
run.** A probe exercising a nearby code path would prove nothing about what runs
every five minutes. It is the same code with the reporting turned off.

**A response the counter does not understand reports nothing — never zero.**
"The server said nobody is on" and "we could not read the answer" are different
states. A zero renders as a live measurement of an empty server; reporting
nothing lets the count go stale, and the site says so rather than showing a
number nobody stands behind. Same reason a server that is down reports nothing
instead of `0`.

**The reply is reassembled across the 4096-byte frame boundary.** Source RCON
splits there and a `ListPlayers` line is about 55 bytes, so a full server
crosses it around 74 players. A client returning on the first frame would
undercount — and worse, a truncation that lands on a newline parses *cleanly*,
so nothing downstream could detect it. `count_test.go` demonstrates exactly that
case: 90 players cut at the boundary counts as 74, correctly formed and wrong.

## Building it yourself

No third-party dependencies — Go's standard library only. The release build is
reproducible, so the published SHA256 is checkable against the source:

```
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o asa-pop.exe .
```

Go 1.27.1, pinned in `.github/workflows/reporter-release.yml`. Two builds from a
cleaned module cache produce an identical hash; `pnpm reporter:verify` rebuilds
the published tag and compares.

```
go vet ./... && go test ./...
```

The tests include a synthetic Source RCON server that splits at 4096 and answers
the end-of-response sentinel the way ARK does, so the client is proved end to
end against a server whose true player count the test chose.

## Not signed

The binary is not code-signed, so Windows SmartScreen will warn. That is a real
cost and the honest answer to it is the paragraph above: the program is small,
the source is here, and the build can be reproduced byte-for-byte.
