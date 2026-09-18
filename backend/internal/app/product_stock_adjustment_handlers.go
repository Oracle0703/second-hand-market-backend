package app

import (
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
)

func (s *Server) handleProductStockAdjustment(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.AdjustProductStockRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) < 2 || len(req.Reason) > 255 {
		common.Fail(c, common.ErrInvalidArgument)
		return
	}

	payload := gin.H{"id": id, "adjustment_type": req.AdjustmentType, "quantity": req.Quantity, "all_remaining": req.AllRemaining, "reason": req.Reason}
	data, err := s.runWithIdempotency(c, payload, func(idemTx *gorm.DB) (map[string]interface{}, error) {
		return (inventoryService{}).adjustStock(idemTx, actor, s.inventoryAudit(c), id, req)
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, data)
}
