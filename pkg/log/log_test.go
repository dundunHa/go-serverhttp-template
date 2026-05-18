package log

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewHandlerUsesTextInDev(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newHandler("dev", slog.LevelInfo, &buf))

	logger.Info("hello", "module", "test")

	got := buf.String()
	if strings.HasPrefix(got, "{") {
		t.Fatalf("dev log should be text, got JSON-like output: %s", got)
	}
	if !strings.Contains(got, "msg=hello") || !strings.Contains(got, "module=test") {
		t.Fatalf("dev log missing fields: %s", got)
	}
}

func TestNewHandlerUsesJSONInProd(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newHandler("prod", slog.LevelInfo, &buf))

	logger.Info("hello", "module", "test")

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("prod log should be JSON: %v; output=%s", err, buf.String())
	}
	if got["msg"] != "hello" || got["module"] != "test" {
		t.Fatalf("prod log missing fields: %#v", got)
	}
}

func TestParseLevelControlsOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(newHandler("prod", parseLevel("warn"), &buf))

	logger.Info("skip")
	logger.Warn("keep")

	got := buf.String()
	if strings.Contains(got, "skip") {
		t.Fatalf("info log should be filtered at warn level: %s", got)
	}
	if !strings.Contains(got, "keep") {
		t.Fatalf("warn log should be emitted: %s", got)
	}
}
