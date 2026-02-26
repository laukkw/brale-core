// 本文件主要内容：定义决策模式及解析方法。

package decisionmode

type Mode string

const (
	ModeFlat       Mode = "FLAT"
	ModeInPosition Mode = "IN_POSITION"
)

func Resolve(inPosition bool) Mode {
	if inPosition {
		return ModeInPosition
	}
	return ModeFlat
}
