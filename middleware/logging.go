package middleware

import (
	"net/http"

	"github.com/felixge/httpsnoop"
	"go.uber.org/zap"
)

func LogRequests(l *zap.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := httpsnoop.CaptureMetrics(h, w, r)

		fields := []zap.Field{
			zap.String("http_method", r.Method),
			zap.String("http_resource", r.URL.Path),
			zap.Int("http_status", m.Code),
			zap.Duration("request_duration", m.Duration),
			zap.Int64("request_length", m.Written),
		}

		if id := GetRequestID(r.Context()); id != "" {
			fields = append(fields, zap.String("request_id", id))
		}

		l.Info("request completed", fields...)
	})
}
