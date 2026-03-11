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

func (s *Server) loadOwnedIntent(tx *gorm.DB, intentID, merchantID uint64) (model.BuyerIntent, error) {
	target := s.DB
	if tx != nil {
		target = tx
	}
	var intent model.BuyerIntent
	if err := target.Where("id = ?", intentID).First(&intent).Error; err != nil {
		return model.BuyerIntent{}, s.dbError(err)
	}
	if intent.MerchantID != merchantID {
		return model.BuyerIntent{}, common.ErrForbidden
	}
	return intent, nil
}

func (s *Server) handleMerchantIntentList(c *gin.Context) {
	actor, err := actorFromContext(c)
	if err != nil {
		common.Fail(c, err)
		return
	}
	page, size := parsePage(c)

	query := s.DB.Table("buyer_intents AS i").
		Select("i.id, i.intent_no, i.product_id, i.status, i.contact_name, i.contact_phone, i.contact_wechat, i.created_at, i.updated_at, p.title AS product_title").
		Joins("LEFT JOIN products AS p ON p.id = i.product_id").
		Where("i.merchant_id = ?", actor.MerchantID)
	if v := strings.TrimSpace(c.Query("status")); v != "" {
		query = query.Where("i.status = ?", v)
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		query = query.Where("p.title LIKE ? OR i.contact_phone LIKE ? OR i.contact_wechat LIKE ?", "%"+kw+"%", "%"+kw+"%", "%"+kw+"%")
	}
	if st := strings.TrimSpace(c.Query("start_at")); st != "" {
		query = query.Where("i.created_at >= ?", st)
	}
	if et := strings.TrimSpace(c.Query("end_at")); et != "" {
		query = query.Where("i.created_at <= ?", et)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}

	type item struct {
		ID            uint64    `json:"id"`
		IntentNo      string    `json:"intent_no"`
		ProductID     uint64    `json:"product_id"`
		ProductTitle  string    `json:"product_title"`
		Status        string    `json:"status"`
		ContactName   *string   `json:"contact_name"`
		ContactPhone  *string   `json:"contact_phone"`
		ContactWechat *string   `json:"contact_wechat"`
		CreatedAt     time.Time `json:"created_at"`
		UpdatedAt     time.Time `json:"updated_at"`
	}
	items := make([]item, 0, size)
	if err := query.Order("i.id DESC").Offset((page - 1) * size).Limit(size).Find(&items).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, common.PageResult[item]{Items: items, Total: total, Page: page, PageSize: size})
}

func (s *Server) handleMerchantIntentDetail(c *gin.Context) {
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
	intent, err := s.loadOwnedIntent(nil, id, actor.MerchantID)
	if err != nil {
		common.Fail(c, err)
		return
	}
	var product model.Product
	if err := s.DB.Where("id = ?", intent.ProductID).First(&product).Error; err != nil {
		common.Fail(c, common.ErrInternal)
		return
	}
	common.Success(c, gin.H{"intent": gin.H{
		"id":                intent.ID,
		"intent_no":         intent.IntentNo,
		"status":            intent.Status,
		"buyer_status_text": buyerStatusText(intent.Status),
		"product":           gin.H{"id": product.ID, "title": product.Title, "status": product.Status},
		"contact_name":      intent.ContactName,
		"contact_phone":     intent.ContactPhone,
		"contact_wechat":    intent.ContactWechat,
		"message":           intent.Message,
		"merchant_note":     intent.MerchantNote,
		"close_reason":      intent.CloseReason,
		"created_at":        intent.CreatedAt,
		"updated_at":        intent.UpdatedAt,
	}})
}

func (s *Server) handleMerchantIntentContacted(c *gin.Context) {
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

	payload := gin.H{"id": id, "to_status": model.IntentContacted}
	data, err := s.runWithIdempotency(c, payload, func() (map[string]interface{}, error) {
		result := map[string]interface{}{}
		err := s.DB.Transaction(func(tx *gorm.DB) error {
			intent, err := s.loadOwnedIntent(tx, id, actor.MerchantID)
			if err != nil {
				return err
			}
			if intent.Status == model.IntentContacted {
				result["intent_id"] = intent.ID
				result["from_status"] = intent.Status
				result["to_status"] = intent.Status
				result["idempotent"] = true
				return nil
			}
			if intent.Status != model.IntentNew {
				return common.ErrInvalidTransition
			}
			now := time.Now()
			if err := tx.Model(&model.BuyerIntent{}).Where("id = ?", intent.ID).Updates(map[string]interface{}{
				"status":     model.IntentContacted,
				"handled_by": actor.UserID,
				"handled_at": &now,
			}).Error; err != nil {
				return err
			}
			from, to := intent.Status, model.IntentContacted
			s.writeOperationLog(c, tx, "intent", intent.ID, "merchant_intent_contacted", &from, &to, common.CodeOK, &actor.MerchantID, nil)
			result["intent_id"] = intent.ID
			result["from_status"] = intent.Status
			result["to_status"] = model.IntentContacted
			result["idempotent"] = false
			result["handled_at"] = now.Format(time.RFC3339)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, data)
}

func (s *Server) handleMerchantIntentClose(c *gin.Context) {
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
	var req dto.MerchantIntentCloseRequest
	if c.Request.ContentLength > 0 {
		if err := bindJSON(c, &req); err != nil {
			common.Fail(c, err)
			return
		}
	}

	payload := gin.H{"id": id, "to_status": model.IntentClosed, "reason": req.Reason, "merchant_note": req.MerchantNote}
	data, err := s.runWithIdempotency(c, payload, func() (map[string]interface{}, error) {
		result := map[string]interface{}{}
		err := s.DB.Transaction(func(tx *gorm.DB) error {
			intent, err := s.loadOwnedIntent(tx, id, actor.MerchantID)
			if err != nil {
				return err
			}
			if intent.Status == model.IntentClosed {
				result["intent_id"] = intent.ID
				result["from_status"] = intent.Status
				result["to_status"] = intent.Status
				result["idempotent"] = true
				return nil
			}
			if intent.Status != model.IntentNew && intent.Status != model.IntentContacted {
				return common.ErrInvalidTransition
			}
			now := time.Now()
			if err := tx.Model(&model.BuyerIntent{}).Where("id = ?", intent.ID).Updates(map[string]interface{}{
				"status":        model.IntentClosed,
				"is_open":       false,
				"handled_by":    actor.UserID,
				"handled_at":    &now,
				"closed_at":     &now,
				"close_reason":  req.Reason,
				"merchant_note": req.MerchantNote,
			}).Error; err != nil {
				return err
			}
			from, to := intent.Status, model.IntentClosed
			s.writeOperationLog(c, tx, "intent", intent.ID, "merchant_intent_close", &from, &to, common.CodeOK, &actor.MerchantID, nil)
			result["intent_id"] = intent.ID
			result["from_status"] = intent.Status
			result["to_status"] = model.IntentClosed
			result["idempotent"] = false
			result["closed_at"] = now.Format(time.RFC3339)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return result, nil
	})
	if err != nil {
		common.Fail(c, err)
		return
	}
	common.Success(c, data)
}
