package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func Directory() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "topo", "plugins"), nil
}

type Collection struct {
	ctx     context.Context
	cancel  context.CancelFunc
	plugins []*Plugin
}

func LoadAll(ctx context.Context, directory string) (result *Collection, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	paths, err := executables(directory)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	collection := &Collection{ctx: ctx, cancel: cancel}
	defer func() {
		if err != nil {
			collection.Close()
		}
	}()
	for _, path := range paths {
		loaded, err := Load(ctx, path)
		if err != nil {
			return nil, err
		}
		for _, previous := range collection.plugins {
			if previous.descriptor.Name == loaded.descriptor.Name {
				loaded.Close()
				return nil, fmt.Errorf("duplicate provider name %q", loaded.descriptor.Name)
			}
		}
		collection.plugins = append(collection.plugins, loaded)
	}
	return collection, nil
}

func (c *Collection) Close() {
	for _, loaded := range c.plugins {
		loaded.Close()
	}
	c.cancel()
}

func executables(directory string) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading plugin directory: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "topo-plugin-") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("checking plugin %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if runtime.GOOS == "windows" {
			if !strings.HasSuffix(strings.ToLower(entry.Name()), ".exe") {
				continue
			}
		} else if info.Mode().Perm()&0o111 == 0 {
			continue
		}
		paths = append(paths, path)
	}
	return paths, nil
}
