// 本文件主要内容：定义 Agent 启用项在 app 层的传递结构。

package decision

type AgentEnabled struct {
	Indicator bool
	Structure bool
	Mechanics bool
}

func (e AgentEnabled) Count() int {
	count := 0
	if e.Indicator {
		count++
	}
	if e.Structure {
		count++
	}
	if e.Mechanics {
		count++
	}
	return count
}
