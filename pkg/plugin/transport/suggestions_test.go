package transport_test

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/arm/topo/pkg/plugin/observations"
	"github.com/arm/topo/pkg/plugin/suggestions"
	"github.com/arm/topo/pkg/plugin/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestSuggestionsRPCPlugin(t *testing.T) {
	for _, method := range []string{"Suggestions", "RunAction"} {
		t.Run(method, func(t *testing.T) {
			requests := make(chan suggestions.Request, 1)
			provider := newSuggestionsRPCClient(t, observationConsumer{requests: requests})
			request := suggestions.Request{
				Project:      suggestions.ProjectMetadata{Name: "Image segmentation"},
				Hints:        suggestions.Hints{"model": json.RawMessage(`{"id":9007199254740993}`)},
				CurrentValue: new(string),
				TargetObservations: observations.Tree{
					"arm": observations.Tree{"fancy": observations.Tree{
						"target": observations.Tree{"cpu_count": int64(9007199254740993)},
					}},
				}}

			var got []suggestions.Suggestion
			var err error
			if method == "Suggestions" {
				response, callErr := provider.Suggestions(context.Background(), request)
				got, err = response.Suggestions, callErr
			} else {
				response, callErr := provider.RunAction(context.Background(), "use-count", request)
				got, err = response.Suggestions, callErr
			}

			require.NoError(t, err)
			assert.Equal(t, []suggestions.Suggestion{{Value: "9007199254740993"}}, got)
			received := <-requests
			assert.Equal(t, request.Project, received.Project)
			assert.Equal(t, request.Hints, received.Hints)
			assert.Equal(t, request.CurrentValue, received.CurrentValue)
		})
	}
}

func newSuggestionsRPCClient(t *testing.T, impl suggestions.Provider) suggestions.Provider {
	t.Helper()
	adapter := &transport.SuggestionsRPCPlugin{Impl: impl}
	server := grpc.NewServer()
	require.NoError(t, adapter.GRPCServer(nil, server))
	listener := bufconn.Listen(1024 * 1024)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.DialContext(t.Context(), "passthrough:///plugin",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client, err := adapter.GRPCClient(t.Context(), nil, conn)
	require.NoError(t, err)
	return client.(suggestions.Provider)
}

func TestSuggestionsDeadline(t *testing.T) {
	provider := newSuggestionsRPCClient(t, observationConsumer{requests: make(chan suggestions.Request, 1)})
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()

	_, err := provider.Suggestions(ctx, suggestions.Request{})

	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSuggestionsActionCancellation(t *testing.T) {
	impl := &cancellableSuggestions{
		started: make(chan struct{}), cancelled: make(chan error, 1), release: make(chan struct{}),
	}
	defer close(impl.release)
	provider := newSuggestionsRPCClient(t, impl)
	ctx, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	action, cancel := context.WithCancel(ctx)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := provider.RunAction(action, "wait", suggestions.Request{})
		result <- err
	}()
	select {
	case <-impl.started:
	case <-ctx.Done():
		t.Fatal("action did not start")
	}

	cancel()

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-ctx.Done():
		t.Fatal("client waited for provider cleanup")
	}
	select {
	case err := <-impl.cancelled:
		require.ErrorIs(t, err, context.Canceled)
	case <-ctx.Done():
		t.Fatal("provider did not receive cancellation")
	}
	reply, err := provider.Suggestions(ctx, suggestions.Request{})
	require.NoError(t, err)
	assert.Equal(t, suggestions.Response{AvailableActions: []suggestions.Action{{ID: "wait"}}}, reply)
}

type cancellableSuggestions struct {
	started   chan struct{}
	cancelled chan error
	release   chan struct{}
}

func (*cancellableSuggestions) Suggestions(context.Context, suggestions.Request) (suggestions.Response, error) {
	return suggestions.Response{AvailableActions: []suggestions.Action{{ID: "wait"}}}, nil
}

func (p *cancellableSuggestions) RunAction(
	ctx context.Context, _ suggestions.ActionID, _ suggestions.Request,
) (suggestions.ActionResponse, error) {
	close(p.started)
	<-ctx.Done()
	p.cancelled <- ctx.Err()
	<-p.release
	return suggestions.ActionResponse{}, ctx.Err()
}

type observationConsumer struct {
	requests chan suggestions.Request
}

func (p observationConsumer) Suggestions(_ context.Context, request suggestions.Request) (suggestions.Response, error) {
	p.requests <- request
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

func (p observationConsumer) RunAction(
	_ context.Context, _ suggestions.ActionID, request suggestions.Request,
) (suggestions.ActionResponse, error) {
	p.requests <- request
	var count int64
	if err := request.TargetObservations.Decode("arm.fancy.target.cpu_count", &count); err != nil {
		return suggestions.ActionResponse{}, err
	}
	return suggestions.ActionResponse{Suggestions: []suggestions.Suggestion{
		{Value: strconv.FormatInt(count, 10)},
	}}, nil
}
