package core

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	logsapi "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

func newLoggerProvider(ctx context.Context, opts Options, res *resource.Resource, level slog.Level) (logsapi.LoggerProvider, func(context.Context) error, func(context.Context) error, error) {
	exp, err := newLogExporter(ctx, opts)
	if err != nil {
		return nil, nil, nil, err
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(levelProcessor{
			Processor: sdklog.NewBatchProcessor(exp),
			min:       severityFor(level),
		}),
		sdklog.WithResource(res),
	)
	return lp, lp.Shutdown, lp.ForceFlush, nil
}

// levelProcessor keeps records below min out of the OTLP leg. Enabled is what
// the slog bridge consults, and OnEmit is called independently of it, so both
// have to hold the line.
type levelProcessor struct {
	sdklog.Processor
	min logsapi.Severity
}

var _ sdklog.Processor = levelProcessor{}

func (p levelProcessor) Enabled(_ context.Context, param sdklog.EnabledParameters) bool {
	// The SDK documents an unset severity as indeterminate, and a processor
	// must not turn "cannot tell" into a dropped record.
	if param.Severity == logsapi.SeverityUndefined {
		return true
	}

	return param.Severity >= p.min
}

func (p levelProcessor) OnEmit(ctx context.Context, record *sdklog.Record) error {
	if record.Severity() < p.min {
		return nil
	}
	return p.Processor.OnEmit(ctx, record)
}

// severityFor mirrors the otelslog bridge's slog.Level-to-severity offset, so
// the threshold lands on the same boundary the bridge stamps onto records.
func severityFor(level slog.Level) logsapi.Severity {
	const offset = slog.Level(logsapi.SeverityDebug) - slog.LevelDebug

	return logsapi.Severity(level + offset)
}

func newLogExporter(ctx context.Context, opts Options) (sdklog.Exporter, error) {
	if opts.LogExporter != nil {
		return opts.LogExporter, nil
	}
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
