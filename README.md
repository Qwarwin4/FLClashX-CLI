# FLClashX CLI

A terminal VPN client for Linux, built on the [mihomo](https://github.com/MetaCubeX/mihomo) core, the same engine inside [FlClashX](https://github.com/pluralplay/FlClashX), just without the window.

Client runs in the background, you talk to it with a few short commands, and that's it. No window, no tray icon, nothing to babysit.

It reads clash/mihomo YAML subscriptions, and plain or base64 lists of `vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://` links.

## Install

There are two ways, and they don't depend on each other. Pick whichever you like.

### Prebuilt binaries

Download `FLClashX-CLI-<version>-linux-amd64.tar.gz` from [Releases](https://github.com/Qwarwin4/FLClashX-CLI/releases), unpack it and run the installer that comes inside:

```sh
tar xzf FLClashX-CLI-v0.3.0-linux-amd64.tar.gz
cd FLClashX-CLI-v0.3.0-linux-amd64
./install.sh
```

Run it as your normal user, it asks for root when it needs it. You don't need Go for this. If `setcap` is missing, the script installs it (`libcap` on Arch, `libcap2-bin` on Debian/Ubuntu).

Prefer to do it by hand? The archive has a single binary, and it goes in twice: once as the command you type, once as the core that gets the network rights.

```sh
sudo install -Dm755 flc /usr/local/bin/flc
sudo install -Dm755 flc /usr/local/lib/flc/flc-core
sudo setcap cap_net_admin,cap_net_bind_service=+ep /usr/local/lib/flc/flc-core
```

Make it a real copy, not a link, so only `flc-core` gets those rights. FLClashX CLI looks for the core either in `../lib/flc/` relative to itself or right next to it, so keeping both in one folder, say `~/.local/bin`, works just as well.

### From source

```sh
git clone https://github.com/Qwarwin4/FLClashX-CLI
cd FLClashX-CLI
./install.sh
```

Run it as your normal user, it asks for root when it needs it. The script:

- installs Go and `setcap` if they're missing (pacman, apt, dnf, zypper, xbps and apk);
- lets an older Go (1.21–1.23) fetch 1.24 by itself;
- checks Go modules against `sum.golang.org`;
- builds FLClashX CLI and puts it in `/usr/local`.

Want a different place? `PREFIX=/opt/flc ./install.sh`.

## Quick start

```sh
flc add-profile --link https://panel.example.com/sub/xxxx
flc start
flc server-list
flc switch-server 3
flc stop
```

The VPN keeps running after you close the terminal. To bring it up on login, add `flc start` to your session's autostart.

If you want a single command to toggle it, for a hotkey for example:

```sh
pgrep -x flc-core >/dev/null && flc stop || flc start
```

## Commands

| Command | What it does |
|---|---|
| `flc start` | start the VPN |
| `flc stop` | stop it |
| `flc restart` | restart it, or just start it if it isn't running |
| `flc status` | what's running: uptime since `flc start`, profile, server, mode, TUN |
| `flc server-list` | servers in the active profile; `*` is the current one, latency on the right |
| `flc switch-server <server>` | switch server by full name, part of it (`germ`) or number from the list; if the VPN is up, it restarts on the new server right away |
| `flc profile-list` | your profiles with traffic used and expiry date |
| `flc switch-profile <profile>` | switch profile; restarts the VPN if it's running |
| `flc servergr-name` | name of the active profile's server group, e.g. `Proxy` |
| `flc servergr-name --profile <profile>` | the same for a given profile |
| `flc servergr-name --all-profiles` | the group name of every profile; `*` marks the active one |
| `flc add-profile --link <url>` | add a profile from a subscription link |
| `flc add-profile --file <path>` | add a profile from a file |
| `flc update-profile [profile]` | re-download a subscription |
| `flc change-name <name>` | rename the active profile |
| `flc config [--profile <name>]` | open a profile's YAML in nano |
| `flc change-mode <rules\|global\|direct>` | routing mode |
| `flc delete-profile <profile>` | delete a profile |
| `flc delete-profile --all` | delete every profile; stop the VPN first |
| `flc debug log [-n 200] [-f]` | core log; `-f` follows it live |
| `flc settings` | interactive settings menu, keyboard and mouse |
| `flc lang <ru\|en>` | interface language; without an argument, shows the current one |
| `flc check-updates` | look for a new FLClashX CLI release and install it if you say yes |
| `flc delete` | remove FLClashX CLI completely: data and binaries |
| `flc version` | version plus info about the client: mihomo version, User-Agent, HWID, paths |
| `flc help` | all commands |

A few details:

- `add-profile` also takes `--name <name>` and `--allow-http` (plain http links are refused by default). The same subscription link can't be added twice; use `update-profile` to refresh it.
- `delete-profile` and `delete` take `-y` to skip the confirmation.
- Names with spaces don't need quotes: `flc switch-profile Work VPN` works.

**Modes:**

- `rules` follows the subscription's rules, so local sites usually go direct.
- `global` sends everything through the VPN.
- `direct` sends nothing through it.

`change-mode` applies right away if the VPN is up, and is remembered for next time. In `global`, FLClashX CLI uses the same server you picked in `rules`, instead of mihomo's default `DIRECT`, which would quietly turn the VPN off.

**Language:** FLClashX CLI speaks English and Russian. On first run it picks one from your locale; `flc lang` changes it for good.

## Settings

The easy way is `flc settings`: a menu right in the terminal with three sections, **General**, **Updates** and **Privacy**. Switch sections with Tab or a mouse click, move with the arrows, flip switches with Enter and save with `s`. If the VPN is running, most changes are applied on the fly, without a restart.

Under the hood it's all in `~/.config/flc/settings.yaml`, created on first run, which you can also edit by hand:

```yaml
tun: true               # all traffic through the VPN; false = only a proxy on 127.0.0.1:mixed-port
tun-stack: mixed        # system | gvisor | mixed
mixed-port: 7890
mode: rule              # rule | global | direct, same as flc change-mode
log-level: info         # silent | error | warning | info | debug
flags: auto             # country flags in server names: auto | emoji | text

update-interval: 0      # refresh the active subscription every N minutes while the VPN runs; 0 = never
keep-rules: false       # keep your own routing rules when a subscription is refreshed
check-updates: true     # once a day, look for a new FLClashX CLI version

hide-hwid: real         # real | fake | none, see Privacy below
hide-user-agent: false
hide-device: false
hide-os: false
hide-processes: false
wipe-log: false
```

**Subscription refresh.** With `update-interval` set (5 to 10080 minutes), FLClashX CLI refreshes the active subscription in the background while the VPN is up and hands it to the core on the fly: connections aren't dropped and your selected servers stay selected. `flc status` shows when the next refresh is due, and a log of refreshes goes to `~/.local/state/flc/watch.log`.

**Keeping your rules.** If you edit the rules of a profile in `flc config`, a refresh normally replaces them with the subscription's. With `keep-rules: true` your `rules`, `sub-rules` and `rule-providers` survive the refresh. If they point to a group that the new subscription no longer has, FLClashX CLI takes the subscription's rules instead and says why.

**New versions.** With `check-updates: true`, once a day while the VPN runs FLClashX CLI asks GitHub whether a new version is out. It never installs anything by itself: you get a desktop notification (if `notify-send` is around) and a hint under `flc status` and `flc version`. Installing is still `flc check-updates`.

**Country flags.** Server names often start with a flag like 🇩🇪. Some terminals, Alacritty among them, can't draw flags and show two letter boxes instead. With `flags: text` FLClashX CLI writes the country code in plain letters (`DE Germany`). `auto`, the default, keeps emoji in kitty, foot, WezTerm and Ghostty and when the output goes into a pipe, and uses letters everywhere else.

## Privacy

When FLClashX CLI downloads a subscription, it tells the panel a few things about itself. Panels use them to count devices, and their Telegram bots show them to whoever runs the panel. The **Privacy** section decides what goes out:

| Setting | Off (default) | On |
|---|---|---|
| `hide-hwid` | `real`: `x-hwid` is a hash of `/etc/machine-id`, never the id itself | `fake`: a stand-in HWID, different for each panel and not tied to this computer; device limits still work. `none`: no HWID at all, and then no model and OS either |
| `hide-user-agent` | `FLClashX CLI <version>/mihomo <version>` | `clash.meta`, the same as most clash clients send |
| `hide-device` | `x-device-model`: vendor and model from DMI, e.g. `LENOVO ThinkPad T14` | not sent |
| `hide-os` | `x-device-os: Linux` and `x-ver-os`, e.g. `Ubuntu 24.04` | not sent |
| `hide-processes` | the core looks up which app opened each connection and logs it | it doesn't, and `PROCESS-NAME` rules stop working |
| `wipe-log` | the core log and the previous run's log stay on disk | both are deleted on `flc stop`, along with `watch.log` |

`flc version` shows what's being sent right now. The hidden values also stay out of GitHub requests when checking for updates.

A panel keeps what it saw before. After switching `hide-hwid` to `fake` or `none`, the panel shows a new device or none at all, and the old entry stays in the bot until you delete it there.

## Where things live

| Path | What |
|---|---|
| `~/.config/flc/profiles/` | profiles |
| `~/.config/flc/state.json` | profile list and selected servers |
| `~/.config/flc/settings.yaml` | settings |
| `~/.config/flc/lang` | interface language |
| `~/.local/share/flc/core/` | geo databases and core cache |
| `~/.local/state/flc/core.log` | log (`core.log.1` is the previous run) |
| `~/.local/state/flc/watch.log` | background subscription refreshes and version checks |
| `~/.config/flc/hwid-key` | random key behind the fake HWID; delete it to get new fake HWIDs |
| `$XDG_RUNTIME_DIR/flc/` | control socket and pid while the VPN is up |

## Troubleshooting

`flc debug log` is the first place to look.

**`REALITY authentication failed`.** The server didn't accept the handshake. Check, in this order:
1. Your clock: run `timedatectl`, and if it isn't synchronized, run `sudo timedatectl set-ntp true`. This matters most in VMs.
2. Whether the same server works in another client.
3. Whether your panel serves a different config by User-Agent. FLClashX CLI sends `FLClashX CLI <version>/mihomo <version>`, `flc version` shows it.

**TUN didn't come up.** Run `getcap` on `flc-core`. If the caps are missing, run `setcap` again.

**Not sure if traffic goes through the VPN?** Run `curl https://ifconfig.me`. It should show the server's IP, not yours.

## Security

A VPN client handles your credentials and your network, so FLClashX CLI is careful about both.

- **Least privilege.** The CLI runs as you and refuses to run as root.
  - Only the core gets extra rights, and only the two TUN needs: `CAP_NET_ADMIN` and `CAP_NET_BIND_SERVICE`.
  - The core sets `no_new_privs`, refuses root and gets a minimal environment.
  - Any local user can start the core, and with it TUN routes for the whole machine. So FLClashX CLI is meant for a personal computer, not a shared server.
- **No network-facing control.** The core is driven through a unix socket in a `0700` directory. There's no TCP controller, no web dashboard and no secret to leak.
- **A locked control API.** Even through that socket, nothing can reconfigure the core: the mihomo endpoints that load a config, open ports, allow LAN access, restart or upgrade the core are switched off. FLClashX CLI only uses two endpoints of its own, one to change the routing mode and one to reload the config it wrote itself, and the reload goes through the same lockdown as a normal start.
- **Profiles can't open doors.** Anything that would open ports or touch the system is stripped from the profile: `external-controller*`, `external-ui*`, `listeners`, `tunnels`, `iptables`, `tuic-server`, `allow-lan`, `dns.listen`, `ntp.write-to-system` and friends. After parsing, the core switches all of it off again. The local proxy only listens on `127.0.0.1`.
- **Private files.** Profiles, settings and logs are `0600`, directories `0700`. The generated runtime config, which holds your keys, is deleted on stop.
- **Careful downloads.**
  - Subscriptions only go over https (TLS 1.2+), with no https→http redirects, a size cap and a timeout.
  - If a panel redirects to another host, the HWID and device headers are dropped, so they only ever reach the host in your link.
  - An https subscription can't pull its proxy or rule providers over plain http, where they could be swapped on the way, and its `geox-url` has to be https too.
  - The token in your link never shows up in error messages.
- **Safe output.** Server names, release notes and log lines are stripped of control characters, so a malicious subscription can't inject escape sequences into your terminal.
- **No half-written files.** `config` edits a copy and only saves valid YAML. All writes are atomic, and concurrent commands wait for each other.
- **Tight process handling.** `stop` checks the pid against `/proc`, so it never kills an unrelated process that happens to reuse the pid.
- **Careful updates.**
  - Updates are built from the tagged source on GitHub, nothing prebuilt is downloaded.
  - Only files that belong to you or root and that nobody else can write to get copied.
  - Root is asked for only where it has to be: writing into a root-owned folder, `setcap`, and removing the binaries on `flc delete`.

## Updating

```sh
flc check-updates
```

It asks GitHub for the latest release, shows what changed and waits for a `y` before touching anything. On yes it:

1. downloads the source of that release from GitHub;
2. builds it with the same `install.sh`, so you need Go (the installer puts it in if it's missing);
3. swaps `flc` and `flc-core` right where they are, whichever way you installed them.

If anything fails, the old version stays put. If the VPN was running, it offers to restart it on the new version.

The update only talks to GitHub over https, refuses redirects to anywhere else, and unpacks into a private temp folder where no path can escape.

## Making a release

Bump `version` in `src/main.go`, commit, then tag that commit and publish a release for the tag:

```sh
git tag v0.3.0
git push origin v0.3.0
gh release create v0.3.0 --title v0.3.0 --notes-file notes.md
```

No files need to be attached: `flc check-updates` builds the tag from source.

## Uninstall

```sh
flc delete
```

It stops the VPN, wipes your profiles, settings and logs, then removes `flc` and `flc-core`, however they were installed. Your sudo password is only needed if they live somewhere root-owned like `/usr/local`.

Other users' data on the same machine is left alone; each of them runs `flc delete` for their own.

## License

GPL-3.0, same as mihomo, which is built into FLClashX CLI.
