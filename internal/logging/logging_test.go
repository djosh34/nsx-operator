package logging_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/djosh34/nsx-operator/internal/logging"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNewInfoLoggerWritesJSONLAndSuppressesDebug(t *testing.T) {
	var sink bytes.Buffer
	logger, err := logging.New(logging.Options{
		Level: "info",
		Sink:  zapcore.AddSync(&sink),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Debug("debug details", zap.String("component", "test"))
	logger.Info("startup completed", zap.String("component", "test"))
	err = logger.Sync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	lines := nonEmptyLines(sink.String())
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1; output: %q", len(lines), sink.String())
	}

	var entry map[string]any
	err = json.Unmarshal([]byte(lines[0]), &entry)
	if err != nil {
		t.Fatalf("log line is not valid JSON: %v; line: %q", err, lines[0])
	}
	if entry["level"] != "info" {
		t.Fatalf("level = %v, want info", entry["level"])
	}
	if entry["msg"] != "startup completed" {
		t.Fatalf("msg = %v, want startup completed", entry["msg"])
	}
	if entry["component"] != "test" {
		t.Fatalf("component = %v, want test", entry["component"])
	}
	if strings.Contains(sink.String(), "debug details") {
		t.Fatalf("debug log was emitted at info level: %q", sink.String())
	}
}

func TestNewDebugLoggerWritesDebugJSONL(t *testing.T) {
	var sink bytes.Buffer
	logger, err := logging.New(logging.Options{
		Level: "debug",
		Sink:  zapcore.AddSync(&sink),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Debug("loaded logging config", zap.String("component", "startup"))
	err = logger.Sync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	lines := nonEmptyLines(sink.String())
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1; output: %q", len(lines), sink.String())
	}

	var entry map[string]any
	err = json.Unmarshal([]byte(lines[0]), &entry)
	if err != nil {
		t.Fatalf("log line is not valid JSON: %v; line: %q", err, lines[0])
	}
	if entry["level"] != "debug" {
		t.Fatalf("level = %v, want debug", entry["level"])
	}
	if entry["msg"] != "loaded logging config" {
		t.Fatalf("msg = %v, want loaded logging config", entry["msg"])
	}
}

func TestNewWarnAndErrorLevelsFilterLowerPriorityLogs(t *testing.T) {
	tests := map[string]struct {
		level      string
		log        func(*zap.Logger)
		wantOutput bool
	}{
		"warn suppresses info": {
			level: "warn",
			log: func(logger *zap.Logger) {
				logger.Info("suppressed info")
			},
			wantOutput: false,
		},
		"warn emits warn": {
			level: "warn",
			log: func(logger *zap.Logger) {
				logger.Warn("visible warning")
			},
			wantOutput: true,
		},
		"error suppresses warn": {
			level: "error",
			log: func(logger *zap.Logger) {
				logger.Warn("suppressed warning")
			},
			wantOutput: false,
		},
		"error emits error": {
			level: "error",
			log: func(logger *zap.Logger) {
				logger.Error("visible error")
			},
			wantOutput: true,
		},
	}

	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			var sink bytes.Buffer
			logger, err := logging.New(logging.Options{
				Level: testCase.level,
				Sink:  zapcore.AddSync(&sink),
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			testCase.log(logger)
			err = logger.Sync()
			if err != nil {
				t.Fatalf("Sync() error = %v", err)
			}

			hasOutput := len(nonEmptyLines(sink.String())) > 0
			if hasOutput != testCase.wantOutput {
				t.Fatalf("hasOutput = %t, want %t; output: %q", hasOutput, testCase.wantOutput, sink.String())
			}
		})
	}
}

