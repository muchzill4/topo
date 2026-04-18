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
	SourceEngine engine.Engine
	TargetEngine engine.Engine
}

func SupportsRegistry(noRegistry bool, dest ssh.Destination) bool {
	return !noRegistry && !dest.IsPlainLocalhost()
}

func SupportsSSHControlSockets(goos string) bool {
	return goos != "windows"
}

func NeedsTransfer(dest ssh.Destination, sourceEngine, targetEngine engine.Engine) bool {
	return !dest.IsPlainLocalhost() || sourceEngine != targetEngine
}

func NewDeploymentStop(e engine.Engine, composeFile string, dest ssh.Destination) goperation.Sequence {
	return goperation.Sequence{operation.NewComposeStop(e, composeFile, engine.NewHostFromDestination(dest))}
}

func NewDeployment(composeFile string, opts DeployOptions) (goperation.Sequence, goperation.Operation) {
	sourceHost := engine.LocalHost
	ops := []goperation.Operation{
		operation.NewComposeBuild(opts.SourceEngine, composeFile, sourceHost),
		operation.NewComposePull(opts.SourceEngine, composeFile, sourceHost),
	}

	targetHost := engine.NewHostFromDestination(opts.TargetHost)
	var cleanup goperation.Operation
	if NeedsTransfer(opts.TargetHost, opts.SourceEngine, opts.TargetEngine) {
		if opts.Registry != nil {
			start, securityCheck, stop := ssh.NewSSHTunnel(opts.TargetHost, opts.Registry.Port, opts.Registry.UseControlSockets)
			cleanup = stop
			ops = append(ops, operation.NewRunRegistry(opts.SourceEngine, opts.Registry.Port)...)
			ops = append(ops, start)
			ops = append(ops, securityCheck)
			ops = append(ops, operation.NewRegistryTransfer(opts.SourceEngine, opts.TargetEngine, composeFile, sourceHost, targetHost, opts.Registry.Port))
			ops = append(ops, stop)
		} else {
			ops = append(ops, operation.NewComposePipeTransfer(opts.SourceEngine, opts.TargetEngine, composeFile, sourceHost, targetHost))
		}
	}
	ops = append(ops, operation.NewComposeUp(opts.TargetEngine, composeFile, targetHost, opts.RecreateMode))
	return goperation.NewSequence(ops...), cleanup
}
