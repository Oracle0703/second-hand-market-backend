package app

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/dto"
	"second-hand-market-backend/backend/internal/model"
)

func (s *Server) handleCreateOrder(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.CreateOrderRequest
	if err := bindJSON(c, &req); err != nil {
		common.Fail(c, err)
		return
	}
	order, err := (inventoryService{}).createOrder(s.DB.WithContext(c.Request.Context()), actor, s.inventoryAudit(c), req)
	if err != nil {
		if bizErr, ok := err.(*common.BizError); ok && bizErr.Code == common.CodeConflict {
			common.Fail(c, bizErr)
			return
		}
		common.Fail(c, err)
		return
	}
	common.Success(c, gin.H{"order_id": order.ID, "order_no": order.OrderNo, "status": order.Status, "product_status": model.ProductLocked})
}

func (s *Server) handleOrderList(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	page, size := parsePage(c)
	query := s.DB.Table("orders AS o").
		Joins("LEFT JOIN products AS p ON p.id = o.product_id").
		Joins("LEFT JOIN categories AS c2 ON c2.id = p.category_id").
		Joins("LEFT JOIN categories AS c1 ON c1.id = c2.parent_id").
		Where("o.merchant_id = ?", actor.MerchantID)
	if v := strings.TrimSpace(c.Query("status")); v != "" {
		query = query.Where("o.status = ?", v)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		query = query.Where("o.order_no LIKE ?", "%"+kw+"%")
	}
	if lv1 := strings.TrimSpace(c.Query("category_level1_id")); lv1 != "" {
		query = query.Where("c1.id = ?", lv1)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	type item struct {
		ID                 uint64    `json:"id"`
		OrderNo            string    `json:"order_no"`
		ProductID          uint64    `json:"product_id"`
		Status             string    `json:"status"`
		DealPriceCent      int       `json:"deal_price_cent"`
		CreatedAt          time.Time `json:"created_at"`
		CategoryLevel1ID   *uint64   `json:"category_level1_id"`
		CategoryLevel1Name *string   `json:"category_level1_name"`
		CategoryLevel2ID   *uint64   `json:"category_level2_id"`
		CategoryLevel2Name *string   `json:"category_level2_name"`
	}
	items := make([]item, 0, size)
	if err := query.Select(
		"o.id, o.order_no, o.product_id, o.status, o.deal_price_cent, o.created_at, c1.id AS category_level1_id, c1.name AS category_level1_name, c2.id AS category_level2_id, c2.name AS category_level2_name",
	).Order("o.id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, common.PageResult[item]{Items: items, Total: total, Page: page, PageSize: size})
}

func (s *Server) handleOrderDetail(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	order, err := s.loadOwnedOrder(nil, id, actor.MerchantID)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var product model.Product
	_ = s.DB.Where("id = ?", order.ProductID).First(&product).Error
	var events []model.OrderEvent
	_ = s.DB.Where("order_id = ?", order.ID).Order("id ASC").Find(&events).Error
	common.Success(c, gin.H{"order_detail": gin.H{"id": order.ID, "order_no": order.OrderNo, "status": order.Status, "deal_price_cent": order.DealPriceCent, "product": gin.H{"id": product.ID, "title": product.Title, "status": product.Status}}, "events": events})
}

func (s *Server) doOrderAction(c *gin.Context, id uint64, toStatus, action string, note *string) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	payload := gin.H{"id": id, "to_status": toStatus, "note": note}
	data, err := s.runWithIdempotency(c, payload, func(idemTx *gorm.DB) (map[string]interface{}, error) {
		return (inventoryService{}).transitionOrder(idemTx, actor, s.inventoryAudit(c), id, toStatus, action, note)
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, data)
}

func (s *Server) handleOrderComplete(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.OrderActionRequest
	if c.Request.ContentLength > 0 {
		if err := bindJSON(c, &req); err != nil {
			common.Fail(c, err)
			return
		}
	}
	s.doOrderAction(c, id, model.OrderCompleted, "order_complete", req.Note)
}

func (s *Server) handleOrderClose(c *gin.Context) {
	id, err := parseUintParam(c, "id")
	if err != nil {
		common.Fail(c, err)
		return
	}
	var req dto.OrderActionRequest
	if c.Request.ContentLength > 0 {
		if err := bindJSON(c, &req); err != nil {
			common.Fail(c, err)
			return
		}
	}
	s.doOrderAction(c, id, model.OrderClosed, "order_close", req.Reason)
}
