package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/arm/topo/pkg/plugin/observations"
	"github.com/arm/topo/pkg/plugin/transport/pluginpb"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type ObservationsRPCPlugin struct {
	goplugin.NetRPCUnsupportedPlugin
	Impl observations.Provider
}

func (p *ObservationsRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, server *grpc.Server) error {
	pluginpb.RegisterObservationsServiceServer(server, &observationsServer{impl: p.Impl, broker: broker})
	return nil
}

func (p *ObservationsRPCPlugin) GRPCClient(
	_ context.Context, broker *goplugin.GRPCBroker, conn *grpc.ClientConn,
) (any, error) {
	return &observationsClient{rpc: pluginpb.NewObservationsServiceClient(conn), broker: broker}, nil
}

type observationsClient struct {
	rpc    pluginpb.ObservationsServiceClient
	broker *goplugin.GRPCBroker
}

func (p *observationsClient) ObserveTarget(
	ctx context.Context, runner observations.TargetRunner,
) (observations.Tree, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	callback, cancel := context.WithCancel(ctx)
	defer cancel()
	id := p.broker.NextId()
	go p.broker.AcceptAndServe(id, func(options []grpc.ServerOption) *grpc.Server {
		server := grpc.NewServer(options...)
		pluginpb.RegisterTargetRunnerServiceServer(server, &runnerServer{impl: runner})
		// End this observation's reverse channel without closing the plugin connection.
		context.AfterFunc(callback, server.Stop)
		return server
	})

	reply, err := p.rpc.ObserveTarget(ctx, &pluginpb.ObservationRequest{RunnerBrokerId: id})
	if err != nil {
		return nil, callError(ctx, err)
	}
	return deserialiseObservations(reply.GetTree())
}

type observationsServer struct {
	pluginpb.UnimplementedObservationsServiceServer
	impl   observations.Provider
	broker *goplugin.GRPCBroker
}

func (p *observationsServer) ObserveTarget(
	ctx context.Context, request *pluginpb.ObservationRequest,
) (_ *pluginpb.ObservationResponse, err error) {
	conn, err := p.broker.Dial(request.GetRunnerBrokerId())
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, conn.Close()) }()

	result, err := p.impl.ObserveTarget(ctx, &runnerClient{rpc: pluginpb.NewTargetRunnerServiceClient(conn)})
	if err != nil {
		return nil, err
	}
	tree, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("serialising observations: %w", err)
	}
	return &pluginpb.ObservationResponse{Tree: tree}, nil
}

func deserialiseObservations(data []byte) (observations.Tree, error) {
	var result observations.Tree
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("deserialising observations: %w", err)
	}
	return result, nil
}

type runnerClient struct {
	rpc pluginpb.TargetRunnerServiceClient
}

func (r *runnerClient) Run(ctx context.Context, command string) (string, string, error) {
	reply, err := r.rpc.Run(ctx, &pluginpb.RunRequest{Command: command})
	if err != nil {
		return "", "", callError(ctx, err)
	}
	var commandErr error
	if len(reply.GetError()) != 0 {
		commandErr = errors.New(string(reply.GetError()))
	}
	return string(reply.GetStdout()), string(reply.GetStderr()), commandErr
}

type runnerServer struct {
	pluginpb.UnimplementedTargetRunnerServiceServer
	impl observations.TargetRunner
}

func (r *runnerServer) Run(ctx context.Context, request *pluginpb.RunRequest) (*pluginpb.RunResponse, error) {
	stdout, stderr, err := r.impl.Run(ctx, request.GetCommand())
	reply := &pluginpb.RunResponse{Stdout: []byte(stdout), Stderr: []byte(stderr)}
	if err != nil {
		reply.Error = []byte(err.Error())
	}
	return reply, nil
}
