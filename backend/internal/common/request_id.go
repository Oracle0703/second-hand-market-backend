package common

import "github.com/gin-gonic/gin"

const requestIDKey = "request_id"

func SetRequestID(c *gin.Context, id string) {
	c.Set(requestIDKey, id)
}

func RequestIDFromContext(c *gin.Context) string {
	v, ok := c.Get(requestIDKey)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}
