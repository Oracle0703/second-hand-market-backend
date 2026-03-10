package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"second-hand-market-backend/backend/internal/common"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()
		}
		common.SetRequestID(c, rid)
		c.Writer.Header().Set("X-Request-ID", rid)
		c.Next()
	}
}
