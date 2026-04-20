package compose

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

type composeFileSchema struct {
	Name     string `yaml:"name"`
	Services map[string]struct {
		Build any    `yaml:"build"`
		Image string `yaml:"image"`
	} `yaml:"services"`
}

func ProjectName(composeFilePath string) string {
	abs, err := filepath.Abs(composeFilePath)
	if err != nil {
		abs = composeFilePath
	}
	return filepath.Base(filepath.Dir(abs))
}

func ImageNames(composeFile io.Reader, defaultProjectName string) ([]string, error) {
	cf, err := parseComposeFile(composeFile)
	if err != nil {
		return nil, err
	}
	project := cf.Name
	if project == "" {
		project = defaultProjectName
	}
	var names []string
	for name, svc := range cf.Services {
		if svc.Image != "" {
			names = append(names, svc.Image)
		} else {
			names = append(names, project+"-"+name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func PullableServices(composeFile io.Reader) ([]string, error) {
	cf, err := parseComposeFile(composeFile)
	if err != nil {
		return nil, err
	}
	var names []string
	for name, svc := range cf.Services {
		if svc.Build == nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func parseComposeFile(r io.Reader) (composeFileSchema, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return composeFileSchema{}, err
	}
	var cf composeFileSchema
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return composeFileSchema{}, fmt.Errorf("parsing compose file: %w", err)
	}
	return cf, nil
}
