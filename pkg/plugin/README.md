# Configuration actions

Interactive configuration discovers installed `topo-plugin-*` executables in the
`topo/plugins` subdirectory of the OS user configuration directory. Use
`topo configure --plugin-dir /path/to/plugins` to override it (`clone` also supports
this flag). Non-interactive configuration does not load plugins.

Suggestions providers receive project metadata in `Request.Project` (currently
`Name`, taken from `x-topo.name`), the parameter's advisory `hints`, current value,
and plugin observations when a target is supplied. The same project metadata is
included in `RunAction` requests. If `x-topo.name` is absent, the name is empty.
Available actions appear in the prompt footer, starting with `Ctrl+O`. Actions
run only when explicitly selected and receive a fresh snapshot of the edited value.

Actions run in the background: typing, Enter, and Ctrl+C remain available. The
first returned suggestion fills the input for editing and confirmation. Errors
are displayed without ending the prompt. Accepting a value or cancelling the
prompt cancels its outstanding actions without waiting for cleanup. Providers
must honor their request context and release resources before returning. Cleanup
can overlap later calls to the same plugin.

## Transport compatibility

The experimental v1 wire protocol now uses gRPC. Rebuild plugin executables
built with the older `net/rpc` SDK; Topo no longer loads them.
