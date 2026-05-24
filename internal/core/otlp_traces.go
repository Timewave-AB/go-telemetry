package core

import (
	"context"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	traceapi "go.opentelemetry.io/otel/trace"
)

func newTracerProvider(ctx context.Context, opts Options, res *resource.Resource) (traceapi.TracerProvider, func(context.Context) error, error) {
	exp, err := newTraceExporter(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler(opts.TraceSampleRatio)),
	)
	return tp, tp.Shutdown, nil
}

func newTraceExporter(ctx context.Context, opts Options) (sdktrace.SpanExporter, error) {
	endpoint := resolveOTLPEndpoint(opts.OTLPEndpoint, opts.Transport)
	switch opts.Transport {
	case TransportHTTP:
		o := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint)}
		if !opts.OTLPSecure {
			o = append(o, otlptracehttp.WithInsecure())
		}
		return otlptracehttp.New(ctx, o...)
	default:
		o := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
		if !opts.OTLPSecure {
			o = append(o, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.New(ctx, o...)
	}
}

func sampler(ratio float64) sdktrace.Sampler {
	if ratio <= 0 {
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
	if ratio >= 1 {
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}
