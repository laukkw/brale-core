package runtimeapi

import (
	"strconv"
	"strings"

	"brale-core/internal/store"
)

type dashboardFlowSelection struct {
	Anchor DashboardFlowAnchor
	Gate   *store.GateEventRecord
}

func selectDashboardFlowSelection(selectedSnapshotID uint, hasSelectedSnapshot bool, pos store.PositionRecord, isOpen bool, gates []store.GateEventRecord) (dashboardFlowSelection, bool) {
	anchor := resolveDashboardFlowAnchor(pos, isOpen, gates)
	selection := dashboardFlowSelection{
		Anchor: anchor,
		Gate:   selectGateForAnchor(anchor, gates),
	}
	if !hasSelectedSnapshot {
		return selection, true
	}
	selection.Gate = findGateBySnapshotID(selectedSnapshotID, gates)
	if selection.Gate == nil {
		return dashboardFlowSelection{}, false
	}
	selection.Anchor = DashboardFlowAnchor{
		Type:       "selected_round",
		SnapshotID: selectedSnapshotID,
		Confidence: "high",
		Reason:     "selected_by_snapshot_id",
	}
	return selection, true
}

func selectGateForAnchor(anchor DashboardFlowAnchor, gates []store.GateEventRecord) *store.GateEventRecord {
	if anchor.SnapshotID > 0 {
		if gate := selectAnchorGate(anchor.SnapshotID, gates); gate != nil {
			return gate
		}
	}
	return selectLatestFlowGate(gates)
}

func findGateBySnapshotID(snapshotID uint, gates []store.GateEventRecord) *store.GateEventRecord {
	if snapshotID == 0 || len(gates) == 0 {
		return nil
	}
	for idx := range gates {
		if gates[idx].SnapshotID == snapshotID {
			return &gates[idx]
		}
	}
	return nil
}

func selectAnchorGate(anchorSnapshotID uint, gates []store.GateEventRecord) *store.GateEventRecord {
	if len(gates) == 0 {
		return nil
	}
	if anchorSnapshotID > 0 {
		for idx := range gates {
			if gates[idx].SnapshotID == anchorSnapshotID {
				return &gates[idx]
			}
		}
	}
	for idx := range gates {
		if gates[idx].SnapshotID > 0 {
			return &gates[idx]
		}
	}
	return nil
}

func selectLatestFlowGate(gates []store.GateEventRecord) *store.GateEventRecord {
	if len(gates) == 0 {
		return nil
	}
	for idx := range gates {
		if gates[idx].SnapshotID > 0 {
			return &gates[idx]
		}
	}
	return &gates[0]
}

func resolveDashboardFlowAnchor(pos store.PositionRecord, isOpen bool, gates []store.GateEventRecord) DashboardFlowAnchor {
	if isOpen {
		if snapID, ok := resolveOpeningSnapshotID(pos, gates); ok {
			confidence := "medium"
			reason := "matched_by_position_timeline"
			if fromOpenIntentID(snapID, pos.OpenIntentID) {
				confidence = "high"
				reason = "matched_by_open_intent_id"
			}
			return DashboardFlowAnchor{
				Type:       "opening_round",
				SnapshotID: snapID,
				Confidence: confidence,
				Reason:     reason,
			}
		}
		if latest, ok := latestGateSnapshotID(gates); ok {
			return DashboardFlowAnchor{
				Type:       "latest_round",
				SnapshotID: latest,
				Confidence: "low",
				Reason:     "opening_round_unresolved_fallback_latest",
			}
		}
		return DashboardFlowAnchor{
			Type:       "latest_round",
			SnapshotID: 0,
			Confidence: "low",
			Reason:     "no_history_for_open_position",
		}
	}
	if latest, ok := latestGateSnapshotID(gates); ok {
		return DashboardFlowAnchor{
			Type:       "latest_round",
			SnapshotID: latest,
			Confidence: "medium",
			Reason:     "flat_use_latest_round",
		}
	}
	return DashboardFlowAnchor{
		Type:       "latest_round",
		SnapshotID: 0,
		Confidence: "low",
		Reason:     "no_history_available",
	}
}

func latestGateSnapshotID(gates []store.GateEventRecord) (uint, bool) {
	for _, gate := range gates {
		if gate.SnapshotID > 0 {
			return gate.SnapshotID, true
		}
	}
	return 0, false
}

func resolveOpeningSnapshotID(pos store.PositionRecord, gates []store.GateEventRecord) (uint, bool) {
	if len(gates) == 0 {
		return 0, false
	}
	if openIntentSnapshot, ok := parseSnapshotIDFromOpenIntentID(pos.OpenIntentID); ok {
		for _, gate := range gates {
			if gate.SnapshotID == openIntentSnapshot {
				return gate.SnapshotID, true
			}
		}
	}
	anchorTimestamp := int64(0)
	if !pos.CreatedAt.IsZero() {
		anchorTimestamp = pos.CreatedAt.Unix()
	}
	if anchorTimestamp <= 0 && !pos.UpdatedAt.IsZero() {
		anchorTimestamp = pos.UpdatedAt.Unix()
	}
	bestSnapshot := uint(0)
	bestTimestamp := int64(0)
	for _, gate := range gates {
		if gate.SnapshotID == 0 || gate.Timestamp <= 0 {
			continue
		}
		if anchorTimestamp > 0 {
			if gate.Timestamp > anchorTimestamp {
				continue
			}
			if gate.Timestamp >= bestTimestamp {
				bestTimestamp = gate.Timestamp
				bestSnapshot = gate.SnapshotID
			}
			continue
		}
		if bestSnapshot == 0 || gate.Timestamp < bestTimestamp {
			bestTimestamp = gate.Timestamp
			bestSnapshot = gate.SnapshotID
		}
	}
	if bestSnapshot > 0 {
		return bestSnapshot, true
	}
	return 0, false
}

func parseSnapshotIDFromOpenIntentID(openIntentID string) (uint, bool) {
	raw := strings.TrimSpace(openIntentID)
	if raw == "" {
		return 0, false
	}
	tokens := strings.FieldsFunc(raw, func(r rune) bool {
		return r < '0' || r > '9'
	})
	for _, token := range tokens {
		if len(token) < 9 {
			continue
		}
		parsed, err := strconv.ParseUint(token, 10, 64)
		if err != nil || parsed == 0 {
			continue
		}
		return uint(parsed), true
	}
	return 0, false
}

func fromOpenIntentID(snapshotID uint, openIntentID string) bool {
	parsed, ok := parseSnapshotIDFromOpenIntentID(openIntentID)
	return ok && parsed == snapshotID
}
