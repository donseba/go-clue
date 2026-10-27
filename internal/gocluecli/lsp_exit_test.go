package gocluecli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestLSPExitCode(t *testing.T) {
	tests := []struct {
		name     string
		shutdown bool
		wantCode int
	}{
		{name: "after shutdown", shutdown: true, wantCode: 0},
		{name: "without shutdown", shutdown: false, wantCode: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input bytes.Buffer

			writeTestRPC(t, &input, map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "initialize",
				"params":  map[string]any{},
			})

			if tt.shutdown {
				writeTestRPC(t, &input, map[string]any{
					"jsonrpc": "2.0",
					"id":      2,
					"method":  lspShutdownMethod,
				})
			}

			writeTestRPC(t, &input, map[string]any{
				"jsonrpc": "2.0",
				"method":  lspExitMethod,
			})

			// The server stops at exit, so it does not answer this request.
			writeTestRPC(t, &input, map[string]any{
				"jsonrpc": "2.0",
				"id":      3,
				"method":  "initialize",
				"params":  map[string]any{},
			})

			var output, logs bytes.Buffer

			err := runLSP(&input, &output, &logs, t.TempDir())

			code := 0

			var exit exitCodeError

			switch {
			case errors.As(err, &exit):
				code = exit.code
			case err != nil:
				t.Fatalf("runLSP() error = %v, want nil or an exit code", err)
			}

			if code != tt.wantCode {
				t.Fatalf("exit code = %d (error %v), want %d", code, err, tt.wantCode)
			}

			if strings.Contains(output.String(), `"id":3`) {
				t.Fatalf("the server answered a request after exit:\n%s", output.String())
			}
		})
	}
}
