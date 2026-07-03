# Example configuration files

Drop-in examples for the optional Phase-1 power features. Point the matching
server flag at a copy you've edited:

```bash
./lookout-server \
  --health-config examples/health-config.json \
  --checks        examples/checks.json \
  --plugins       examples/plugins.json
```

| File | Flag | What it configures |
| --- | --- | --- |
| `health-config.json` | `--health-config` | Per-host / per-group thresholds and watched services. Applied and persisted on boot. A `0` or omitted value **inherits** the built-in default (it does not disable the signal). CPU and load alerting are ON by default; the seeded rule's 2-observation flap window keeps a single-sample spike from paging. |
| `checks.json` | `--checks` | TCP / HTTP probes run on the sweeper cadence; failures feed the same health → alert pipeline as host reports. |
| `plugins.json` | `--plugins` | Nagios-convention custom checks (exit `0` ok / `1` warning / `2` critical / `3` unknown). `Command` is the allow-listed **base name** of the executable — never a path — and `Args` are passed verbatim as argv with no shell. |

The `_comment` keys are ignored by the loader; keep or delete them. See
[`docs/manual/configuration.md`](../docs/manual/configuration.md) for the full
field reference.
