package core

import (
	"context"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	logsapi "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

func newLoggerProvider(ctx context.Context, opts Options, res *resource.Resource) (logsapi.LoggerProvider, func(context.Context) error, error) {
	exp, err := newLogExporter(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
		sdklog.WithResource(res),
	)
	return lp, lp.Shutdown, nil
}

func newLogExporter(ctx context.Context, opts Options) (sdklog.Exporter, error) {
	endpoint := resolveOTLPEndpoint(opts.OTLPEndpoint, opts.Transport)
	switch opts.Transport {
	case TransportHTTP:
		o := []otlploghttp.Option{otlploghttp.WithEndpoint(endpoint)}
		if !opts.OTLPSecure {
			o = append(o, otlploghttp.WithInsecure())
		}
		return otlploghttp.New(ctx, o...)
	default:
		o := []otlploggrpc.Option{otlploggrpc.WithEndpoint(endpoint)}
		if !opts.OTLPSecure {
			o = append(o, otlploggrpc.WithInsecure())
		}
		return otlploggrpc.New(ctx, o...)
	}
}
