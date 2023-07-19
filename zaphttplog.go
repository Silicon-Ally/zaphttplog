// Package zaplog provides a structured request/response logging implementation
// based on zap (go.uber.org/zap). It provides much the same functionality and API
// as github.com/go-chi/httplog, but backed by zap instead of zerolog
// (github.com/rs/zerolog).
package zaphttplog

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var defaultOptions = Options{
	Concise:     false,
	SkipHeaders: nil,
}

type Option func(*Options)

func WithConcise(v bool) Option {
	return func(o *Options) { o.Concise = v }
}

func WithSkipHeaders(headersToSkip []string) Option {
	return func(o *Options) { o.SkipHeaders = headersToSkip }
}

type Options struct {
	// Concise mode includes fewer log details during the request flow. For example
	// excluding details like request content length, user-agent and other details.
	// This is useful if during development your console is too noisy.
	Concise bool

	// SkipHeaders are additional headers which are redacted from the logs
	SkipHeaders []string
}

func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}

	return &Options{
		Concise:     o.Concise,
		SkipHeaders: copySlice(o.SkipHeaders),
	}
}

func copySlice[T any](in []T) []T {
	if in == nil {
		return nil
	}

	out := make([]T, len(in))
	copy(out, in)
	return out
}

func NewMiddleware(logger *zap.Logger, options ...Option) func(next http.Handler) http.Handler {
	opts := defaultOptions.Clone()
	for _, o := range options {
		o(opts)
	}

	return func(next http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			reqField := requestLogField(r, opts)
			entry := &requestLoggerEntry{
				msg:    fmt.Sprintf("%s %s", r.Method, r.URL.Path),
				logger: logger.With(reqField),
				opts:   opts,
			}

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			buf := newLimitBuffer(512)
			ww.Tee(buf)

			t1 := time.Now()
			defer func() {
				var respBody []byte
				if ww.Status() >= 400 {
					respBody, _ = io.ReadAll(buf)
				}
				entry.Write(ww.Status(), ww.BytesWritten(), ww.Header(), time.Since(t1), respBody)
			}()

			next.ServeHTTP(ww, middleware.WithLogEntry(r, entry))
		}
		return http.HandlerFunc(fn)
	}
}

type requestLoggerEntry struct {
	logger *zap.Logger
	msg    string
	opts   *Options
}

func statusLabel(status int) string {
	switch {
	case status >= 100 && status < 300:
		return "OK"
	case status >= 300 && status < 400:
		return "Redirect"
	case status >= 400 && status < 500:
		return "Client Error"
	case status >= 500:
		return "Server Error"
	default:
		return "Unknown"
	}
}

func statusLevel(logger *zap.Logger, status int) func(string, ...zap.Field) {
	switch {
	case status <= 0:
		return logger.Warn
	case status < 400: // for codes in 100s, 200s, 300s
		return logger.Info
	case status >= 400 && status < 500:
		return logger.Warn
	case status >= 500:
		return logger.Error
	default:
		return logger.Info
	}
}

type objEncoderFn func(enc zapcore.ObjectEncoder) error

func headerLogField(header http.Header, opts *Options) []objEncoderFn {
	var out []objEncoderFn
	addStringField := func(k, v string) {
		out = append(out, func(enc zapcore.ObjectEncoder) error { enc.AddString(k, v); return nil })
	}
	for k, v := range header {
		k = strings.ToLower(k)
		if k == "authorization" || k == "cookie" || k == "set-cookie" {
			addStringField(k, "***")
			break
		}
		switch {
		case len(v) == 0:
			continue
		case len(v) == 1:
			addStringField(k, v[0])
		default:
			addStringField(k, fmt.Sprintf("[%s]", strings.Join(v, "], [")))
		}

		for _, skip := range opts.SkipHeaders {
			if k == skip {
				addStringField(k, "***")
				break
			}
		}
	}
	return out
}

