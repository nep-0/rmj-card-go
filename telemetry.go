package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

var qqReplyDuration metric.Float64Histogram

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func instrumentRequests(next http.Handler) (http.Handler, http.Handler, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, nil, err
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)
	duration, err := otel.Meter("rmj-card").Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithDescription("Duration of HTTP requests"))
	if err != nil {
		return nil, nil, err
	}
	qqReplyDuration, err = otel.Meter("rmj-card").Float64Histogram("qq.message.reply.duration", metric.WithUnit("s"), metric.WithDescription("Duration of QQ group message replies"))
	if err != nil {
		return nil, nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		writer := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		status := writer.status
		if status == 0 {
			status = http.StatusOK
		}
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		duration.Record(r.Context(), time.Since(start).Seconds(), metric.WithAttributes(
			attribute.String("http.request.method", r.Method),
			attribute.String("http.route", route),
			attribute.String("http.response.status_code", strconv.Itoa(status)),
		))
	}), promhttp.Handler(), nil
}

func recordQQReply(ctx context.Context, replyType, command string, start time.Time, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("qq.reply.type", replyType),
		attribute.String("qq.command", command),
	}
	if err != nil {
		attrs = append(attrs, attribute.String("qq.reply.status", "error"))
	} else {
		attrs = append(attrs, attribute.String("qq.reply.status", "success"))
	}
	if qqReplyDuration != nil {
		qqReplyDuration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attrs...))
	}
}
