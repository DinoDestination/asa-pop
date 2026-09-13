# Setting up asa-pop

**asa-pop** is a small program that sits on the computer running your ARK
server, checks how many people are playing every five minutes, and sends that
one number to your listing on [Dino Destination](https://www.dinodestination.com).
Nothing else leaves your machine — just the player count.

|  |  |
|---|---|
| **Time** | about 10 minutes |
| **Changes to your server** | usually none — you probably already have what it needs |
| **Restart needed?** | only if remote admin turns out to be switched off, which is uncommon |
| **Runs on** | Windows, the same computer as your server |

**[Download it here.](https://github.com/DinoDestination/asa-pop/releases/latest)**

---

## Before you start

Three things. If any of them is a no, stop here and message us — the program
can't work around them and it's better to know now.

- Your ARK server runs on **a Windows computer you can log into** — your own PC,
  or a machine you rent and can open a desktop on. If someone else hosts it for
  you (Nitrado, GPortal, or similar) this program can't be installed and we'll
  need a different approach.
- You can **open your server's settings file and read two lines out of it**, or
  ask whoever set the server up to read them to you. You almost certainly won't
  need to change anything.
- You have **your key from us** — a short line of characters we send you. It's
  what ties the count to your listing. Ask if you don't have one.

---

## Part one — find your admin password, and check remote admin is on

Your server can accept admin commands over the network. That's how asa-pop asks
"how many people are on?" It's a standard ARK feature called RCON, and **if you
use any admin tool at all it is already switched on** — web control panels,
Discord admin bots and the various RCON apps all work through it. That's most
servers.

You need to open your settings file anyway, to read your admin password. While
you're in there you can confirm remote admin in the same glance, which is why
this is one step rather than two.

> [!NOTE]
> **You do not need to stop your server to do this.** You're only reading the
> file. Nothing in this section changes anything.

**1. Open the settings file.** Starting from the folder your server is installed
in, go into:

```
ShooterGame\Saved\Config\WindowsServer\GameUserSettings.ini
```

Open it with Notepad. It's a long list of settings in plain text.

If you run your server through a web control panel rather than off a desktop,
the same settings are usually on a settings page there, labelled something like
**RCON**, **Enable RCON** or **Admin password**.

**2. Find the line that says `[ServerSettings]`** — square brackets and all.
There will be many settings listed underneath it. The three lines you care about
are somewhere in that block.

**3. Look for these three, and write down the last two.**

```ini
RCONEnabled=True
RCONPort=27020
ServerAdminPassword=something-here
```

**What you'll almost certainly see:** all three present, `RCONEnabled` set to
`True`, a port number (27020 is the usual one, but any number is fine — just
note which), and a password after the `=`. Capitalisation doesn't matter;
`true` is the same as `True`.

**If that's what you see, you're done here.** Write down the port and the
password and go to [Part two](#part-two--run-asa-pop-once-by-hand). No changes,
no restart.

**If it doesn't look like that** — a line is missing entirely, `RCONEnabled` says
`False`, or there's nothing after `ServerAdminPassword=` — then remote admin is
off and you're in the uncommon case. See
[If remote admin is off](#if-remote-admin-is-off) below, then come back here.

---

## Part two — run asa-pop once, by hand

This first run proves it can talk to your server. It asks two questions and
tells you straight away whether it worked.

**4. Make a folder for it** and put it somewhere you'll remember — `C:\asa-pop`
is fine. Don't use your Downloads folder: the program keeps its settings and its
log next to itself, and Downloads is a folder people empty.

**5. Download all three files** from
[the latest release](https://github.com/DinoDestination/asa-pop/releases/latest)
into that folder — `asa-pop.exe`, `Turn-on-automatic-reporting.bat` and
`Turn-off-automatic-reporting.bat`. They have to sit together; the two `.bat`
files look for the program next to themselves. Then double-click `asa-pop.exe`.

Windows will probably show a blue "Windows protected your PC" box. That's
because the program isn't signed by a big company, not because anything is wrong
with it. Click **More info**, then **Run anyway**. If you'd rather check it
first, the release page lists a checksum you can verify.

**6. A small black window opens and asks you two things.** Here's the whole
thing — this is all of it:

```
RCON port [27020]:
ServerAdminPassword (typing is hidden):
Checking 127.0.0.1:27020 ...
WORKED. 4 player(s) online right now.

Do you run another map on this computer? [y/N]:

Worked on 1 of your server(s).
```

For the port, press Enter if yours is 27020 — otherwise type the number you
wrote down. For the password, type the `ServerAdminPassword` you wrote down.
Nothing appears as you type; that's deliberate. Press Enter.

If you run more than one map on this same computer, answer **y** to the third
question and it'll ask again for the next one. Otherwise press Enter.

**7. If it says `WORKED`, you're past the hard part.** If it says `DID NOT
WORK`, the line underneath explains why — and the three most common reasons are
[at the bottom of this page](#when-it-doesnt-work) with fixes.

**8. Say yes when it offers to save your settings.** Press Enter. It writes a
small file called `asa-pop.json` next to the program so you never have to type
the password again.

That file contains your admin password, so it lives on your machine and nowhere
else. Don't post it, and don't put the folder anywhere shared.

**9. Add your key to that file.** Open `asa-pop.json` in Notepad. Find the part
that says `"key"` and paste the key we sent you between the quote marks, so it
looks like this:

```json
"key": "the-key-we-sent-you",
```

Save and close. Without the key the program knows your player count but has no
idea which listing it belongs to, so nothing shows up on the site.

---

## Part three — make it keep running

Right now it only counts when you double-click it. This last part makes Windows
do it every five minutes on its own.

**10. Double-click `Turn-on-automatic-reporting.bat`.** That's it — no typing.

It checks your saved settings first and refuses if anything's missing, so it
won't set up a job that can't work. If it succeeds, Windows runs the reporter
every five minutes under the name *Dino Destination population reporter*. The
window stays open so you can read what it says.

**11. Read the last few lines before you close it.** They tell you something
that matters: the job **only runs while you're logged into Windows**. If you log
out, or the computer restarts and nobody signs back in, the counting stops until
someone does.

> [!IMPORTANT]
> **This limit is real, and it's the one thing here we can't fix for you.** If
> the server machine normally sits logged in, there's nothing to do. If it
> doesn't, **tell us** — making it run regardless needs a Windows password typed
> into an Administrator prompt, which is why the program won't do it quietly
> behind your back. It's a five-minute job together, and the alternative is a
> counter that stops without telling you.

`Turn-off-automatic-reporting.bat` undoes step 10 the same way, whenever you
want. Your listing stops showing a live count within 25 minutes and says so; the
average already recorded stays, because that's history rather than a claim about
right now.

---

## If remote admin is off

**Most people never need this section.** You're here because Part one showed
`RCONEnabled=False`, a missing line, or a blank `ServerAdminPassword`.

> [!WARNING]
> **This part is genuinely technical, and it needs a server restart.** You're
> editing a settings file by hand and there's no button for it. If that isn't
> something you want to do, send this section to whoever set your server up —
> it's a two-minute job for them and they'll know exactly what it means.

**a. Stop your server.** Editing this file while the server is running won't
work — ARK overwrites it when it shuts down, so your change would disappear.
This is the most common mistake in the whole setup.

**b. Open `GameUserSettings.ini` again** (the path is in Part one) and find
`[ServerSettings]`.

**c. Make sure these three lines are there, underneath `[ServerSettings]`.** If
one already exists but is wrong, correct it rather than adding a second copy.

```ini
RCONEnabled=True
RCONPort=27020
ServerAdminPassword=SomethingLongAndPrivate
```

Replace `SomethingLongAndPrivate` with a password of your own, and write it
down. Don't leave it blank; the program can't connect without one.

**d. Save the file and start your server again.** The change only takes effect
on a restart — ARK reads this file when it boots and not afterwards.

Then go back to [Part two](#part-two--run-asa-pop-once-by-hand).

---

## When it's working

Within about ten minutes your listing shows a live player count. If it stops
arriving for 25 minutes the site marks it as stale rather than showing an old
number as though it were current.

**One thing worth knowing:** once the settings are saved, double-clicking
`asa-pop.exe` no longer opens the question-and-answer window. It does its job
silently and closes, which looks exactly like nothing happening. It isn't broken
— see the third fix below.

**To check on it a week later,** open `asa-pop.log` in the same folder. One line
per check, newest at the bottom, and it records failures as well as successes:

```
2026-09-12T14:05:02Z  the-island   ok            count=4     sent
2026-09-12T14:10:02Z  the-island   ok            count=6     sent
2026-09-12T14:15:02Z  the-island   port-closed   count=-     not sent
```

If the newest line is hours old, the scheduled job isn't running. If the lines
are recent but say something other than `ok`, the server side is the problem.

---

## When it doesn't work

Three failures account for nearly all of them.

### "DID NOT WORK" and something about the port being closed

**Means** — nothing is listening where it knocked. The server is running fine;
it just isn't accepting admin commands.

**Check first** — that `RCONPort` really is the number you typed. If your server
uses something other than 27020, typing the default here is the likeliest
mistake and costs nothing to rule out.

**Then** — if you just turned remote admin on, **did you restart the server?**
Or was the server running while you edited the file, so it overwrote your change
on shutdown? Between them those are the most common cause, and the message names
them.

**Also check** — `RCONEnabled=True` is actually present and underneath
`[ServerSettings]` rather than some other heading further down. See
[If remote admin is off](#if-remote-admin-is-off).

**Not the cause** — the port players connect on. That's a different port and
it's supposed to refuse this.

### "DID NOT WORK" and something about the password

**Means** — it reached your server, your server answered, and it rejected the
password. Everything else is set up correctly, which is most of the way there.

**Usually** — a typo, or a trailing space copied out of the `.ini` file. Nothing
shows on screen while you type, so there's no way to spot it as you go — just
run the program again and retype it slowly.

**Also check** — you're using `ServerAdminPassword`, not your server's join
password and not your own account password. They're three different things.

### Nothing happens — a window flashes and vanishes

**Means** — **almost always that it's working.** Once your settings are saved,
the program stops asking questions: it checks the count, sends it, and closes.
On a fast machine that's under a second.

**Confirm it** — open `asa-pop.log` in the same folder. If there's a line
stamped with the last minute or two, that flash was the whole job done
correctly.

**If the log is empty** — the program can't write next to itself. Move the
folder out of `Program Files`, `Downloads`, or anywhere synced by OneDrive, and
try again. `C:\asa-pop` is a safe choice.

---

Anything else, or the same thing twice: send us the last ten lines of
`asa-pop.log`. That file says what happened and when, and it's far quicker than
describing it.

Anything in here that didn't match what you saw on screen — tell us. A step that
reads fine and doesn't work is our bug, not yours.

---

## If someone asks what it is

Paste this:

> **asa-pop** is a tiny program for your server's machine that puts a live
> player count on your Dino Destination listing.
>
> Every 5 minutes it asks your own server how many people are on and sends just
> that number — one figure, nothing else. No logs, no player names, no files.
> It's about 5 MB, and the source code is public if you want to look at it.
>
> Setup is about 10 minutes and usually changes nothing on your server — it
> works through RCON, which you already have on if you use any admin tool. You
> look up your admin password, run the program once, and double-click one file.
> The only catch is that the server has to run on a Windows machine you can log
> into — if someone else hosts it for you, this won't work and we'll sort
> something else out.
>
> Why bother: a listing with a live count gets found. The site can't rank by
> current players alone and never will, but "4 online now" is the difference
> between someone joining and someone scrolling past.
>
> Setup guide: https://github.com/DinoDestination/asa-pop/blob/main/SETUP.md
