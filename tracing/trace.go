package tracing

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

type Options struct {
	EnableJaeger   bool
	JaegerEndpoint string
}

func newJaegerExporter(endpoint string) (tracesdk.SpanExporter, error) {
	exp, err := jaeger.New(
		jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(endpoint)),
	)
	if err != nil {
		return nil, err
	}
	return exp, nil
}

func InitTracerProvider(log logr.Logger, o Options) (*tracesdk.TracerProvider, error) {
	tracerOptions := []tracesdk.TracerProviderOption{
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("analyzer-lsp"),
		)),
	}
	if o.EnableJaeger {
		exp, err := newJaegerExporter(o.JaegerEndpoint)
		if err != nil {
			log.Error(err, "failed to create jaeger exporter")
			return nil, err
		}
		tracerOptions = append(tracerOptions,
			tracesdk.WithBatcher(exp))
	}

	tp := tracesdk.NewTracerProvider(tracerOptions...)
	otel.SetTracerProvider(tp)

	return tp, nil
}

func Shutdown(ctx context.Context, log logr.Logger, tp *tracesdk.TracerProvider) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	if err := tp.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down tracer provider")
	}
}

func StartNewSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	ctx, span := otel.Tracer("").Start(ctx, name)
	span.SetAttributes(attrs...)
	return ctx, span
}
