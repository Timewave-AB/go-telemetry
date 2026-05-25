package core

import (
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	logsapi "go.opentelemetry.io/otel/log"
	logsglobal "go.opentelemetry.io/otel/log/global"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	traceapi "go.opentelemetry.io/otel/trace"
)

// Probes used by init_test.go to verify Init never touches OTel globals.
func getDefaultTracerProvider() traceapi.TracerProvider     { return otel.GetTracerProvider() }
func getDefaultMeterProvider() metricapi.MeterProvider      { return otel.GetMeterProvider() }
func getDefaultPropagator() propagation.TextMapPropagator   { return otel.GetTextMapPropagator() }
func getDefaultLoggerProvider() logsapi.LoggerProvider      { return logsglobal.GetLoggerProvider() }
func otelHandle(err error) { otel.Handle(err) }

// installMarkerErrorHandler replaces the OTel error handler with one
// that calls fn. Use restoreDefaultOtelErrorHandler in defer.
func installMarkerErrorHandler(fn func(error)) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(fn))
}

// restoreDefaultOtelErrorHandler reinstates a stderr-writing handler so
// subsequent tests see the same observable behaviour as a fresh process.
func restoreDefaultOtelErrorHandler() {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		fmt.Fprintln(os.Stderr, err)
	}))
}
