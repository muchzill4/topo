# Experimental Go plugins

`topo run-plugins` runs local plugins against a selected target and prints JSON
facts. Plugins do not affect built-in probing, hardware output or catalogue matching.

## Build and run

From the Topo checkout, build the [sample plugins](../../pkg/plugin/examples)
into `pkg/plugin/examples/bin/` and run them:

```sh
./pkg/plugin/examples/build-plugins.sh
go run ./cmd/topo run-plugins \
  --plugin-dir ./pkg/plugin/examples/bin --target user@host --timeout 30s
```

For installed plugins, copy or symlink `topo-plugin-*` executables into the
platform's default directory:

| Host    | Directory                                        |
| ------- | ------------------------------------------------ |
| Linux   | `${XDG_CONFIG_HOME:-$HOME/.config}/topo/plugins` |
| macOS   | `$HOME/Library/Application Support/topo/plugins` |
| Windows | `%AppData%\topo\plugins`                         |

Windows binaries must end in `.exe`. Plugin binaries are not included in Topo releases.

```sh
topo run-plugins --target user@host
```

- `--plugin-dir` replaces the default directory. Topo does not search `PATH`.
- The default timeout is five seconds; `--timeout 0` disables it.
- Plugins run sequentially in filename order. Any failure stops the command.

## Write a plugin

Use the public Go API in [`pkg/plugin`](../../pkg/plugin):

1. Implement `Plugin.Describe` to return metadata without probing. Give each
   plugin a nonempty, unique name.
2. To provide facts, implement `FactsProviderPlugin` and advertise
   `Contracts: []string{plugin.FactsContract}` (`facts.v1`). Plugins without this
   contract are skipped by `run-plugins`.
3. Return `FactsResult` containing `FactGroup` entries. Group IDs must be nonempty
   and unique within the result. Fact keys must be nonempty and unique within
   each group; values are booleans, integers or strings.
4. Use the supplied `TargetRunner` to execute commands on the selected target.
   Topo owns target selection, SSH credentials and execution.

## Trust boundary

Plugins run as local processes with your permissions and arbitrary target command
access. Install only trusted plugins: they are not sandboxed. Topo stops plugin
processes when the command returns.
