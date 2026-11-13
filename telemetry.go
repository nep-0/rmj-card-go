package main

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

type telemetry struct {
	traces          *sdktrace.TracerProvider
	qqReplyDuration metric.Float64Histogram
}

var telemetryState *telemetry

type statusWriter struct {
	http.ResponseWriter
	status int
}

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

func newTelemetry(ctx context.Context) (*telemetry, http.Handler, error) {
	res := resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("rmj-card"))
	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, nil, err
	}
	traces := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(traces)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	metricExporter, err := prometheus.New()
	if err != nil {
		return nil, nil, err
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricExporter), sdkmetric.WithResource(res))
	otel.SetMeterProvider(provider)
	qqDuration, err := otel.Meter("rmj-card").Float64Histogram("qq.message.reply.duration", metric.WithUnit("s"), metric.WithDescription("Duration of QQ group message replies"))
	if err != nil {
		return nil, nil, err
	}
	telemetryState = &telemetry{traces: traces, qqReplyDuration: qqDuration}
	return telemetryState, promhttp.Handler(), nil
}

func instrumentRequests(next http.Handler) http.Handler {
	tracer := otel.Tracer("rmj-card")
	duration, _ := otel.Meter("rmj-card").Float64Histogram("http.server.request.duration", metric.WithUnit("s"), metric.WithDescription("Duration of HTTP requests"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		ctx, span := tracer.Start(r.Context(), r.Method+" "+route)
		defer span.End()
		start := time.Now()
		writer := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r.WithContext(ctx))
		status := writer.status
		if status == 0 {
			status = http.StatusOK
		}
		span.SetAttributes(semconv.HTTPRequestMethodKey.String(r.Method), semconv.HTTPRoute(route), semconv.HTTPResponseStatusCode(status))
		if status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
		duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("http.route", route), attribute.String("http.response.status_code", strconv.Itoa(status))))
	})
}

func (t *telemetry) recordQQReply(ctx context.Context, replyType, command string, start time.Time, err error) {
	if t == nil || t.qqReplyDuration == nil {
		return
	}
	tracer := otel.Tracer("rmj-card")
	ctx, span := tracer.Start(ctx, "qq.reply", trace.WithAttributes(attribute.String("qq.command", command), attribute.String("qq.reply.type", replyType)))
	defer span.End()
	status := "success"
	if err != nil {
		status = "error"
		span.RecordError(err)
		span.SetStatus(codes.Error, "reply failed")
	}
	t.qqReplyDuration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attribute.String("qq.reply.type", replyType), attribute.String("qq.command", command), attribute.String("qq.reply.status", status)))
}
