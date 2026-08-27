package subscriber

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		attrs := []slog.Attr{
			slog.String("client_ip", c.ClientIP()),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("protocol", c.Request.Proto),
			slog.Int("status", status),
			slog.Int64("latency_ms", latency.Milliseconds()),
			slog.String("user_agent", c.Request.UserAgent()),
		}

		switch {
		case status >= 500:
			logger.LogAttrs(c, slog.LevelError, "HTTP request", attrs...)
		case status >= 400:
			logger.LogAttrs(c, slog.LevelWarn, "HTTP request", attrs...)
		default:
			logger.LogAttrs(c, slog.LevelInfo, "HTTP request", attrs...)
		}
	}
}
