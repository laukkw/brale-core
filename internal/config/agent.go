// 本文件主要内容：解析 Agent 启用配置并提供统一访问方法。

package config

import "fmt"

type AgentEnabled struct {
	Indicator bool
	Structure bool
	Mechanics bool
}

func ResolveAgentEnabled(cfg AgentConfig) (AgentEnabled, error) {
	indicator, err := requireAgentBool(cfg.Indicator, "agent.indicator")
	if err != nil {
		return AgentEnabled{}, err
	}
	structure, err := requireAgentBool(cfg.Structure, "agent.structure")
	if err != nil {
		return AgentEnabled{}, err
	}
	mechanics, err := requireAgentBool(cfg.Mechanics, "agent.mechanics")
	if err != nil {
		return AgentEnabled{}, err
	}
	if !indicator && !structure && !mechanics {
		return AgentEnabled{}, fmt.Errorf("agent must enable at least one stage")
	}
	return AgentEnabled{
		Indicator: indicator,
		Structure: structure,
		Mechanics: mechanics,
	}, nil
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

func requireAgentBool(v *bool, name string) (bool, error) {
	if v == nil {
		return false, fmt.Errorf("%s is required", name)
	}
	return *v, nil
}
