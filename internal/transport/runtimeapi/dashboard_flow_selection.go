package runtimeapi

import (
	readmodel "brale-core/internal/readmodel/dashboard"
	"brale-core/internal/store"
)

type dashboardFlowSelection struct {
	Anchor DashboardFlowAnchor
	Gate   *store.GateEventRecord
}

func selectDashboardFlowSelection(selectedSnapshotID uint, hasSelectedSnapshot bool, pos store.PositionRecord, isOpen bool, gates []store.GateEventRecord) (dashboardFlowSelection, bool) {
	selection, gate, ok := readmodel.SelectFlowSelection(selectedSnapshotID, hasSelectedSnapshot, pos, isOpen, gates)
	if !ok {
		return dashboardFlowSelection{}, false
	}
	return dashboardFlowSelection{
		Anchor: DashboardFlowAnchor(selection.Anchor),
		Gate:   gate,
	}, true
}

func selectGateForAnchor(anchor DashboardFlowAnchor, gates []store.GateEventRecord) *store.GateEventRecord {
	selection, gate, _ := readmodel.SelectFlowSelection(anchor.SnapshotID, false, store.PositionRecord{}, false, gates)
	if selection.Anchor.SnapshotID == anchor.SnapshotID && gate != nil {
		return gate
	}
	return gate
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
	return DashboardFlowAnchor(readmodel.ResolveFlowAnchor(pos, isOpen, gates))
}