func (l *requestLoggerEntry) Write(status, byteCnt int, header http.Header, elapsed time.Duration, extra interface{}) {
	var msg bytes.Buffer
	if l.msg != "" {
		msg.WriteString(l.msg)
		msg.WriteString(" - ")
	}
	msg.WriteString(strconv.Itoa(status))
	msg.WriteRune(' ')
	msg.WriteString(statusLabel(status))

	fields := []objEncoderFn{
		func(enc zapcore.ObjectEncoder) error { enc.AddInt("status", status); return nil },
		func(enc zapcore.ObjectEncoder) error { enc.AddInt("bytes", byteCnt); return nil },
		func(enc zapcore.ObjectEncoder) error { enc.AddDuration("elapsed", elapsed); return nil },
	}

	if !l.opts.Concise {
		// Include response header, as well for error status codes (>400) we include
		// the response body so we may inspect the log message sent back to the client.
		if status >= 400 {
			body, _ := extra.([]byte)
			fields = append(fields, func(enc zapcore.ObjectEncoder) error { enc.AddByteString("body", body); return nil })
		}
		if len(header) > 0 {
			fields = append(fields, func(enc zapcore.ObjectEncoder) error {
				return enc.AddObject("header", toMarshaler(headerLogField(header, l.opts)))
			})
		}
	}

	log := statusLevel(l.logger, status)

	log(msg.String(), zap.Object("httpResponse", toMarshaler(fields)))
}

func toMarshaler(in []objEncoderFn) zapcore.ObjectMarshaler {
	return zapcore.ObjectMarshalerFunc(func(enc zapcore.ObjectEncoder) error {
		for _, f := range in {
			if err := f(enc); err != nil {
				return err
			}
		}
		return nil
	})
}

func (l *requestLoggerEntry) Panic(v interface{}, stack []byte) {
	l.logger = l.logger.With(
		zap.ByteString("stacktrace", stack),
		zap.Any("panic", v),
	)

	l.msg = fmt.Sprintf("%+v", v)
}

func requestLogField(r *http.Request, opts *Options) zap.Field {
	var fields []objEncoderFn
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	requestURL := fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)

	fields = append(fields,
		func(enc zapcore.ObjectEncoder) error { enc.AddString("requestURL", requestURL); return nil },
		func(enc zapcore.ObjectEncoder) error { enc.AddString("requestMethod", r.Method); return nil },
		func(enc zapcore.ObjectEncoder) error { enc.AddString("requestPath", r.URL.Path); return nil },
		func(enc zapcore.ObjectEncoder) error { enc.AddString("remoteIP", r.RemoteAddr); return nil },
		func(enc zapcore.ObjectEncoder) error { enc.AddString("proto", r.Proto); return nil },
	)
	if reqID := middleware.GetReqID(r.Context()); reqID != "" {
		fields = append(fields, func(enc zapcore.ObjectEncoder) error { enc.AddString("requestID", reqID); return nil })
	}

	if opts.Concise {
		return zap.Object("httpRequest", toMarshaler(fields))
	}

	fields = append(fields, func(enc zapcore.ObjectEncoder) error { enc.AddString("scheme", scheme); return nil })

	if len(r.Header) > 0 {
		fields = append(fields, func(enc zapcore.ObjectEncoder) error {
			return enc.AddObject("header", toMarshaler(headerLogField(r.Header, opts)))
		})
	}

	return zap.Object("httpRequest", toMarshaler(fields))

}

// limitBuffer is used to pipe response body information from the
// response writer to a certain limit amount. The idea is to read
// a portion of the response body such as an error response so we
// may log it.
type limitBuffer struct {
	*bytes.Buffer
	limit int
}

func newLimitBuffer(size int) io.ReadWriter {
	return limitBuffer{
		Buffer: bytes.NewBuffer(make([]byte, 0, size)),
		limit:  size,
	}
}

func (b limitBuffer) Write(p []byte) (n int, err error) {
	if b.Buffer.Len() >= b.limit {
		return len(p), nil
	}
	limit := b.limit
	if len(p) < limit {
		limit = len(p)
	}
	return b.Buffer.Write(p[:limit])
}

func (b limitBuffer) Read(p []byte) (n int, err error) {
	return b.Buffer.Read(p)
}
