package observability

import (
	"log/slog"
	"net/http"
	"time"
)

// Middleware is the standard net/http decorator shape used across the API.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware so that the first argument is the outermost layer,
// which is the order they are read in.
func Chain(h http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// responseRecorder captures what the handler actually wrote. Without it the
// access log cannot report status or size.
type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
		r.ResponseWriter.WriteHeader(status)
	}
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *responseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// RequestContext assigns a request ID, continues or starts a trace, and binds a
// request-scoped logger. It runs before everything else so that even a panic is
// logged with identity attached.
func RequestContext(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(HeaderRequestID)
			if requestID == "" {
				requestID = NewRequestID()
			}

			trace, ok := ParseTraceParent(r.Header.Get(HeaderTraceParent))
			if ok {
				trace = trace.Child()
			} else {
				trace = NewTrace()
			}

			scoped := logger.With(
				slog.String("request_id", requestID),
				slog.String("trace_id", trace.TraceID),
				slog.String("span_id", trace.SpanID),
			)

			ctx := WithLogger(WithTrace(WithRequestID(r.Context(), requestID), trace), scoped)

			w.Header().Set(HeaderRequestID, requestID)
			w.Header().Set(HeaderTraceParent, trace.TraceParent())

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AccessLog records method, route, status, latency and response size.
func AccessLog() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &responseRecorder{ResponseWriter: w}

			next.ServeHTTP(recorder, r)

			if recorder.status == 0 {
				recorder.status = http.StatusOK
			}

			level := slog.LevelInfo
			switch {
			case recorder.status >= 500:
				level = slog.LevelError
			case recorder.status >= 400:
				level = slog.LevelWarn
			}

			LoggerFrom(r.Context()).Log(r.Context(), level, "http_request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", recorder.status),
				slog.Int64("latency_ms", time.Since(started).Milliseconds()),
				slog.Int("bytes", recorder.bytes),
			)
		})
	}
}

// CORS allows a browser-based client — currently the admin dashboard, a
// React+Vite app served from a different origin than the API — to call it.
// Mobile apps never need this: React Native's fetch does not enforce CORS,
// so without a browser client this middleware would have nothing to do.
//
// An empty allowlist is a valid, safe default: it answers every preflight
// with no Access-Control-Allow-Origin header, which every browser treats as
// "not permitted" — the same as CORS not being configured at all.
func CORS(allowedOrigins []string) Middleware {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[origin] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowed[origin] {
				// Origin is echoed rather than "*" so the browser will still
				// send the Authorization header a Bearer-token client needs —
				// the wildcard forbids that pairing outright.
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Recover turns a panic into a 500 rather than a dropped connection. onPanic
// lets the caller render the platform's standard error envelope without this
// package importing httpx.
func Recover(onPanic func(http.ResponseWriter, *http.Request, any)) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					LoggerFrom(r.Context()).Error("panic recovered",
						slog.Any("panic", recovered),
						slog.String("path", r.URL.Path),
					)
					onPanic(w, r, recovered)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
