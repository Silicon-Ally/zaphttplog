package zaphttplog

import (
	"net/http"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

func TestStatusLabel(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{
			status: http.StatusOK,
			want:   "OK",
		},
		{
			status: http.StatusNoContent,
			want:   "OK",
		},
		{
			status: http.StatusFound,
			want:   "Redirect",
		},
		{
			status: http.StatusTemporaryRedirect,
			want:   "Redirect",
		},
		{
			status: http.StatusNotFound,
			want:   "Client Error",
		},
		{
			status: http.StatusForbidden,
			want:   "Client Error",
		},
		{
			status: http.StatusInternalServerError,
			want:   "Server Error",
		},
		{
			status: http.StatusBadGateway,
			want:   "Server Error",
		},
	}

	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			got := statusLabel(test.status)
			if got != test.want {
				t.Errorf("statusLabel(%d) = %q, want %q", test.status, got, test.want)
			}
		})
	}
}

func TestStatusLevel(t *testing.T) {
	var logs []zapcore.Entry
	l := zaptest.NewLogger(t, zaptest.WrapOptions(zap.Hooks(func(e zapcore.Entry) error {
		logs = append(logs, e)
		return nil
	})))

	tests := []struct {
		status int
		want   func(string, ...zap.Field)
	}{
		{
			status: 0,
			want:   l.Warn,
		},
		{
			status: http.StatusOK,
			want:   l.Info,
		},
		{
			status: http.StatusNoContent,
			want:   l.Info,
		},
		{
			status: http.StatusFound,
			want:   l.Info,
		},
		{
			status: http.StatusTemporaryRedirect,
			want:   l.Info,
		},
		{
			status: http.StatusNotFound,
			want:   l.Warn,
		},
		{
			status: http.StatusForbidden,
			want:   l.Warn,
		},
		{
			status: http.StatusInternalServerError,
			want:   l.Error,
		},
		{
			status: http.StatusBadGateway,
			want:   l.Error,
		},
	}

	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			got := statusLevel(l, test.status)

			// We can't directly compare `got` and `test.want` because they're functions, so
			// we make sure they log to the appropriate levels instead.
			got("log")
			test.want("log")

			if len(logs) < 2 {
				t.Fatalf("log functions didn't write logs, %d logs recorded", len(logs))
			}

			gotLog, wantLog := logs[len(logs)-2], logs[len(logs)-1]
			if gotLog.Level != wantLog.Level {
				t.Errorf("statusLevel(%d) = %q, want %q", test.status, gotLog.Level, wantLog.Level)
			}
		})
	}
}
