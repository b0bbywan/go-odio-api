# apt upgrades for odio-api

Drives odio-api's [upgrade backend](../../../README.md#software-upgrades) on a plain Debian, Ubuntu or Raspberry Pi OS, without odios.

- `odio-apt-upgrade check` refreshes the package lists and writes the result file: `current` is `/etc/debian_version`, `latest` the number of pending packages, listed under `extra.packages` in `GET /upgrade`.
- `odio-apt-upgrade upgrade` runs `apt upgrade` (new dependencies such as a kernel ABI bump included, nothing removed), streams its progress to odio-api, then rewrites the result file.

apt runs as root through a sudoers rule limited to the script's two root subcommands. Progress needs `socat`; without it, or without odio-api, the upgrade runs the same with no live progress.

## Install

As root, with `USER` the account running odio-api:

```sh
install -m 755 odio-apt-upgrade /usr/local/bin/
sed 's/^USER /alice /' sudoers > /etc/sudoers.d/odio-apt-upgrade
chmod 440 /etc/sudoers.d/odio-apt-upgrade && visudo -c
```

As `USER`:

```sh
install -Dm644 -t ~/.config/systemd/user odio-apt-check.service odio-apt-check.timer odio-apt-upgrade.service
systemctl --user daemon-reload
systemctl --user enable --now odio-apt-check.timer
```

Then in odio-api's config:

```yaml
upgrade:
  enabled: true
  resultFile: /home/alice/.cache/odio-api/upgrades.json
  checkUnit: odio-apt-check.service
  upgradeUnit: odio-apt-upgrade.service
```

## Notes

- The script must stay owned by root and not writable by `USER`: the sudoers rule trusts it.
- Debian does not restart user units: when odio-api itself is upgraded, the old binary keeps running until `systemctl --user restart odio-api`.
- With `unattended-upgrades` installed, a kernel upgrade raises `reboot_required` on `GET /power`.
