package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/arm/topo/internal/deploy/command"
	"github.com/arm/topo/internal/deploy/operation"
	"github.com/arm/topo/internal/deploy/post_deploy"
	goperation "github.com/arm/topo/internal/operation"
	"github.com/arm/topo/internal/ssh"
)

type RegistryConfig struct {
	Port                string
	SkipRemotePortCheck bool
	UseControlSockets   bool
}

type DeployOptions struct {
	RecreateMode operation.RecreateMode
	TargetHost   ssh.Destination
	Registry     *RegistryConfig
}

type Deployment struct {
	composeFile string
	opts        DeployOptions
}

func SupportsRegistry(noRegistry bool, dest ssh.Destination) bool {
	return !noRegistry && !dest.IsPlainLocalhost()
}

func SupportsSSHControlSockets(goos string) bool {
	return goos != "windows"
}

func NewDeploymentStop(composeFile string, dest ssh.Destination) goperation.Sequence {
	return goperation.Sequence{operation.NewDockerComposeStop(composeFile, command.NewHostFromDestination(dest))}
}

func NewDeployment(composeFile string, opts DeployOptions) *Deployment {
	return &Deployment{composeFile: composeFile, opts: opts}
}

func (d *Deployment) Run(ctx context.Context, w io.Writer) error {
	sourceHost := command.LocalHost
	targetHost := command.NewHostFromDestination(d.opts.TargetHost)

	if err := goperation.NewSequence(
		operation.NewDockerComposeBuild(d.composeFile, sourceHost),
		operation.NewDockerComposePull(d.composeFile, sourceHost),
	).Run(ctx, w); err != nil {
		return err
	}

	if !d.opts.TargetHost.IsPlainLocalhost() {
		if d.opts.Registry != nil {
			if err := d.runRegistryTransfer(ctx, w, sourceHost, targetHost); err != nil {
				return err
			}
		} else {
			if err := goperation.Run(ctx, w, operation.NewDockerComposePipeTransfer(d.composeFile, sourceHost, targetHost)); err != nil {
				return err
			}
		}
	}

	return goperation.NewSequence(
		operation.NewDockerComposeUp(d.composeFile, targetHost, d.opts.RecreateMode),
		post_deploy.NewDeploySuccess(d.composeFile, targetHost, post_deploy.DefaultMessage(d.composeFile)),
	).Run(ctx, w)
}

func (d *Deployment) runRegistryTransfer(ctx context.Context, w io.Writer, sourceHost command.Host, targetHost command.Host) (err error) {
	const tunnelCleanupTimeout = 10 * time.Second

	registry := d.opts.Registry
	if err := operation.NewRunRegistry(registry.Port).Run(ctx, w); err != nil {
		return err
	}

	startTunnel, stopTunnel := ssh.NewSSHTunnel(d.opts.TargetHost, registry.Port, registry.UseControlSockets)
	tunnelStarted := false
	defer func() {
		if !tunnelStarted {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), tunnelCleanupTimeout)
		defer cancel()
		if cleanupErr := goperation.Run(cleanupCtx, w, stopTunnel); cleanupErr != nil {
			if err == nil {
				err = cleanupErr
				return
			}
			err = errors.Join(err, fmt.Errorf("cleanup failed: %w", cleanupErr))
		}
	}()

	if err := goperation.Run(ctx, w, startTunnel); err != nil {
		return err
	}
	tunnelStarted = true
	if !registry.SkipRemotePortCheck {
		if err := goperation.Run(ctx, w, operation.NewRegistryTunnelExposureCheck(d.opts.TargetHost, registry.Port)); err != nil {
			return err
		}
	}
	if err := goperation.Run(ctx, w, operation.NewRegistryTransfer(d.composeFile, sourceHost, targetHost, registry.Port)); err != nil {
		return err
	}
	return nil
}
