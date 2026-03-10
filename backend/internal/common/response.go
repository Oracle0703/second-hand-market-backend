package common

import (
	"errors"

	"github.com/gin-gonic/gin"
)

type APIResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	RequestID string      `json:"request_id"`
	Data      interface{} `json:"data,omitempty"`
}

func Success(c *gin.Context, data interface{}) {
	c.JSON(200, APIResponse{Code: CodeOK, Message: "OK", RequestID: RequestIDFromContext(c), Data: data})
}

func Fail(c *gin.Context, err error) {
	var bizErr *BizError
	if errors.As(err, &bizErr) {
		c.JSON(bizErr.HTTPStatus, APIResponse{Code: bizErr.Code, Message: bizErr.Message, RequestID: RequestIDFromContext(c)})
		return
	}
	c.JSON(500, APIResponse{Code: CodeInternal, Message: "internal error", RequestID: RequestIDFromContext(c)})
}

type PageResult[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}
