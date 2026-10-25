package plugin

import (
	"context"
	"fmt"
	"net/rpc"
)

// TargetRunner executes on the target selected by Topo, not on the plugin's host.
type TargetRunner interface {
	Run(context.Context, string) (stdout, stderr string, err error)
}

type runnerClient struct {
	client *rpc.Client
}

// RunReply carries execution errors as data so stdout and stderr are preserved.
type RunReply struct {
	Stdout string
	Stderr string
	Error  string
}

func (r *runnerClient) Run(ctx context.Context, command string) (string, string, error) {
	var result RunReply
	if err := call(ctx, r.client, "Plugin.Run", command, &result); err != nil {
		return "", "", err
	}
	if result.Error != "" {
		return result.Stdout, result.Stderr, fmt.Errorf("%s", result.Error)
	}
	return result.Stdout, result.Stderr, nil
}

type runnerServer struct {
	ctx  context.Context
	impl TargetRunner
}

func (r *runnerServer) Run(command string, reply *RunReply) error {
	stdout, stderr, err := r.impl.Run(r.ctx, command)
	*reply = RunReply{Stdout: stdout, Stderr: stderr}
	if err != nil {
		reply.Error = err.Error()
	}
	return nil
}
