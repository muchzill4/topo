package cpu

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/arm/topo/pkg/plugin"
)

func inventory(ctx context.Context, runner plugin.TargetRunner) (plugin.FactsResult, error) {
	output, _, err := runner.Run(ctx, "lscpu --json --extended=CPU,MODELNAME")
	if err != nil {
		return plugin.FactsResult{}, fmt.Errorf("querying CPU inventory: %w", err)
	}
	var document struct {
		CPUs []struct {
			CPU       json.Number `json:"cpu"`
			ModelName string      `json:"modelname"`
		} `json:"cpus"`
	}
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		return plugin.FactsResult{}, fmt.Errorf("parsing CPU inventory: %w", err)
	}
	var result plugin.FactsResult
	seen := make(map[uint64]bool, len(document.CPUs))
	for _, row := range document.CPUs {
		number, err := strconv.ParseUint(row.CPU.String(), 10, 32)
		if err != nil || seen[number] {
			return plugin.FactsResult{}, fmt.Errorf("invalid or duplicate logical CPU ID: %q", row.CPU)
		}
		seen[number] = true
		group := plugin.FactGroup{ID: fmt.Sprintf("cpu%d", number)}
		if model := strings.TrimSpace(row.ModelName); model != "" && model != "-" {
			group.Facts = append(group.Facts, plugin.Fact{
				Key: "cpu.model", Label: "CPU model",
				Value: plugin.Value{Type: "string", Text: model},
			})
		}
		result.Groups = append(result.Groups, group)
	}
	return result, nil
}
