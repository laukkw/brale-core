// 本文件主要内容：定义持仓状态枚举与常用集合。

package position

const (
	PositionNew             = "NEW"
	PositionOpenArmed       = "OPEN_ARMED"
	PositionOpenSubmitting  = "OPEN_SUBMITTING"
	PositionOpenPending     = "OPEN_PENDING"
	PositionOpenAborting    = "OPEN_ABORTING"
	PositionOpenAborted     = "OPEN_ABORTED"
	PositionOpenActive      = "OPEN_ACTIVE"
	PositionCloseArmed      = "CLOSE_ARMED"
	PositionCloseSubmitting = "CLOSE_SUBMITTING"
	PositionClosePending    = "CLOSE_PENDING"
	PositionClosed          = "CLOSED"
	PositionError           = "ERROR"
)

var OpenPositionStatuses = []string{
	PositionOpenArmed,
	PositionOpenSubmitting,
	PositionOpenPending,
	PositionOpenAborting,
	PositionOpenActive,
	PositionCloseArmed,
	PositionCloseSubmitting,
	PositionClosePending,
}

var ReconcilePositionStatuses = []string{
	PositionOpenArmed,
	PositionOpenSubmitting,
	PositionOpenPending,
	PositionOpenAborting,
	PositionOpenAborted,
	PositionOpenActive,
	PositionCloseArmed,
	PositionCloseSubmitting,
	PositionClosePending,
}
