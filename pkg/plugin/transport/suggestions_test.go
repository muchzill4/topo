package transport_test

import (
	"context"
	"net"
	"net/rpc"
	"strconv"
	"testing"

	"github.com/arm/topo/pkg/plugin/observations"
	"github.com/arm/topo/pkg/plugin/suggestions"
	"github.com/arm/topo/pkg/plugin/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuggestionsRPCPlugin(t *testing.T) {
	for _, method := range []string{"Suggestions", "RunAction"} {
		t.Run(method, func(t *testing.T) {
			adapter := &transport.SuggestionsRPCPlugin{Impl: observationConsumer{}}
			server, err := adapter.Server(nil)
			require.NoError(t, err)
			rpcServer := rpc.NewServer()
			require.NoError(t, rpcServer.RegisterName("Plugin", server))
			clientConnection, serverConnection := net.Pipe()
			defer serverConnection.Close()
			go rpcServer.ServeConn(serverConnection)
			rpcClient := rpc.NewClient(clientConnection)
			defer rpcClient.Close()
			client, err := adapter.Client(nil, rpcClient)
			require.NoError(t, err)
			provider := client.(suggestions.Provider)
			request := suggestions.Request{TargetObservations: observations.Tree{
				"arm": observations.Tree{"fancy": observations.Tree{
					"target": observations.Tree{"cpu_count": int64(9007199254740993)},
				}},
			}}

			var got []suggestions.Suggestion
			if method == "Suggestions" {
				response, callErr := provider.Suggestions(context.Background(), request)
				got, err = response.Suggestions, callErr
			} else {
				response, callErr := provider.RunAction(context.Background(), "use-count", request)
				got, err = response.Suggestions, callErr
			}

			require.NoError(t, err)
			assert.Equal(t, []suggestions.Suggestion{{Value: "9007199254740993"}}, got)
		})
	}
}

type observationConsumer struct{}

func (observationConsumer) Suggestions(_ context.Context, request suggestions.Request) (suggestions.Response, error) {
	var data struct {
		CPUCount int64 `json:"cpu_count"`
	}
	if err := request.TargetObservations.Decode("arm.fancy.target", &data); err != nil {
		return suggestions.Response{}, err
	}
	return suggestions.Response{Suggestions: []suggestions.Suggestion{
		{Value: strconv.FormatInt(data.CPUCount, 10)},
	}}, nil
}

func (observationConsumer) RunAction(
	_ context.Context, _ suggestions.ActionID, request suggestions.Request,
) (suggestions.ActionResponse, error) {
	var count int64
	if err := request.TargetObservations.Decode("arm.fancy.target.cpu_count", &count); err != nil {
		return suggestions.ActionResponse{}, err
	}
	return suggestions.ActionResponse{Suggestions: []suggestions.Suggestion{
		{Value: strconv.FormatInt(count, 10)},
	}}, nil
}
