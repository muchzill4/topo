# Configuration actions

Interactive configuration discovers installed `topo-plugin-*` executables in the
`topo/plugins` subdirectory of the OS user configuration directory. Use
`topo configure --plugin-dir /path/to/plugins` to override it (`clone` also supports
this flag). Non-interactive configuration does not load plugins.

Suggestions providers receive the parameter's advisory `hints`, current value, and
plugin observations when a target is supplied. Available actions appear in the
prompt footer, starting with `Ctrl+O`. Actions run only when explicitly selected.
The action receives a fresh snapshot of the edited value. The AI Portal example
only offers model selection for parameters with a nonempty `model.task` or
`model.format` string, or `huggingface.repo-id` in their `meta.type` hint (a string
or array). It does not offer that action for unrelated parameters.

Actions run in the background: typing, Enter, and Ctrl+C remain available. The
first returned suggestion fills the input for editing and confirmation. Errors
are displayed without ending the prompt. Accepting a value or cancelling the
prompt cancels its outstanding actions and waits for cleanup. Closing an external
browser alone does not end an action; the user can still enter a value manually.

`RunAction` implementations must observe their context and release resources
before returning. The AI Portal example closes its local callback server on
cancellation. A plugin that does not finish cancellation within two seconds loses
its RPC connection.

## Transport compatibility

The experimental transport now includes `BeginAction` and `CancelAction` RPCs and
a per-invocation ID on `RunAction`. Registration happens before execution so
cancellation cannot race ahead of registration. Completion of `RunAction`
acknowledges cleanup. The protocol version remains unchanged; rebuild installed
plugins against this SDK. The public `suggestions.Provider` interface is unchanged.
