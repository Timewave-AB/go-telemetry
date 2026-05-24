package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	metricapi "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

func newMeterProvider(ctx context.Context, opts Options, res *resource.Resource) (metricapi.MeterProvider, func(context.Context) error, error) {
	exp, err := newMetricExporter(ctx, opts)
	if err != nil {
		return nil, nil, err
	}
	readerOpts := []sdkmetric.PeriodicReaderOption{}
	if opts.MetricExportInterval > 0 {
		readerOpts = append(readerOpts, sdkmetric.WithInterval(opts.MetricExportInterval))
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp, readerOpts...)),
		sdkmetric.WithResource(res),
	)
	return mp, mp.Shutdown, nil
}

func newMetricExporter(ctx context.Context, opts Options) (sdkmetric.Exporter, error) {
	endpoint := resolveOTLPEndpoint(opts.OTLPEndpoint, opts.Transport)
	switch opts.Transport {
	case TransportHTTP:
		o := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(endpoint)}
		if !opts.OTLPSecure {
			o = append(o, otlpmetrichttp.WithInsecure())
		}
		return otlpmetrichttp.New(ctx, o...)
	default:
		o := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(endpoint)}
		if !opts.OTLPSecure {
			o = append(o, otlpmetricgrpc.WithInsecure())
		}
		return otlpmetricgrpc.New(ctx, o...)
	}
}
