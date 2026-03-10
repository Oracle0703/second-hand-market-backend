package common

import (
	"fmt"
	"sync/atomic"
	"time"
)

var sequence uint64

func BuildBizNo(prefix string) string {
	n := atomic.AddUint64(&sequence, 1)
	return fmt.Sprintf("%s%s%04d", prefix, time.Now().Format("20060102150405"), n%10000)
}
