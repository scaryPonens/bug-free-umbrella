package mcp

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const defaultRequestTimeout = 5 * time.Second

type ServerConfig struct {
	RequestTimeout time.Duration
}

func NewServer(tracer trace.Tracer, prices PriceReader, signals SignalReaderWriter, cfg ServerConfig) *sdkmcp.Server {
	requestTimeout := cfg.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "bug-free-umbrella-mcp",
		Version: "1.0.0",
	}, &sdkmcp.ServerOptions{
		Instructions: "Use these tools/resources to inspect market data and deterministic trade signals.",
		Logger:       slog.Default(),
	})

	srv.AddReceivingMiddleware(timeoutMiddleware(requestTimeout))
	if tracer != nil {
		srv.AddReceivingMiddleware(tracingMiddleware(tracer))
	}

	registerTools(srv, prices, signals)
	registerResources(srv, prices, signals)
	return srv
}

func NewHTTPTransportHandler(server *sdkmcp.Server, cfg HTTPHandlerConfig) http.Handler {
	base := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		return server
	}, &sdkmcp.StreamableHTTPOptions{})
	return wrapHTTPHandler(base, cfg)
}

func timeoutMiddleware(timeout time.Duration) sdkmcp.Middleware {
	return func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, req sdkmcp.Request) (sdkmcp.Result, error) {
			if timeout <= 0 {
				return next(ctx, method, req)
			}
			timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return next(timeoutCtx, method, req)
		}
	}
}

func tracingMiddleware(tracer trace.Tracer) sdkmcp.Middleware {
	return func(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
		return func(ctx context.Context, method string, req sdkmcp.Request) (sdkmcp.Result, error) {
			spanName := mcpSpanName(method, req)
			ctx, span := tracer.Start(ctx, spanName)
			span.SetAttributes(attribute.String("mcp.method", method))
			defer span.End()

			if callReq, ok := req.(*sdkmcp.CallToolRequest); ok {
				span.SetAttributes(attribute.String("mcp.tool", strings.TrimSpace(callReq.Params.Name)))
			}
			if readReq, ok := req.(*sdkmcp.ReadResourceRequest); ok {
				span.SetAttributes(attribute.String("mcp.resource.uri", strings.TrimSpace(readReq.Params.URI)))
			}

			result, err := next(ctx, method, req)
			if err != nil {
				span.RecordError(err)
			}
			return result, err
		}
	}
}

func mcpSpanName(method string, req sdkmcp.Request) string {
	switch method {
	case "tools/call":
		if callReq, ok := req.(*sdkmcp.CallToolRequest); ok {
			name := strings.TrimSpace(callReq.Params.Name)
			if name != "" {
				return "mcp.tool." + strings.ReplaceAll(name, "/", ".")
			}
		}
		return "mcp.tool.call"
	case "resources/read":
		return "mcp.resource.read"
	default:
		return "mcp." + strings.ReplaceAll(method, "/", ".")
	}
}
