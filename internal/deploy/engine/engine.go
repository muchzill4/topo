package engine

import "fmt"

type Engine struct {
	binary string
}

var (
	Docker = Engine{"docker"}
	Podman = Engine{"podman"}
)

var knownEngines = map[string]Engine{
	"docker": Docker,
	"podman": Podman,
}

func ParseEngine(name string) (Engine, error) {
	if e, ok := knownEngines[name]; ok {
		return e, nil
	}
	return Engine{}, fmt.Errorf("unknown engine %q: supported engines are docker, podman", name)
}

func (e Engine) String() string {
	return e.binary
}
