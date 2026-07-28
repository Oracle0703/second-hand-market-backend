package app

import (
	"encoding/json"
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"second-hand-market-backend/backend/internal/common"
	"second-hand-market-backend/backend/internal/model"
)

var errIdempotencyClaimConflict = errors.New("idempotency claim conflict")

type idempotentOperation func(tx *gorm.DB) (map[string]interface{}, error)

type idempotencyScope struct {
	Key         string
	OperatorID  uint64
	Path        string
	RequestHash string
}

func (s *Server) runWithIdempotency(c *gin.Context, payload interface{}, fn idempotentOperation) (map[string]interface{}, error) {
	key := c.GetHeader("Idempotency-Key")
	if key == "" {
		return s.runIdempotentTransaction(fn)
	}
	actor, ok := common.GetActor(c)
	if !ok {
		return nil, common.ErrInternal
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, common.ErrInternal
	}
	scope := idempotencyScope{
		Key:         key,
		OperatorID:  actor.UserID,
		Path:        c.FullPath(),
		RequestHash: common.SHA256(string(raw)),
	}

	var data map[string]interface{}
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		record := model.IdempotencyRecord{
			IdemKey:     scope.Key,
			OperatorID:  scope.OperatorID,
			Path:        scope.Path,
			RequestHash: scope.RequestHash,
			ResultCode:  common.CodeOK,
			ResponseRaw: datatypes.JSON([]byte("null")),
		}
		if createErr := tx.Create(&record).Error; createErr != nil {
			if errors.Is(createErr, gorm.ErrDuplicatedKey) {
				return errIdempotencyClaimConflict
			}
			return common.ErrInternal
		}
		result, runErr := fn(tx)
		if runErr != nil {
			return runErr
		}
		if result == nil {
			return common.ErrInternal
		}
		encoded, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return common.ErrInternal
		}
		update := tx.Model(&model.IdempotencyRecord{}).
			Where("id = ?", record.ID).
			Updates(map[string]interface{}{
				"result_code":  common.CodeOK,
				"response_raw": datatypes.JSON(encoded),
			})
		if update.Error != nil || update.RowsAffected != 1 {
			return common.ErrInternal
		}
		data = result
		return nil
	})
	if errors.Is(err, errIdempotencyClaimConflict) {
		return s.replayIdempotencyResult(scope)
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Server) runIdempotentTransaction(fn idempotentOperation) (map[string]interface{}, error) {
	var data map[string]interface{}
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		result, runErr := fn(tx)
		if runErr != nil {
			return runErr
		}
		if result == nil {
			return common.ErrInternal
		}
		data = result
		return nil
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *Server) replayIdempotencyResult(scope idempotencyScope) (map[string]interface{}, error) {
	var record model.IdempotencyRecord
	if err := s.DB.Where("idem_key = ? AND operator_id = ? AND path = ?", scope.Key, scope.OperatorID, scope.Path).First(&record).Error; err != nil {
		return nil, common.ErrInternal
	}
	if record.RequestHash != scope.RequestHash {
		return nil, common.ErrDuplicateSubmit
	}
	if record.ResultCode != common.CodeOK {
		return nil, common.ErrInternal
	}
	var data map[string]interface{}
	if err := json.Unmarshal(record.ResponseRaw, &data); err != nil || data == nil {
		return nil, common.ErrInternal
	}
	data["idempotent"] = true
	return data, nil
}
