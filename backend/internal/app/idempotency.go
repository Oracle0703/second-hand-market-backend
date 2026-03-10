package app

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

func (s *Server) runWithIdempotency(c *gin.Context, payload interface{}, fn func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		return fn()
	}
	actor, _ := common.GetActor(c)
	raw, _ := json.Marshal(payload)
	hash := common.SHA256(string(raw))

	var record model.IdempotencyRecord
	err := s.DB.Where("idem_key = ? AND operator_id = ? AND path = ?", key, actor.UserID, c.FullPath()).First(&record).Error
	if err == nil {
		if record.RequestHash != hash {
			return nil, common.ErrDuplicateSubmit
		}
		var data map[string]interface{}
		if uErr := json.Unmarshal(record.ResponseRaw, &data); uErr != nil {
			return nil, common.ErrInternal
		}
		data["idempotent"] = true
		return data, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, common.ErrInternal
	}

	data, runErr := fn()
	if runErr != nil {
		return nil, runErr
	}
	enc, _ := json.Marshal(data)
	_ = s.DB.Create(&model.IdempotencyRecord{
		IdemKey:     key,
		OperatorID:  actor.UserID,
		Path:        c.FullPath(),
		RequestHash: hash,
		ResultCode:  common.CodeOK,
		ResponseRaw: enc,
	}).Error
	return data, nil
}
