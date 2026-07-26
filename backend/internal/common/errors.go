package common

import "net/http"

const (
	CodeOK                  = 0
	CodeInvalidArgument     = 10001
	CodeUnauthorized        = 10002
	CodeForbidden           = 10003
	CodeNotFound            = 10004
	CodeInvalidTransition   = 10005
	CodeReviewNotApproved   = 10006
	CodeAccountDisabled     = 10007
	CodeInvalidUpload       = 10008
	CodeRateLimit           = 10009
	CodeConflict            = 10010
	CodeDuplicateSubmit     = 10011
	CodeInvalidFileBinding  = 10012
	CodeUploadQuotaExceeded = 10013
	CodeInternal            = 20001
)

type BizError struct {
	Code       int
	Message    string
	HTTPStatus int
}

func (e *BizError) Error() string {
	return e.Message
}

func NewBizError(code int, message string, httpStatus int) *BizError {
	return &BizError{Code: code, Message: message, HTTPStatus: httpStatus}
}

var (
	ErrInvalidArgument     = NewBizError(CodeInvalidArgument, "invalid argument", http.StatusBadRequest)
	ErrUnauthorized        = NewBizError(CodeUnauthorized, "unauthorized", http.StatusUnauthorized)
	ErrForbidden           = NewBizError(CodeForbidden, "forbidden", http.StatusForbidden)
	ErrNotFound            = NewBizError(CodeNotFound, "resource not found", http.StatusNotFound)
	ErrInvalidTransition   = NewBizError(CodeInvalidTransition, "invalid status transition", http.StatusBadRequest)
	ErrReviewNotApproved   = NewBizError(CodeReviewNotApproved, "review not approved", http.StatusForbidden)
	ErrAccountDisabled     = NewBizError(CodeAccountDisabled, "account disabled", http.StatusForbidden)
	ErrInvalidUpload       = NewBizError(CodeInvalidUpload, "invalid upload file", http.StatusBadRequest)
	ErrUploadTooLarge      = NewBizError(CodeInvalidUpload, "upload file too large", http.StatusRequestEntityTooLarge)
	ErrRateLimit           = NewBizError(CodeRateLimit, "rate limit exceeded", http.StatusTooManyRequests)
	ErrConflict            = NewBizError(CodeConflict, "conflict", http.StatusConflict)
	ErrDuplicateSubmit     = NewBizError(CodeDuplicateSubmit, "duplicate submit", http.StatusConflict)
	ErrInvalidFileBinding  = NewBizError(CodeInvalidFileBinding, "invalid file binding", http.StatusBadRequest)
	ErrUploadQuotaExceeded = NewBizError(CodeUploadQuotaExceeded, "upload quota exceeded", http.StatusConflict)
	ErrInternal            = NewBizError(CodeInternal, "internal error", http.StatusInternalServerError)
)
