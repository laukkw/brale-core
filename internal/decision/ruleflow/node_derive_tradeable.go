package ruleflow

import (
	"encoding/json"
	"strings"

	"github.com/rulego/rulego/api/types"
)

type DeriveTradeableNode struct{}

func (n *DeriveTradeableNode) Type() string {
	return "brale/derive_tradeable"
}

func (n *DeriveTradeableNode) New() types.Node {
	return &DeriveTradeableNode{}
}

func (n *DeriveTradeableNode) Init(types.Config, types.Configuration) error {
	return nil
}

func (n *DeriveTradeableNode) Destroy() {}

func (n *DeriveTradeableNode) OnMsg(ctx types.RuleContext, msg types.RuleMsg) {
	root, err := readRoot(msg)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	providers := toMap(root["providers"])
	indicator := toMap(providers["indicator"])
	structure := toMap(providers["structure"])
	mechanics := toMap(providers["mechanics"])

	derived := map[string]any{
		"indicator": map[string]any{
			"tradeable": toBool(indicator["momentum_expansion"]) && toBool(indicator["alignment"]) && !toBool(indicator["mean_rev_noise"]),
		},
		"structure": map[string]any{
			"tradeable": toBool(structure["clear_structure"]) && toBool(structure["integrity"]),
		},
		"mechanics": map[string]any{
			"tradeable": !toBool(toMap(mechanics["liquidation_stress"])["value"]),
		},
	}
	structureDirection := strings.ToLower(toString(root["structure_direction"]))
	root["structure"] = map[string]any{
		"direction": structureDirection,
	}

	root["derived"] = derived
	payload, err := json.Marshal(root)
	if err != nil {
		ctx.TellFailure(msg, err)
		return
	}
	msg.DataType = types.JSON
	msg.SetData(string(payload))
	ctx.TellSuccess(msg)
}
