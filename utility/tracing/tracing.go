package tracing

import (
	"SuperBizAgent/internal/consts"
	"context"
	"os"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const defaultServiceName = "opscaptionai-backend"

func Init(ctx context.Context) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if !enabled(ctx) {
		otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	endpoint := normalizeOptionalEndpoint(configString(ctx, "tracing.jaeger_endpoint"))
	if endpoint == "" {
		otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(endpoint)))
	if err != nil {
		g.Log().Warningf(ctx, "invalid tracing.jaeger_endpoint %q, tracing disabled: %v", endpoint, err)
		otel.SetTracerProvider(oteltrace.NewNoopTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio(ctx)))),
		sdktrace.WithResource(resource.NewSchemaless(
			attribute.String("service.name", serviceName(ctx)),
			attribute.String("deployment.environment", environmentName()),
		)),
	)

	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

func StartSpan(ctx context.Context, tracerName, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	ctx, span := otel.Tracer(fallback(tracerName, defaultServiceName)).Start(ctx, spanName, opts...)
	return ContextWithTraceID(ctx), span
}

func ContextWithTraceID(ctx context.Context) context.Context {
	if ctx == nil {
		return nil
	}
	traceID := CurrentTraceID(ctx)
	if traceID == "" {
		return ctx
	}
	return context.WithValue(ctx, consts.CtxKeyTraceID, traceID)
}

func CurrentTraceID(ctx context.Context) string {
	spanCtx := oteltrace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return ""
	}
	return spanCtx.TraceID().String()
}

func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	if len(attrs) == 0 {
		return
	}
	oteltrace.SpanFromContext(ctx).SetAttributes(attrs...)
}

func enabled(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "tracing.enabled")
	if err != nil || strings.TrimSpace(v.String()) == "" {
		return false
	}
	return v.Bool()
}

func serviceName(ctx context.Context) string {
	return fallback(strings.TrimSpace(configString(ctx, "tracing.service_name")), defaultServiceName)
}

func sampleRatio(ctx context.Context) float64 {
	v, err := g.Cfg().Get(ctx, "tracing.sample_ratio")
	if err != nil {
		return 1
	}
	ratio := v.Float64()
	if ratio <= 0 {
		return 1
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}

func configString(ctx context.Context, key string) string {
	v, err := g.Cfg().Get(ctx, key)
	if err != nil {
		return ""
	}
	return v.String()
}

func normalizeOptionalEndpoint(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "${") && strings.Contains(value, "}") {
		return ""
	}
	return value
}

func environmentName() string {
	for _, value := range []string{
		os.Getenv("APP_ENV"),
		os.Getenv("ENVIRONMENT"),
		os.Getenv("GO_ENV"),
	} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "development"
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