func TestNewWithNilSinkWritesJSONLToStderr(t *testing.T) {
	stderr := captureLoggingStderr(t, func() {
		logger, err := logging.New(logging.Options{Level: "info"})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		logger.Info("default sink log", logging.Component("test"))
		err = logger.Sync()
		if err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
	})

	lines := nonEmptyLines(stderr)
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1; output: %q", len(lines), stderr)
	}
	var entry map[string]any
	err := json.Unmarshal([]byte(lines[0]), &entry)
	if err != nil {
		t.Fatalf("stderr line is not valid JSON: %v; line: %q", err, lines[0])
	}
	if entry["msg"] != "default sink log" {
		t.Fatalf("msg = %v, want default sink log", entry["msg"])
	}
}

func TestNewStderrWritesJSONLToStderr(t *testing.T) {
	stderr := captureLoggingStderr(t, func() {
		logger, err := logging.NewStderr("info")
		if err != nil {
			t.Fatalf("NewStderr() error = %v", err)
		}
		logger.Info("stderr logger log", logging.Component("test"))
		err = logger.Sync()
		if err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
	})

	lines := nonEmptyLines(stderr)
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1; output: %q", len(lines), stderr)
	}
	var entry map[string]any
	err := json.Unmarshal([]byte(lines[0]), &entry)
	if err != nil {
		t.Fatalf("stderr line is not valid JSON: %v; line: %q", err, lines[0])
	}
	if entry["msg"] != "stderr logger log" {
		t.Fatalf("msg = %v, want stderr logger log", entry["msg"])
	}
}

func TestNewRejectsUnsupportedLevelWithoutWritingLogs(t *testing.T) {
	sink := &failingWriteSyncer{}
	logger, err := logging.New(logging.Options{
		Level: "trace",
		Sink:  sink,
	})
	if err == nil {
		t.Fatal("New() error = nil, want unsupported level error")
	}
	if logger != nil {
		t.Fatalf("New() logger = %#v, want nil", logger)
	}
	if sink.writeCalled {
		t.Fatal("sink was written for unsupported level")
	}
}

func TestNewStderrRejectsUnsupportedLevel(t *testing.T) {
	logger, err := logging.NewStderr("trace")
	if err == nil {
		t.Fatal("NewStderr() error = nil, want unsupported level error")
	}
	if logger != nil {
		t.Fatalf("NewStderr() logger = %#v, want nil", logger)
	}
}

func TestFieldHelpersWriteRequiredJSONKeys(t *testing.T) {
	var sink bytes.Buffer
	logger, err := logging.New(logging.Options{
		Level: "info",
		Sink:  zapcore.AddSync(&sink),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Info(
		"reconcile sweep completed",
		logging.Component("controller"),
		logging.NetworkCloudFQDN("nc.example.test"),
		logging.GroupID("group-1"),
		logging.SweepID("sweep-1"),
		logging.ReconcileKey("default/resource"),
	)
	err = logger.Sync()
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	lines := nonEmptyLines(sink.String())
	if len(lines) != 1 {
		t.Fatalf("log lines = %d, want 1; output: %q", len(lines), sink.String())
	}

	var entry map[string]any
	err = json.Unmarshal([]byte(lines[0]), &entry)
	if err != nil {
		t.Fatalf("log line is not valid JSON: %v; line: %q", err, lines[0])
	}

	wantFields := map[string]string{
		"component":        "controller",
		"networkCloudFQDN": "nc.example.test",
		"groupID":          "group-1",
		"sweepID":          "sweep-1",
		"reconcileKey":     "default/resource",
	}
	for key, want := range wantFields {
		if entry[key] != want {
			t.Fatalf("%s = %v, want %s", key, entry[key], want)
		}
	}
}

func nonEmptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

type failingWriteSyncer struct {
	writeCalled bool
}

func (sink *failingWriteSyncer) Write([]byte) (int, error) {
	sink.writeCalled = true
	return 0, errors.New("unexpected write")
}

func (sink *failingWriteSyncer) Sync() error {
	return nil
}

func captureLoggingStderr(t *testing.T, run func()) string {
	t.Helper()

	originalStderr := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = writer
	defer func() {
		os.Stderr = originalStderr
	}()

	run()

	err = writer.Close()
	if err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	err = reader.Close()
	if err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return string(output)
}
