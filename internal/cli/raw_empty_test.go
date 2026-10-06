package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRawEmptySuccess(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent} {
		for _, asJSON := range []bool{false, true} {
			t.Run(strconv.Itoa(status)+"/json="+strconv.FormatBool(asJSON), func(t *testing.T) {
				t.Setenv("GOVEETL_TOKEN", "fixture-token")
				t.Setenv("GOVEETL_DRY_RUN", "false")
				var requests atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Method != http.MethodDelete || r.URL.Path != "/empty" {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					}
					w.WriteHeader(status)
				}))
				defer server.Close()
				args := []string{"--config", filepath.Join(t.TempDir(), "config.yaml"), "--base-url", server.URL, "raw", "DELETE", "/empty"}
				want := ""
				if asJSON {
					args = append(args, "--json")
					want = "null\n"
				}
				var stdout, stderr bytes.Buffer
				code := Execute(context.Background(), args, strings.NewReader(""), &stdout, &stderr)
				if code != 0 || stdout.String() != want || stderr.Len() != 0 || requests.Load() != 1 {
					t.Fatalf("code=%d stdout=%q stderr=%q requests=%d", code, &stdout, &stderr, requests.Load())
				}
			})
		}
	}
}
