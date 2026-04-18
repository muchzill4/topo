package deploy

import (
	"github.com/arm/topo/internal/deploy/engine"
	"github.com/arm/topo/internal/deploy/operation"
	goperation "github.com/arm/topo/internal/operation"
	"github.com/arm/topo/internal/ssh"
)

type RegistryConfig struct {
	Port              string
	UseControlSockets bool
}

type DeployOptions struct {
	RecreateMode operation.RecreateMode
	TargetHost   ssh.Destination
	Registry     *RegistryConfig
}

func SupportsRegistry(noRegistry bool, dest ssh.Destination) bool {
	return !noRegistry && !dest.IsPlainLocalhost()
}

func SupportsSSHControlSockets(goos string) bool {
	return goos != "windows"
}

func NewDeploymentStop(composeFile string, dest ssh.Destination) goperation.Sequence {
	e := engine.Docker
	return goperation.Sequence{operation.NewComposeStop(e, composeFile, engine.NewHostFromDestination(dest))}
}

func NewDeployment(composeFile string, opts DeployOptions) (goperation.Sequence, goperation.Operation) {
	e := engine.Docker
	sourceHost := engine.LocalHost
	ops := []goperation.Operation{
		operation.NewComposeBuild(e, composeFile, sourceHost),
		operation.NewComposePull(e, composeFile, sourceHost),
	}

	targetHost := engine.NewHostFromDestination(opts.TargetHost)
	var cleanup goperation.Operation
	if !opts.TargetHost.IsPlainLocalhost() {
		if opts.Registry != nil {
			start, securityCheck, stop := ssh.NewSSHTunnel(opts.TargetHost, opts.Registry.Port, opts.Registry.UseControlSockets)
			cleanup = stop
			ops = append(ops, operation.NewRunRegistry(e, opts.Registry.Port)...)
			ops = append(ops, start)
			ops = append(ops, securityCheck)
			ops = append(ops, operation.NewRegistryTransfer(e, e, composeFile, sourceHost, targetHost, opts.Registry.Port))
			ops = append(ops, stop)
		} else {
			ops = append(ops, operation.NewComposePipeTransfer(e, e, composeFile, sourceHost, targetHost))
		}
	}
	ops = append(ops, operation.NewComposeUp(e, composeFile, targetHost, opts.RecreateMode))
	return goperation.NewSequence(ops...), cleanup
}
