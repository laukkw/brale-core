// 本文件主要内容：封装持仓与下单意图的服务依赖与共享错误。
package position

import (
	"errors"

	"brale-core/internal/execution"
	"brale-core/internal/store"
)

type PositionService struct {
	Store     store.Store
	Executor  execution.Executor
	Notifier  Notifier
	Cache     *PositionCache
	PlanCache *PlanCache
	RiskPlans *RiskPlanService
}

var ErrPositionActive = errors.New("position already active")
var ErrPositionNotArmed = errors.New("position not armed")
