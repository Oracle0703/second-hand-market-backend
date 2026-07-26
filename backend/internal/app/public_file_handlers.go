package app

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (s *Server) handlePublicProductImage(c *gin.Context) {
	objectKey, err := normalizeObjectKey(c.Param("path"))
	if err != nil || !strings.HasPrefix(objectKey, "product_image/") {
		c.Status(http.StatusNotFound)
		return
	}

	file, stat, err := s.openLocalRegularFile(objectKey)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	defer file.Close()

	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, stat.Name(), stat.ModTime(), file)
}
