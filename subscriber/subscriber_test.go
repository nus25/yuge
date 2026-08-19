package subscriber

import (
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/urfave/cli/v2"
)

func TestGyokaMinRequestInterval_ConvertsMilliseconds(t *testing.T) {
	flagSet := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	flagSet.Int("gyoka-min-request-interval-ms", 500, "")
	ctx := cli.NewContext(nil, flagSet, nil)

	if got := gyokaMinRequestInterval(ctx); got != 500*time.Millisecond {
		t.Fatalf("gyokaMinRequestInterval() = %v, want %v", got, 500*time.Millisecond)
	}
}

func TestRegisterFeedRoutes_AcceptsPostAndPut(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			fs, tempDir, err := createFeedService(t)
			defer os.RemoveAll(tempDir)
			if err != nil {
				t.Fatalf("createFeedService() error = %v", err)
			}

			configFile := filepath.Join(tempDir, "config", "test-config.yaml")
			if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
				t.Fatalf("MkdirAll() error = %v", err)
			}
			if err := os.WriteFile(configFile, []byte(testConfig), 0644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			api := NewFeedApiHandler(fs)
			router := gin.Default()
			registerFeedAPIRoutes(router, api)

			req, _ := http.NewRequest(method, "/api/feed/test-feed", nil)
			req.Header.Set("Content-Type", "application/json")
			req.Body = io.NopCloser(createJSONBody(t, map[string]any{
				"uri":           "at://did:plc:abcdefg/app.bsky.feed.generator/test-feed",
				"configFile":    "test-config.yaml",
				"inactiveStart": false,
			}))

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusCreated, recorder.Body.String())
			}
		})
	}
}
