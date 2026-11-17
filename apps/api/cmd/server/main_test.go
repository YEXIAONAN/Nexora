package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestStartupNeverLogsConnectionSecrets(t *testing.T) {
	const marker = "SYNTHETIC_STARTUP_SECRET"
	for _, tc := range []struct{ name, dsn string }{
		{"malformed URL", "postgres://audit:" + marker + "%zz@127.0.0.1/audit"},
		{"query password", "postgres://audit@127.0.0.1/audit?password=" + marker + "&port=invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NEXORA_ENV", "test")
			t.Setenv("NEXORA_DATABASE_URL", tc.dsn)
			t.Setenv("NEXORA_MIGRATE_ON_START", "false")
			t.Setenv("NEXORA_LOG_LEVEL", "info")
			reader, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			original := os.Stdout
			os.Stdout = writer
			defer func() { os.Stdout = original }()
			startupErr := run()
			writer.Close()
			output, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if startupErr == nil {
				t.Fatal("invalid configuration must stop startup")
			}
			if strings.Contains(string(output), marker) || strings.Contains(startupErr.Error(), marker) {
				t.Fatal("startup log or returned error exposed synthetic credentials")
			}
			if strings.Contains(string(output), "http server listening") {
				t.Fatal("invalid configuration started a listener")
			}
		})
	}
}
