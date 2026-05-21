package catalog_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/arm/topo/internal/catalog"
	"github.com/arm/topo/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetch(t *testing.T) {
	t.Run("given a remote url, it fetches the catalog json", func(t *testing.T) {
		catalogJSON := `[{"json": "for-real"}]`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/give-json" {
				w.Write([]byte(catalogJSON))
			}
		}))
		defer server.Close()

		url := fmt.Sprintf("%s/give-json", server.URL)
		got, err := catalog.FetchTemplatesJSON(url)

		require.NoError(t, err)
		assert.Equal(t, catalogJSON, string(got))
	})

	t.Run("given a file:// url, it fetches the catalog json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "file.json")
		catalogJSON := `[{"json": "actually-json"}]`
		testutil.RequireWriteFile(t, path, catalogJSON)

		url := fmt.Sprintf("file://%s", path)
		got, err := catalog.FetchTemplatesJSON(url)

		require.NoError(t, err)
		assert.Equal(t, catalogJSON, string(got))
	})

	t.Run("returns error when request fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)

		}))
		defer server.Close()

		url := fmt.Sprintf("%s/give-json-pretty-please", server.URL)
		_, err := catalog.FetchTemplatesJSON(url)

		assert.Error(t, err)
	})
}
