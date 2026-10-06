# FLClashX CLI

A terminal VPN client for Linux, built on the [mihomo](https://github.com/MetaCubeX/mihomo) core, the same engine inside [FlClashX](https://github.com/pluralplay/FlClashX), just without the window.

Client runs in the background, you talk to it with a few short commands, and that's it. No window, no tray icon, nothing to babysit.

It reads clash/mihomo YAML subscriptions, and plain or base64 lists of `vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://` links.

## Install

There are two ways, and they don't depend on each other. Pick whichever you like.

### Prebuilt binaries

Download `flc-<version>-linux-amd64.tar.gz` from [Releases](https://github.com/Qwarwin4/FLClashX-CLI/releases) and put the two binaries in place:

```sh
tar xzf flc-v0.1.0-linux-amd64.tar.gz
cd flc-v0.1.0-linux-amd64
sudo install -Dm755 flc /usr/local/bin/flc
sudo install -Dm755 flc-core /usr/local/lib/flc/flc-core
sudo setcap cap_net_admin,cap_net_bind_service=+ep /usr/local/lib/flc/flc-core
```

`setcap` comes with `libcap` on Arch and `libcap2-bin` on Debian/Ubuntu. You don't need Go for this.

flc looks for the core either in `../lib/flc/` relative to itself or right next to it. So keeping both files in one folder, say `~/.local/bin`, works just as well. Just run `setcap` on that `flc-core`.

### From source

```sh
git clone https://github.com/Qwarwin4/flc
cd flc
./install.sh
```

Run it as your normal user, it asks for root when it needs it. The script:

- installs Go and `setcap` if they're missing (pacman, apt, dnf, zypper, xbps and apk);
- lets an older Go (1.21–1.23) fetch 1.24 by itself;
- checks Go modules against `sum.golang.org`;
- builds both binaries and puts them in `/usr/local`.

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
| `flc status` | what's running: profile, server, mode, TUN |
| `flc server-list` | servers in the active profile; `*` is the current one, latency on the right |
| `flc switch-server <server>` | switch server by full name, part of it (`germ`) or number from the list; if the VPN is up, it restarts on the new server right away |
| `flc profile-list` | your profiles with traffic used and expiry date |
| `flc switch-profile <profile>` | switch profile; restarts the VPN if it's running |
| `flc add-profile --link <url>` | add a profile from a subscription link |
| `flc add-profile --file <path>` | add a profile from a file |
| `flc update [profile]` | re-download a subscription |
| `flc change-name <name>` | rename the active profile |
| `flc config [--profile <name>]` | open a profile's YAML in nano |
| `flc change-mode <rules\|global\|direct>` | routing mode |
| `flc delete-profile <profile>` | delete a profile |
| `flc debug log [-n 200] [-f]` | core log; `-f` follows it live |
| `flc lang <ru\|en>` | interface language; without an argument, shows the current one |
| `flc check-updates` | look for a new flc release and install it if you say yes |
| `flc delete` | remove flc completely: data and binaries |
| `flc version` | version |

A few details:

- `add-profile` also takes `--name <name>` and `--allow-http` (plain http links are refused by default).
- `delete-profile` and `delete` take `-y` to skip the confirmation.
- Names with spaces don't need quotes: `flc switch-profile Work VPN` works.

**Modes:**

- `rules` follows the subscription's rules, so local sites usually go direct.
- `global` sends everything through the VPN.
- `direct` sends nothing through it.

`change-mode` applies right away if the VPN is up, and is remembered for next time. In `global`, flc uses the same server you picked in `rules`, instead of mihomo's default `DIRECT`, which would quietly turn the VPN off.

**Language:** flc speaks English and Russian. On first run it picks one from your locale; `flc lang` changes it for good.

## Settings

`~/.config/flc/settings.yaml` is created on first run:

```yaml
tun: true            # all traffic through the VPN; false = only a proxy on 127.0.0.1:mixed-port
tun-stack: mixed     # system | gvisor | mixed
mixed-port: 7890
mode: rule           # rule | global | direct, same as flc change-mode
log-level: info      # silent | error | warning | info | debug
user-agent: clash.meta
send-hwid: true      # x-hwid for panels that limit devices (a hash of machine-id, not the id itself)
```

Changes apply on the next `flc start` or `flc restart`.

## Where things live

| Path | What |
|---|---|
| `~/.config/flc/profiles/` | profiles |
| `~/.config/flc/state.json` | profile list and selected servers |
| `~/.config/flc/settings.yaml` | settings |
| `~/.config/flc/lang` | interface language |
| `~/.local/share/flc/core/` | geo databases and core cache |
| `~/.local/state/flc/core.log` | log (`core.log.1` is the previous run) |
| `$XDG_RUNTIME_DIR/flc/` | control socket and pid while the VPN is up |

## Troubleshooting

`flc debug log` is the first place to look.

**`REALITY authentication failed`.** The server didn't accept the handshake. Check, in this order:
1. Your clock: run `timedatectl`, and if it isn't synchronized, run `sudo timedatectl set-ntp true`. This matters most in VMs.
2. Whether the same server works in another client.
3. Whether your panel serves a different config to a different `user-agent`.

**`TUN didn't come up`.** Run `getcap` on `flc-core`. If the caps are missing, run `setcap` again.

**Not sure if traffic goes through the VPN?** Run `curl https://ifconfig.me`. It should show the server's IP, not yours.

## Security

A VPN client handles your credentials and your network, so flc is careful about both.

- **Least privilege.** The CLI runs as you and refuses to run as root.
  - Only the core gets extra rights, and only the two TUN needs: `CAP_NET_ADMIN` and `CAP_NET_BIND_SERVICE`.
  - The core sets `no_new_privs`, refuses root and gets a minimal environment.
  - Any local user can start the core, and with it TUN routes for the whole machine. So flc is meant for a personal computer, not a shared server.
- **No network-facing control.** The core is driven through a unix socket in a `0700` directory. There's no TCP controller, no web dashboard and no secret to leak.
- **Profiles can't open doors.** Anything that would open ports or touch the system is stripped from the profile: `external-controller*`, `external-ui*`, `listeners`, `tunnels`, `iptables`, `tuic-server`, `allow-lan`, `dns.listen`, `ntp.write-to-system` and friends. After parsing, the core switches all of it off again. The local proxy only listens on `127.0.0.1`.
- **Private files.** Profiles, settings and logs are `0600`, directories `0700`. The generated runtime config, which holds your keys, is deleted on stop.
- **Careful downloads.**
  - Subscriptions only go over https (TLS 1.2+), with no https→http redirects, a size cap and a timeout.
  - The token in your link never shows up in error messages.
- **Safe output.** Server names, release notes and log lines are stripped of control characters, so a malicious subscription can't inject escape sequences into your terminal.
- **No half-written files.** `config` edits a copy and only saves valid YAML. All writes are atomic, and concurrent commands wait for each other.
- **Tight process handling.** `stop` checks the pid against `/proc`, so it never kills an unrelated process that happens to reuse the pid.
- **Careful updates.**
  - Prebuilt archives are checked against the release's `SHA256SUMS`.
  - Only files that belong to you or root and that nobody else can write to get copied.
  - Root is asked for only where it has to be: writing into a root-owned folder, `setcap`, and removing the binaries on `flc delete`.

## Updating

```sh
flc check-updates
```

It asks GitHub for the latest release, shows what changed and waits for a `y` before touching anything. On yes it:

1. downloads the prebuilt archive;
2. checks it against `SHA256SUMS`;
3. swaps `flc` and `flc-core` right where they are, whichever way you installed them.

If the release has no build for your machine, it builds that tag from source instead.

If anything fails, the old version stays put. If the VPN was running, it offers to restart it on the new version.

The update only talks to GitHub over https, refuses redirects to anywhere else, and unpacks into a private temp folder where no path can escape.

## Making a release

```sh
./release.sh v0.1.0
gh release create v0.1.0 dist/* --title v0.1.0
```

`release.sh` puts into `dist/`:
- `flc-<tag>-linux-amd64.tar.gz`
- `SHA256SUMS`

These are exactly the names `flc check-updates` looks for. Commit `go.sum` before tagging.

## Uninstall

```sh
flc delete
```

It stops the VPN, wipes your profiles, settings and logs, then removes `flc` and `flc-core`, however they were installed. Your sudo password is only needed if they live somewhere root-owned like `/usr/local`.

Other users' data on the same machine is left alone; each of them runs `flc delete` for their own.

## License

GPL-3.0, same as mihomo, which is built into `flc-core`.
