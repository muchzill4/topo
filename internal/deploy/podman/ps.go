package podman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/arm/topo/internal/project"
	"github.com/arm/topo/internal/ssh"
)

const hostProcessingDomain = "Linux Host"

type PSContainer struct {
	ID     string `json:"ID"`
	Names  string `json:"Names"`
	Image  string `json:"Image"`
	State  string `json:"State"`
	Status string `json:"Status"`
	Ports  string `json:"Ports"`
}

type Container struct {
	Id               string `json:"id"`
	Names            string `json:"names"`
	Image            string `json:"image"`
	State            string `json:"state"`
	Status           string `json:"status"`
	ProcessingDomain string `json:"processingDomain"`
	Address          string `json:"address"`
}

func ListContainers(scope project.Scope, target ssh.Destination, hostname string, all bool) (containers []Container, err error) {
	socket := LocalSocket
	var tunnel *ssh.TCPToUnixSocketTunnel
	if !target.IsPlainLocalhost() {
		tunnel, err = TunnelRemoteSocketPath(context.Background(), io.Discard, target)
		if err != nil {
			return nil, err
		}
		defer func() {
			err = errors.Join(err, tunnel.Close())
		}()
		socket = NewSocket(tunnel.SocketURL())
	}

	rawJSON, err := getContainers(scope, socket, all)
	if err != nil {
		return nil, err
	}
	raws, err := ParseContainers(rawJSON)
	if err != nil {
		return nil, err
	}
	containers = RemapAddresses(raws, hostname)
	domains, err := getProcessingDomains(raws, socket)
	if err != nil {
		return nil, err
	}
	for i, raw := range raws {
		if domain, ok := domains[raw.ID]; ok {
			containers[i].ProcessingDomain = domain
		}
	}
	return containers, nil
}

func getContainers(scope project.Scope, socket Socket, all bool) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd, err := ComposeCommand(context.Background(), socket, scope, composePSArgs(all)...)
	if err != nil {
		return "", err
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("podman compose ps: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func composePSArgs(all bool) []string {
	args := []string{"ps", "--format", "json"}
	if all {
		args = append(args, "--all")
	}
	return args
}

func ParseContainers(rawJSON string) ([]PSContainer, error) {
	raws := []PSContainer{}
	decoder := json.NewDecoder(strings.NewReader(rawJSON))
	for {
		var raw PSContainer
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		raws = append(raws, raw)
	}
	return raws, nil
}

func getProcessingDomains(raws []PSContainer, socket Socket) (map[string]string, error) {
	args := []string{"inspect", "--type", "container"}
	for _, raw := range raws {
		if raw.ID != "" {
			args = append(args, raw.ID)
		}
	}
	if len(args) == 3 {
		return map[string]string{}, nil
	}

	var stdout, stderr bytes.Buffer
	cmd := Command(context.Background(), socket, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("podman inspect: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ParseProcessingDomains(stdout.String())
}

func ParseProcessingDomains(rawJSON string) (map[string]string, error) {
	var inspected []struct {
		ID     string `json:"Id"`
		Config struct {
			Annotations map[string]string `json:"Annotations"`
		} `json:"Config"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &inspected); err != nil {
		return nil, err
	}

	domains := map[string]string{}
	for _, container := range inspected {
		domain := strings.TrimSpace(container.Config.Annotations["remoteproc.name"])
		if domain == "" || container.ID == "" {
			continue
		}
		domains[container.ID] = domain
		if len(container.ID) > 12 {
			domains[container.ID[:12]] = domain
		}
	}
	return domains, nil
}

func RemapAddresses(raws []PSContainer, hostname string) []Container {
	containers := make([]Container, len(raws))
	for i, raw := range raws {
		containers[i] = Container{
			Id:               raw.ID,
			Names:            raw.Names,
			Image:            raw.Image,
			State:            raw.State,
			Status:           raw.Status,
			ProcessingDomain: hostProcessingDomain,
			Address:          publishedAddress(raw.Ports, hostname),
		}
	}
	return containers
}

func publishedAddress(rawPorts, hostname string) string {
	if hostname == "" {
		return rawPorts
	}
	parts := strings.Split(rawPorts, ", ")
	for i, part := range parts {
		if idx := strings.Index(part, "->"); idx != -1 {
			part = part[:idx]
		}
		parts[i] = strings.ReplaceAll(part, "0.0.0.0", hostname)
	}
	return strings.Join(parts, ", ")
}
