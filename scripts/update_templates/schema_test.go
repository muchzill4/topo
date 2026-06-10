package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateValidator(t *testing.T) {
	schemaJSON, err := readCatalogSchema()
	require.NoError(t, err)

	validator, err := NewCatalogSchemaFromBytes(schemaJSON)
	require.NoError(t, err)

	t.Run("accepts template that matches catalog schema", func(t *testing.T) {
		template := Template{
			XTopo: XTopo{
				Name:        "Hello World",
				Description: "A friendly template",
				Features:    []string{"web"},
			},
			URL: "https://github.com/Arm-Examples/topo-welcome.git",
			Ref: "main",
		}

		err := validator.Validate(template)

		assert.NoError(t, err)
	})

	t.Run("rejects template that does not match catalog schema", func(t *testing.T) {
		template := Template{
			XTopo: XTopo{
				Description: "Missing a name",
			},
			URL: "https://github.com/Arm-Examples/topo-welcome.git",
			Ref: "main",
		}

		err := validator.Validate(template)

		assert.Error(t, err)
	})
}
