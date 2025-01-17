package otelfiber

import (
	"context"
	"net/http"

	"github.com/gofiber/fiber/v2"

	otelcontrib "go.opentelemetry.io/contrib"
	"go.opentelemetry.io/otel"

	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	tracerKey  = "otel-go-contrib-tracer-gofiber-fiber"
	tracerName = "go.opentelemetry.io/contrib/instrumentation/github.com/gofiber/fiber/otelfiber"
)

// Middleware returns fiber handler which will trace incoming requests.
func Middleware(service string, opts ...Option) fiber.Handler {
	cfg := config{}
	for _, opt := range opts {
		opt.apply(&cfg)
	}
	if cfg.TracerProvider == nil {
		cfg.TracerProvider = otel.GetTracerProvider()
	}
	tracer := cfg.TracerProvider.Tracer(
		tracerName,
		oteltrace.WithInstrumentationVersion(otelcontrib.SemVersion()),
	)
	if cfg.Propagators == nil {
		cfg.Propagators = otel.GetTextMapPropagator()
	}

	return func(c *fiber.Ctx) error {
		c.Locals(tracerKey, tracer)
		savedCtx, cancel := context.WithCancel(c.Context())

		defer func() {
			c.SetUserContext(savedCtx)
			cancel()
		}()

		reqHeader := make(http.Header)
		c.Request().Header.VisitAll(func(k, v []byte) {
			reqHeader.Add(string(k), string(v))
		})

		ctx := cfg.Propagators.Extract(savedCtx, propagation.HeaderCarrier(reqHeader))
		opts := []oteltrace.SpanStartOption{
			oteltrace.WithAttributes(
				semconv.HTTPServerNameKey.String(service),
				semconv.HTTPMethodKey.String(c.Method()),
				semconv.HTTPTargetKey.String(string(c.Request().RequestURI())),
				semconv.HTTPURLKey.String(c.OriginalURL()),
				semconv.NetHostIPKey.String(c.IP()),
				semconv.NetHostNameKey.String(c.Hostname()),
				semconv.HTTPUserAgentKey.String(string(c.Request().Header.UserAgent())),
				semconv.HTTPRequestContentLengthKey.Int(c.Request().Header.ContentLength()),
				semconv.HTTPSchemeKey.String(c.Protocol()),
				semconv.NetTransportTCP),
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		}
		if len(c.IPs()) > 0 {
			opts = append(opts, oteltrace.WithAttributes(semconv.HTTPClientIPKey.String(c.IPs()[0])))
		}
		// temporary set to c.Path() first
		// update with c.Route().Path after c.Next() is called
		// to get pathRaw
		spanName := c.Path()
		ctx, span := tracer.Start(ctx, spanName, opts...)
		defer span.End()

		// pass the span through userContext
		c.SetUserContext(ctx)

		// serve the request to the next middleware
		err := c.Next()

		span.SetName(c.Route().Path)
		span.SetAttributes(semconv.HTTPRouteKey.String(c.Route().Path))

		if err != nil {
			span.RecordError(err)
			// invokes the registered HTTP error handler
			// to get the correct response status code
			_ = c.App().Config().ErrorHandler(c, err)
		}

		attrs := semconv.HTTPAttributesFromHTTPStatusCode(c.Response().StatusCode())
		spanStatus, spanMessage := semconv.SpanStatusFromHTTPStatusCode(c.Response().StatusCode())
		span.SetAttributes(attrs...)
		span.SetStatus(spanStatus, spanMessage)

		return nil
	}
}
