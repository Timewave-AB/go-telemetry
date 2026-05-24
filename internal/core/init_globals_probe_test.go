package core

import (
	"go.opentelemetry.io/otel"
	logsglobal "go.opentelemetry.io/otel/log/global"
	logsapi "go.opentelemetry.io/otel/log"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	traceapi "go.opentelemetry.io/otel/trace"
)

// Probes used by init_test.go to verify Init never touches OTel globals.
func getDefaultTracerProvider() traceapi.TracerProvider { return otel.GetTracerProvider() }
func getDefaultMeterProvider() metricapi.MeterProvider  { return otel.GetMeterProvider() }
func getDefaultPropagator() propagation.TextMapPropagator { return otel.GetTextMapPropagator() }
func getDefaultLoggerProvider() logsapi.LoggerProvider  { return logsglobal.GetLoggerProvider() }
