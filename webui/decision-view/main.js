// 本文件主要内容：基于 force-graph / 3d-force-graph 渲染 2D/3D 决策链，并提供 org-roam 风格交互。
const BASE_PATH = normalizeBase(window.__DECISION_VIEW_BASE__ || "/decision-view");
const API_BASE = `${BASE_PATH}/api`;

const STAGE_COLORS = {
  origin: "#7dd3fc",
  round: "#39d0ff",
  provider: "#8fb4ff",
  indicator: "#39d0ff",
  structure: "#30e6c2",
  mechanics: "#ffd666",
  gate: "#ff5c8d",
  execution: "#cccccc",
  missing: "#8a90a5",
};

const DIRECTION_COLORS = {
  long: "#00ff88",
  short: "#ff4444",
  hold: "#4488ff",
};

const DOUBLE_CLICK_MS = 260;
const GRAPH_MODE_2D = "2d";
const GRAPH_MODE_3D = "3d";
const DEFAULT_GRAPH_MODE = GRAPH_MODE_3D;

const state = {
  configBySymbol: new Map(),
  roundsBySymbol: new Map(),
  symbols: [],

  graph: null,
  graphsByMode: new Map(),
  graphRootsByMode: new Map(),
  graphMode: DEFAULT_GRAPH_MODE,
  viewportByMode: new Map(),
  currentGraphData: { nodes: [], links: [] },
  hasFit: false,
  expandedRounds: new Set(),
  defaultExpandedRounds: new Set(),
  parentByNodeId: new Map(),

  visibleNodesById: new Map(),
  adjacentByNodeId: new Map(),
  linkKeysByNodeId: new Map(),
  highlightNodeIds: new Set(),
  highlightLinkKeys: new Set(),
  nodeObjects: new Map(),

  activeNodeId: null,
  hoverNodeId: null,
  pendingHoverId: null,
  hoverFrameRequested: false,

  followEnabled: false,
  followOffset: null,

  lastClick: null,
  ignoreClickUntil: 0,
  dragging: false,
  rebuildFrameRequested: false,
  pendingRebuildFit: false,

  settings: {
    filters: {
      hideOrphans: false,
      showDailies: true,
      showMissingNodes: true,
      tagQuery: "",
    },
    physics: {
      charge: -120,
      linkDistance: 24,
      linkStrengthScale: 1,
    },
    visual: {
      showLabels: true,
      linkWidthScale: 1,
    },
    behavior: {
      doubleClickExpand: true,
      fadeUnrelated: true,
      autoFitAfterExpand: false,
    },
  },

  ui: {
    graphContainer: null,
    status: null,
    modeToggle: null,
    modeToggleLabel: null,
    followToggle: null,
    fitBtn: null,
    resetBtn: null,
    leftPanelToggle: null,
    rightPanelToggle: null,
    leftPanelHandle: null,
    rightPanelHandle: null,
    previewTitle: null,
    previewContent: null,
    filterOrphans: null,
    filterDailies: null,
    filterMissing: null,
    filterTagQuery: null,
    physicsCharge: null,
    physicsLinkDistance: null,
    physicsLinkStrength: null,
    visualLabels: null,
    visualLinkScale: null,
    behaviorDoubleClickExpand: null,
    behaviorFadeUnrelated: null,
    behaviorAutoFit: null,
  },
};

document.addEventListener("DOMContentLoaded", init);

async function init() {
  cacheDOM();
  bindUIEvents();
  setStatus("加载中…");
  try {
    const [configGraph, chains] = await Promise.all([
      fetchJSON(`${API_BASE}/config-graph`),
      fetchJSON(`${API_BASE}/chains`),
    ]);
    ingestConfig(configGraph);
    ingestChains(chains);
    initGraphs();
    applyPhysicsSettings();
    rebuildVisibleGraph({ fit: true });
    renderPreview();
    const modeText = state.graphsByMode.has(GRAPH_MODE_2D) ? "2D/3D" : "3D";
    setStatus(`载入 ${state.symbols.length} 个币种 / ${countRounds()} 轮 · ${modeText}`);
  } catch (err) {
    console.error(err);
    setStatus("加载失败，请检查接口", true);
  }
}

function cacheDOM() {
  state.ui.graphContainer = document.getElementById("graph-container");
  state.ui.status = document.getElementById("status-pill");
  state.ui.modeToggle = document.getElementById("dimension-toggle");
  state.ui.modeToggleLabel = document.querySelector('label[for="dimension-toggle"] span');
  state.ui.followToggle = document.getElementById("follow-toggle");
  state.ui.fitBtn = document.getElementById("fit-btn");
  state.ui.resetBtn = document.getElementById("reset-btn");
  state.ui.leftPanelToggle = document.getElementById("left-panel-toggle");
  state.ui.rightPanelToggle = document.getElementById("right-panel-toggle");
  state.ui.leftPanelHandle = document.getElementById("left-panel-handle");
  state.ui.rightPanelHandle = document.getElementById("right-panel-handle");
  state.ui.previewTitle = document.getElementById("preview-title");
  state.ui.previewContent = document.getElementById("preview-content");
  state.ui.filterOrphans = document.getElementById("filter-orphans");
  state.ui.filterDailies = document.getElementById("filter-dailies");
  state.ui.filterMissing = document.getElementById("filter-missing");
  state.ui.filterTagQuery = document.getElementById("filter-tag-query");
  state.ui.physicsCharge = document.getElementById("physics-charge");
  state.ui.physicsLinkDistance = document.getElementById("physics-link-distance");
  state.ui.physicsLinkStrength = document.getElementById("physics-link-strength");
  state.ui.visualLabels = document.getElementById("visual-labels");
  state.ui.visualLinkScale = document.getElementById("visual-link-scale");
  state.ui.behaviorDoubleClickExpand = document.getElementById("behavior-double-click-expand");
  state.ui.behaviorFadeUnrelated = document.getElementById("behavior-fade-unrelated");
  state.ui.behaviorAutoFit = document.getElementById("behavior-auto-fit");
}

function bindUIEvents() {
  hydrateSettingsFromControls();

  if (state.ui.modeToggle && !state.ui.modeToggle.checked) {
    state.ui.modeToggle.checked = true;
  }
  syncModeToggleLabel();

  if (state.ui.modeToggle) {
    state.ui.modeToggle.addEventListener("change", (evt) => {
      const nextMode = evt.target?.checked ? GRAPH_MODE_3D : GRAPH_MODE_2D;
      setGraphMode(nextMode, { preferActiveFocus: true });
    });
  }
  if (state.ui.followToggle) {
    state.ui.followToggle.addEventListener("change", (evt) => {
      setFollowMode(Boolean(evt.target?.checked));
    });
  }
  if (state.ui.fitBtn) {
    state.ui.fitBtn.addEventListener("click", () => fitGraph(420));
  }
  if (state.ui.resetBtn) {
    state.ui.resetBtn.addEventListener("click", () => resetView());
  }
  if (state.ui.leftPanelToggle) {
    state.ui.leftPanelToggle.addEventListener("click", () => {
      setPanelCollapsed("left", true);
    });
  }
  if (state.ui.rightPanelToggle) {
    state.ui.rightPanelToggle.addEventListener("click", () => {
      setPanelCollapsed("right", true);
    });
  }
  if (state.ui.leftPanelHandle) {
    state.ui.leftPanelHandle.addEventListener("click", () => {
      setPanelCollapsed("left", false);
    });
  }
  if (state.ui.rightPanelHandle) {
    state.ui.rightPanelHandle.addEventListener("click", () => {
      setPanelCollapsed("right", false);
    });
  }
  if (state.ui.previewContent) {
    state.ui.previewContent.addEventListener("click", handlePreviewClick);
  }

  if (state.ui.filterOrphans) {
    state.ui.filterOrphans.addEventListener("change", (evt) => {
      state.settings.filters.hideOrphans = Boolean(evt.target?.checked);
      scheduleGraphRebuild();
    });
  }
  if (state.ui.filterDailies) {
    state.ui.filterDailies.addEventListener("change", (evt) => {
      state.settings.filters.showDailies = Boolean(evt.target?.checked);
      scheduleGraphRebuild();
    });
  }
  if (state.ui.filterMissing) {
    state.ui.filterMissing.addEventListener("change", (evt) => {
      state.settings.filters.showMissingNodes = Boolean(evt.target?.checked);
      scheduleGraphRebuild();
    });
  }
  if (state.ui.filterTagQuery) {
    state.ui.filterTagQuery.addEventListener("input", (evt) => {
      state.settings.filters.tagQuery = String(evt.target?.value || "").trim().toLowerCase();
      scheduleGraphRebuild();
    });
  }

  if (state.ui.physicsCharge) {
    state.ui.physicsCharge.addEventListener("input", (evt) => {
      state.settings.physics.charge = Number(evt.target?.value || -120);
      applyPhysicsSettings();
    });
  }
  if (state.ui.physicsLinkDistance) {
    state.ui.physicsLinkDistance.addEventListener("input", (evt) => {
      state.settings.physics.linkDistance = Number(evt.target?.value || 24);
      applyPhysicsSettings();
    });
  }
  if (state.ui.physicsLinkStrength) {
    state.ui.physicsLinkStrength.addEventListener("input", (evt) => {
      state.settings.physics.linkStrengthScale = Number(evt.target?.value || 1);
      applyPhysicsSettings();
    });
  }

  if (state.ui.visualLabels) {
    state.ui.visualLabels.addEventListener("change", (evt) => {
      state.settings.visual.showLabels = Boolean(evt.target?.checked);
      refreshEmphasis();
    });
  }
  if (state.ui.visualLinkScale) {
    state.ui.visualLinkScale.addEventListener("input", (evt) => {
      state.settings.visual.linkWidthScale = Number(evt.target?.value || 1);
      refreshEmphasis();
    });
  }

  if (state.ui.behaviorDoubleClickExpand) {
    state.ui.behaviorDoubleClickExpand.addEventListener("change", (evt) => {
      state.settings.behavior.doubleClickExpand = Boolean(evt.target?.checked);
    });
  }
  if (state.ui.behaviorFadeUnrelated) {
    state.ui.behaviorFadeUnrelated.addEventListener("change", (evt) => {
      state.settings.behavior.fadeUnrelated = Boolean(evt.target?.checked);
      refreshEmphasis();
    });
  }
  if (state.ui.behaviorAutoFit) {
    state.ui.behaviorAutoFit.addEventListener("change", (evt) => {
      state.settings.behavior.autoFitAfterExpand = Boolean(evt.target?.checked);
    });
  }

  window.addEventListener("resize", resizeAllGraphs);
}

function hydrateSettingsFromControls() {
  state.settings.filters.hideOrphans = Boolean(state.ui.filterOrphans?.checked);
  state.settings.filters.showDailies = state.ui.filterDailies ? Boolean(state.ui.filterDailies.checked) : true;
  state.settings.filters.showMissingNodes = state.ui.filterMissing
    ? Boolean(state.ui.filterMissing.checked)
    : true;
  state.settings.filters.tagQuery = String(state.ui.filterTagQuery?.value || "").trim().toLowerCase();

  state.settings.physics.charge = Number(state.ui.physicsCharge?.value || -120);
  state.settings.physics.linkDistance = Number(state.ui.physicsLinkDistance?.value || 24);
  state.settings.physics.linkStrengthScale = Number(state.ui.physicsLinkStrength?.value || 1);

  state.settings.visual.showLabels = state.ui.visualLabels ? Boolean(state.ui.visualLabels.checked) : true;
  state.settings.visual.linkWidthScale = Number(state.ui.visualLinkScale?.value || 1);

  state.settings.behavior.doubleClickExpand = state.ui.behaviorDoubleClickExpand
    ? Boolean(state.ui.behaviorDoubleClickExpand.checked)
    : true;
  state.settings.behavior.fadeUnrelated = state.ui.behaviorFadeUnrelated
    ? Boolean(state.ui.behaviorFadeUnrelated.checked)
    : true;
  state.settings.behavior.autoFitAfterExpand = state.ui.behaviorAutoFit
    ? Boolean(state.ui.behaviorAutoFit.checked)
    : false;
}

function ingestConfig(payload) {
  const symbols = payload?.symbols || [];
  state.symbols = symbols.map((s) => ({ id: s.id, label: s.label || s.id }));
  state.configBySymbol = new Map(symbols.map((s) => [s.id, s.config]));
}

function ingestChains(payload) {
  const symbols = payload?.symbols || [];
  state.roundsBySymbol = new Map();
  state.parentByNodeId = new Map();
  state.defaultExpandedRounds = new Set();
  state.expandedRounds = new Set();

  symbols.forEach((sym) => {
    const symbolId = String(sym.id || "");
    if (!symbolId) return;

    const rounds = sym.rounds || [];
    state.roundsBySymbol.set(symbolId, rounds);
    if (!state.symbols.find((s) => s.id === symbolId)) {
      state.symbols.push({ id: symbolId, label: sym.label || symbolId });
    }

    const originId = `origin-${symbolId}`;
    state.parentByNodeId.set(originId, null);

    rounds.forEach((round) => {
      const roundId = `round-${round.id}`;
      state.parentByNodeId.set(roundId, originId);
      (round.nodes || []).forEach((n) => {
        if (!n?.id) return;
        state.parentByNodeId.set(String(n.id), roundId);
      });
    });

    const latest = findLatestRound(rounds);
    if (latest?.id) {
      state.defaultExpandedRounds.add(`round-${latest.id}`);
    }
  });

  state.expandedRounds = new Set(state.defaultExpandedRounds);
}

function initGraphs() {
  const container = state.ui.graphContainer;
  if (!container) throw new Error("graph container not found");

  const preferredMode = state.ui.modeToggle?.checked ? GRAPH_MODE_3D : GRAPH_MODE_2D;
  state.graphMode = preferredMode;
  state.graphsByMode = new Map();
  state.graphRootsByMode = new Map();

  const graph3DMount = ensureGraphMount(GRAPH_MODE_3D);
  const graph3D = createGraphForMode(GRAPH_MODE_3D, graph3DMount);
  state.graphsByMode.set(GRAPH_MODE_3D, graph3D);
  state.graphRootsByMode.set(GRAPH_MODE_3D, graph3DMount);

  const has2DEngine = typeof window.ForceGraph === "function";
  if (has2DEngine) {
    const graph2DMount = ensureGraphMount(GRAPH_MODE_2D);
    const graph2D = createGraphForMode(GRAPH_MODE_2D, graph2DMount);
    state.graphsByMode.set(GRAPH_MODE_2D, graph2D);
    state.graphRootsByMode.set(GRAPH_MODE_2D, graph2DMount);
    setStatus(`已启用 2D / 3D`);
  } else {
    state.graphMode = GRAPH_MODE_3D;
    if (state.ui.modeToggle) {
      state.ui.modeToggle.checked = false;
      state.ui.modeToggle.disabled = true;
      state.ui.modeToggle.title = "2D 图引擎未加载";
    }
    setStatus("2D 图引擎未加载，已回退 3D", true);
  }

  setGraphMode(state.graphMode, { restoreViewport: false, preferActiveFocus: false });
  setPanelCollapsed("left", false);
  setPanelCollapsed("right", false);
  resizeAllGraphs();
}

function ensureGraphMount(mode) {
  const container = state.ui.graphContainer;
  if (!container) throw new Error("graph container not found");

  let mount = container.querySelector(`[data-graph-mode="${mode}"]`);
  if (!mount) {
    mount = document.createElement("div");
    mount.dataset.graphMode = mode;
    mount.className = `graph-mode-root graph-mode-root-${mode}`;
    mount.style.position = "absolute";
    mount.style.inset = "0";
    mount.style.width = "100%";
    mount.style.height = "100%";
    mount.style.display = "none";
    mount.style.pointerEvents = "none";
    container.appendChild(mount);
  }

  return mount;
}

function createGraphForMode(mode, mountRoot) {
  if (!mountRoot) throw new Error(`graph mount root missing for mode: ${mode}`);

  const is3D = mode === GRAPH_MODE_3D;
  const graph = is3D
    ? ForceGraph3D({
        controlType: "orbit",
        rendererConfig: { antialias: true },
      })(mountRoot)
    : window.ForceGraph()(mountRoot);

  graph.graphData({ nodes: [], links: [] });
  configureSharedGraph(graph, { is3D });
  if (is3D) {
    configure3DGraph(graph);
  } else {
    configure2DGraph(graph);
  }
  applyDefaultForces(graph);
  return graph;
}

function configureSharedGraph(graph, { is3D }) {
  if (typeof graph.nodeLabel === "function") {
    graph.nodeLabel((n) => (state.settings.visual.showLabels ? n.name || n.id || "" : ""));
  }
  if (typeof graph.linkColor === "function") {
    graph.linkColor((l) => styledLinkColor(l));
  }
  if (typeof graph.linkWidth === "function") {
    graph.linkWidth((l) => styledLinkWidth(l));
  }
  if (typeof graph.linkDirectionalParticles === "function") {
    graph.linkDirectionalParticles(0);
  }
  if (typeof graph.forceEngine === "function") {
    graph.forceEngine("d3");
  }
  if (typeof graph.onNodeClick === "function") {
    graph.onNodeClick((node) => handleNodeClick(node));
  }
  if (typeof graph.onNodeHover === "function") {
    graph.onNodeHover((node) => queueHoverUpdate(node ? String(node.id) : null));
  }
  if (typeof graph.onNodeDrag === "function") {
    graph.onNodeDrag((node, translate) => handleNodeDrag(node, translate));
  }
  if (typeof graph.onNodeDragEnd === "function") {
    graph.onNodeDragEnd((node) => handleNodeDragEnd(node));
  }
  if (typeof graph.onBackgroundClick === "function") {
    graph.onBackgroundClick(() => queueHoverUpdate(null));
  }

  if (typeof graph.linkOpacity === "function") {
    graph.linkOpacity((l) => styledLinkOpacity(l));
  }
  if (is3D && typeof graph.backgroundColor === "function") {
    graph.backgroundColor(getComputedStyle(document.body).getPropertyValue("--bg-color"));
  }
}

function configure3DGraph(graph) {
  graph.nodeThreeObject((n) => createNodeObject(n));
  if (typeof graph.showNavInfo === "function") {
    graph.showNavInfo(false);
  }
}

function configure2DGraph(graph) {
  graph
    .nodeColor((n) => styledNodeColor(n))
    .nodeVal((n) => styledNodeVal(n))
    .nodeCanvasObjectMode(() => "after")
    .nodeCanvasObject((node, ctx, globalScale) => draw2DNodeLabel(node, ctx, globalScale))
    .nodeRelSize(1.4);
}

function applyDefaultForces(graph) {
  applyPhysicsSettingsToGraph(graph);
}

function applyPhysicsSettings() {
  state.graphsByMode.forEach((graph) => {
    applyPhysicsSettingsToGraph(graph);
    if (typeof graph.refresh === "function") graph.refresh();
  });
}

function applyPhysicsSettingsToGraph(graph) {
  if (!graph) return;
  const linkDistance = Number(state.settings.physics.linkDistance || 24);
  const linkStrengthScale = Number(state.settings.physics.linkStrengthScale || 1);
  const charge = Number(state.settings.physics.charge || -120);

  const linkForce = graph.d3Force("link");
  if (linkForce?.distance && linkForce?.strength) {
    linkForce.distance(linkDistance).strength(() => linkStrengthScale);
  }

  const chargeForce = graph.d3Force("charge");
  if (chargeForce?.strength) {
    chargeForce.strength(charge);
  }
  if (typeof graph.d3ReheatSimulation === "function") {
    graph.d3ReheatSimulation();
  }
}

function setGraphMode(mode, { restoreViewport = true, preferActiveFocus = false } = {}) {
  const nextMode = mode === GRAPH_MODE_2D ? GRAPH_MODE_2D : GRAPH_MODE_3D;
  const nextGraph = state.graphsByMode.get(nextMode);
  if (!nextGraph) return false;

  if (state.graphMode && state.graphsByMode.has(state.graphMode)) {
    storeViewportForMode(state.graphMode);
  }

  state.graphMode = nextMode;
  state.graph = nextGraph;
  if (state.ui.modeToggle) {
    state.ui.modeToggle.checked = nextMode === GRAPH_MODE_3D;
  }
  syncModeToggleLabel();

  state.graphRootsByMode.forEach((root, modeKey) => {
    if (!root) return;
    const visible = modeKey === nextMode;
    root.style.display = visible ? "" : "none";
    root.style.pointerEvents = visible ? "auto" : "none";
  });

  resizeAllGraphs();

  const nextData = nextGraph.graphData?.();
  if ((!nextData?.nodes || !nextData.nodes.length) && state.currentGraphData.nodes.length) {
    if (nextMode === GRAPH_MODE_3D) {
      state.nodeObjects.clear();
    }
    nextGraph.graphData(cloneGraphData(state.currentGraphData));
  }
  if (typeof nextGraph.d3ReheatSimulation === "function") {
    nextGraph.d3ReheatSimulation();
  }

  let viewportRestored = false;
  if (restoreViewport) {
    viewportRestored = restoreViewportForMode(nextMode);
  }
  if (!viewportRestored && preferActiveFocus && state.activeNodeId) {
    alignCameraToActive(0);
  }

  refreshEmphasis();
  if (!viewportRestored && !state.activeNodeId && state.currentGraphData.nodes.length) {
    fitGraph(320);
  }
  return true;
}

function storeViewportForMode(mode) {
  const graph = state.graphsByMode.get(mode);
  if (!graph) return;

  if (mode === GRAPH_MODE_3D) {
    const camera = graph.camera?.();
    const position = camera?.position;
    if (!position) return;
    const target = graph.controls?.()?.target || { x: 0, y: 0, z: 0 };
    state.viewportByMode.set(mode, {
      x: Number(position.x || 0),
      y: Number(position.y || 0),
      z: Number(position.z || 0),
      tx: Number(target.x || 0),
      ty: Number(target.y || 0),
      tz: Number(target.z || 0),
    });
    return;
  }

  const center = graph.centerAt?.();
  const zoom = graph.zoom?.();
  if (!center || !Number.isFinite(zoom)) return;
  state.viewportByMode.set(mode, {
    x: Number(center.x || 0),
    y: Number(center.y || 0),
    zoom: Number(zoom),
  });
}

function restoreViewportForMode(mode) {
  const graph = state.graphsByMode.get(mode);
  const viewport = state.viewportByMode.get(mode);
  if (!graph || !viewport) return false;

  if (mode === GRAPH_MODE_3D) {
    if (typeof graph.cameraPosition !== "function") return false;
    graph.cameraPosition(
      {
        x: Number(viewport.x || 0),
        y: Number(viewport.y || 0),
        z: Number(viewport.z || 0),
      },
      {
        x: Number(viewport.tx || 0),
        y: Number(viewport.ty || 0),
        z: Number(viewport.tz || 0),
      },
      0
    );
    return true;
  }

  if (typeof graph.centerAt !== "function" || typeof graph.zoom !== "function") {
    return false;
  }
  graph.centerAt(Number(viewport.x || 0), Number(viewport.y || 0), 0);
  graph.zoom(Number(viewport.zoom || 1), 0);
  return true;
}

function resizeAllGraphs() {
  const container = state.ui.graphContainer;
  if (!container) return;
  const width = Math.max(1, container.clientWidth || window.innerWidth || 1);
  const height = Math.max(1, container.clientHeight || window.innerHeight || 1);
  state.graphsByMode.forEach((graph) => {
    if (typeof graph.width === "function") graph.width(width);
    if (typeof graph.height === "function") graph.height(height);
  });
}

function setGraphDataForAllModes(gData) {
  state.currentGraphData = cloneGraphData(gData);
  const graph3D = state.graphsByMode.get(GRAPH_MODE_3D);
  if (graph3D) {
    state.nodeObjects.clear();
    graph3D.graphData(cloneGraphData(gData));
  }
  const graph2D = state.graphsByMode.get(GRAPH_MODE_2D);
  if (graph2D) {
    graph2D.graphData(cloneGraphData(gData));
  }
}

function cloneGraphData(gData) {
  return {
    nodes: (gData?.nodes || []).map((n) => ({ ...n })),
    links: (gData?.links || []).map((l) => ({ ...l })),
  };
}

function rebuildVisibleGraph({ fit = false } = {}) {
  if (!state.graph) return;

  const previousPositions = captureCurrentNodePositions();

  const gData = buildGraphData(previousPositions);
  state.visibleNodesById = new Map(gData.nodes.map((n) => [String(n.id), n]));
  rebuildAdjacency(gData);

  setGraphDataForAllModes(gData);
  syncActiveNodeAfterDataChange();
  renderPreview();
  refreshEmphasis();

  if (!state.hasFit || fit) {
    fitGraph(500);
    state.hasFit = true;
  }
  if (state.followEnabled) {
    alignCameraToActive(0);
  }
}

function scheduleGraphRebuild({ fit = false } = {}) {
  if (fit) state.pendingRebuildFit = true;
  if (state.rebuildFrameRequested) return;
  state.rebuildFrameRequested = true;
  requestAnimationFrame(() => {
    state.rebuildFrameRequested = false;
    const withFit = state.pendingRebuildFit;
    state.pendingRebuildFit = false;
    rebuildVisibleGraph({ fit: withFit });
  });
}

function buildGraphData(previousPositions) {
  const nodes = [];
  const links = [];
  const nodeIds = new Set();
  const symCount = Math.max(1, state.symbols.length);

  state.symbols.forEach((sym, idx) => {
    const symbolId = String(sym.id || "");
    if (!symbolId) return;

    const angle = (idx / symCount) * Math.PI * 2;
    const originId = `origin-${symbolId}`;
    addNode(
      nodes,
      nodeIds,
      {
        id: originId,
        name: sym.label,
        type: "origin",
        val: 14,
        fx: Math.cos(angle) * 80,
        fy: Math.sin(angle) * 80,
        fz: 0,
        symbol: symbolId,
      },
      previousPositions
    );

    const rounds = state.roundsBySymbol.get(symbolId) || [];
    rounds.forEach((round) => {
      const roundId = `round-${round.id}`;
      addNode(
        nodes,
        nodeIds,
        {
          id: roundId,
          name: roundShortLabel(round),
          type: "round",
          direction: roundDirection(round),
          val: 9,
          symbol: symbolId,
          roundId: round.id,
          data: round,
        },
        previousPositions
      );
      links.push({
        source: originId,
        target: roundId,
        color: "rgba(255,255,255,0.2)",
      });

      if (!state.expandedRounds.has(roundId)) return;
      const stages = round.nodes || [];
      const ordered = [...stages].sort(
        (a, b) => stageOrder(normalizeStage(a.stage)) - stageOrder(normalizeStage(b.stage))
      );

      ordered.forEach((st) => {
        if (st?.id === undefined || st?.id === null || st?.id === "") return;
        nodeIds.add(String(st.id));
      });
      ordered.forEach((st) => {
        if (st?.id === undefined || st?.id === null || st?.id === "") return;
        const stageId = String(st.id);
        const role = (st.type || st.role || "").toLowerCase();
        addNode(
          nodes,
          nodeIds,
          {
            id: stageId,
            name: shortenLabel(stageDisplayName(st), 10),
            type: normalizeStage(st.stage),
            role,
            stage: st.stage,
            symbol: symbolId,
            roundId: round.id,
            data: st,
            val: 6,
          },
          previousPositions
        );
        if (isAgentRole(role)) {
          links.push({
            source: roundId,
            target: stageId,
            color: stageColor(st.stage),
          });
        }
        if (!Array.isArray(st.refs)) return;
        st.refs.forEach((ref) => {
          const refId = String(ref);
          if (!nodeIds.has(refId)) {
            if (!state.settings.filters.showMissingNodes) return;
            addNode(
              nodes,
              nodeIds,
              {
                id: refId,
                name: shortenLabel(refId, 12),
                type: "missing",
                symbol: symbolId,
                roundId: round.id,
                val: 4,
              },
              previousPositions
            );
            state.parentByNodeId.set(refId, roundId);
          }
          links.push({
            source: stageId,
            target: refId,
            color: "rgba(255,255,255,0.12)",
          });
        });
      });
    });
  });

  const normalized = {
    nodes,
    links: links
      .filter((l) => nodeIds.has(String(l.source)) && nodeIds.has(String(l.target)))
      .map((l) => ({
        ...l,
        source: String(l.source),
        target: String(l.target),
      })),
  };

  return filterGraphData(normalized);
}

function filterGraphData(gData) {
  let nodes = [...(gData.nodes || [])];
  let links = [...(gData.links || [])];

  if (!state.settings.filters.showDailies) {
    const keepIds = new Set(nodes.map((n) => String(n.id)));
    nodes = nodes.filter((n) => keepIds.has(String(n.id)));
    links = links.filter((l) => keepIds.has(String(l.source)) && keepIds.has(String(l.target)));
  }

  const query = String(state.settings.filters.tagQuery || "").trim().toLowerCase();
  if (query) {
    const queryParts = query.split(/\s+/).filter(Boolean);
    const matches = new Set(
      nodes
        .filter((n) => {
          const haystack = nodeTagText(n);
          return queryParts.every((q) => haystack.includes(q));
        })
        .map((n) => String(n.id))
    );

    if (matches.size) {
      links.forEach((l) => {
        const source = String(l.source);
        const target = String(l.target);
        if (matches.has(source) || matches.has(target)) {
          matches.add(source);
          matches.add(target);
        }
      });
      nodes = nodes.filter((n) => matches.has(String(n.id)));
      links = links.filter((l) => matches.has(String(l.source)) && matches.has(String(l.target)));
    }
  }

  if (state.settings.filters.hideOrphans) {
    const degree = new Map();
    nodes.forEach((n) => {
      degree.set(String(n.id), 0);
    });
    links.forEach((l) => {
      const source = String(l.source);
      const target = String(l.target);
      degree.set(source, Number(degree.get(source) || 0) + 1);
      degree.set(target, Number(degree.get(target) || 0) + 1);
    });
    const keepIds = new Set(
      [...degree.entries()].filter(([, count]) => count > 0).map(([id]) => String(id))
    );
    nodes = nodes.filter((n) => keepIds.has(String(n.id)));
    links = links.filter((l) => keepIds.has(String(l.source)) && keepIds.has(String(l.target)));
  }

  return { nodes, links };
}

function nodeTagText(node) {
  const tags = [
    String(node.type || ""),
    String(node.stage || ""),
    String(node.role || ""),
    String(node.symbol || ""),
    String(node.direction || ""),
    String(node.name || ""),
  ];
  return tags.join(" ").toLowerCase();
}

function addNode(nodes, nodeIds, node, previousPositions) {
  const id = String(node.id);
  const next = { ...node, id };
  const prev = previousPositions.get(id);
  if (prev) {
    if (Number.isFinite(prev.x)) next.x = prev.x;
    if (Number.isFinite(prev.y)) next.y = prev.y;
    if (Number.isFinite(prev.z)) next.z = prev.z;
    if (Number.isFinite(prev.fx)) next.fx = prev.fx;
    if (Number.isFinite(prev.fy)) next.fy = prev.fy;
    if (Number.isFinite(prev.fz)) next.fz = prev.fz;
  } else {
    if (next.x === undefined && Number.isFinite(next.fx)) next.x = next.fx;
    if (next.y === undefined && Number.isFinite(next.fy)) next.y = next.fy;
    if (next.z === undefined && Number.isFinite(next.fz)) next.z = next.fz;
  }
  nodes.push(next);
  nodeIds.add(id);
}

function rebuildAdjacency(gData) {
  state.adjacentByNodeId = new Map();
  state.linkKeysByNodeId = new Map();

  (gData.nodes || []).forEach((n) => {
    const id = String(n.id);
    state.adjacentByNodeId.set(id, new Set());
    state.linkKeysByNodeId.set(id, new Set());
  });

  (gData.links || []).forEach((l) => {
    const source = nodeRefId(l.source);
    const target = nodeRefId(l.target);
    if (!source || !target) return;

    if (!state.adjacentByNodeId.has(source)) state.adjacentByNodeId.set(source, new Set());
    if (!state.adjacentByNodeId.has(target)) state.adjacentByNodeId.set(target, new Set());
    if (!state.linkKeysByNodeId.has(source)) state.linkKeysByNodeId.set(source, new Set());
    if (!state.linkKeysByNodeId.has(target)) state.linkKeysByNodeId.set(target, new Set());

    const key = linkKey(source, target);
    state.adjacentByNodeId.get(source).add(target);
    state.adjacentByNodeId.get(target).add(source);
    state.linkKeysByNodeId.get(source).add(key);
    state.linkKeysByNodeId.get(target).add(key);
  });
}

function createNodeObject(node) {
  const group = new THREE.Group();
  const radius = Math.max(4, (node.val || 6) * 0.9);
  const geo = new THREE.SphereGeometry(radius, 20, 20);
  const isOrigin = node.type === "origin";
  const mat = new THREE.MeshStandardMaterial({
    color: nodeColor(node),
    roughness: isOrigin ? 0.45 : 0.35,
    metalness: isOrigin ? 0.05 : 0.1,
    transparent: true,
    opacity: isOrigin ? 0.72 : 0.9,
  });
  const mesh = new THREE.Mesh(geo, mat);
  group.add(mesh);

  let label = null;
  if (window.SpriteText) {
    const text = shortenLabel(nodeShortLabel(node), radius >= 7 ? 12 : 10);
    if (text) {
      label = new SpriteText(text);
      label.color = node.type === "origin" ? "#ffffff" : "#f5f7ff";
      label.strokeWidth = isOrigin ? 0 : 0.8;
      label.strokeColor = node.type === "origin" ? "rgba(100,180,240,0.5)" : "rgba(5,5,16,0.85)";
      label.fontWeight = isOrigin ? "bold" : "normal";
      label.textHeight = Math.min(radius * 0.6, 4);
      label.material.depthWrite = false;
      label.material.depthTest = false;
      label.position.set(0, 0, 0);
      group.add(label);
    }
  }

  state.nodeObjects.set(String(node.id), { mesh, label });
  return group;
}

function draw2DNodeLabel(node, ctx, globalScale) {
  if (!state.settings.visual.showLabels) return;
  if (!ctx || !node) return;
  const text = nodeShortLabel(node);
  if (!text) return;

  const minScale = Math.max(0.7, Number(globalScale || 1));
  const fontSize = Math.max(6, Math.min(13, 11 / minScale));
  const isOrigin2D = node.type === "origin";
  ctx.font = `${isOrigin2D ? 900 : 600} ${fontSize}px "IBM Plex Sans", "Noto Sans SC", sans-serif`;
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillStyle = node.type === "origin" ? "rgba(255,255,255,0.95)" : "rgba(245,247,255,0.95)";
  ctx.strokeStyle = "rgba(6,10,22,0.72)";
  ctx.lineWidth = Math.max(1, fontSize * 0.2);
  ctx.strokeText(text, Number(node.x || 0), Number(node.y || 0));
  ctx.fillText(text, Number(node.x || 0), Number(node.y || 0));
}

function styledLinkColor(link) {
  const base = link.color || "rgba(255,255,255,0.35)";
  if (!state.settings.behavior.fadeUnrelated) return base;
  if (!state.highlightLinkKeys.size) return base;
  const key = linkKey(nodeRefId(link.source), nodeRefId(link.target));
  return state.highlightLinkKeys.has(key) ? base : "rgba(120,130,160,0.08)";
}

function styledNodeColor(node) {
  const base = nodeColor(node);
  const id = String(node.id);
  if (!state.settings.behavior.fadeUnrelated) {
    return id === state.activeNodeId ? "#ffd666" : base;
  }
  const hasEmphasis = state.highlightNodeIds.size > 0;
  const isActive = id === state.activeNodeId;
  const highlighted = !hasEmphasis || state.highlightNodeIds.has(id) || isActive;
  if (!highlighted) return "rgba(97,108,136,0.25)";
  return isActive ? "#ffd666" : base;
}

function styledNodeVal(node) {
  const base = Number(node.val || 6);
  const id = String(node.id);
  if (id === state.activeNodeId) return base * 1.22;
  if (!state.settings.behavior.fadeUnrelated) return base;
  if (!state.highlightNodeIds.size || state.highlightNodeIds.has(id)) return base;
  return Math.max(2.2, base * 0.75);
}

function styledLinkWidth(link) {
  const scale = Number(state.settings.visual.linkWidthScale || 1);
  if (!state.settings.behavior.fadeUnrelated) return 1.2 * scale;
  if (!state.highlightLinkKeys.size) return 1.2 * scale;
  const key = linkKey(nodeRefId(link.source), nodeRefId(link.target));
  return state.highlightLinkKeys.has(key) ? 2.4 * scale : 0.35 * scale;
}

function styledLinkOpacity(link) {
  if (!state.settings.behavior.fadeUnrelated) return 0.58;
  if (!state.highlightLinkKeys.size) return 0.58;
  const key = linkKey(nodeRefId(link.source), nodeRefId(link.target));
  return state.highlightLinkKeys.has(key) ? 0.95 : 0.1;
}

function handleNodeClick(node) {
  if (!node) return;
  if (performance.now() < state.ignoreClickUntil) return;
  const id = String(node.id);
  const now = performance.now();
  if (
    state.lastClick &&
    state.lastClick.nodeId === id &&
    now - state.lastClick.ts <= DOUBLE_CLICK_MS
  ) {
    clearTimeout(state.lastClick.timer);
    state.lastClick = null;
    handleNodeDoubleClick(id);
    return;
  }

  if (state.lastClick?.timer) {
    clearTimeout(state.lastClick.timer);
  }
  const timer = setTimeout(() => {
    state.lastClick = null;
    handleNodeSingleClick(id);
  }, DOUBLE_CLICK_MS);
  state.lastClick = { nodeId: id, ts: now, timer };
}

function handleNodeSingleClick(id) {
  const node = state.visibleNodesById.get(id);
  if (!node) return;
  setActiveNode(id);
}

function handleNodeDoubleClick(id) {
  const node = state.visibleNodesById.get(id);
  if (!node) return;
  setActiveNode(id);
  if (!state.settings.behavior.doubleClickExpand) return;
  if (node.type === "round") {
    toggleRound(id);
    return;
  }
}

function toggleRound(roundId) {
  if (state.expandedRounds.has(roundId)) {
    state.expandedRounds.delete(roundId);
  } else {
    state.expandedRounds.add(roundId);
  }
  rebuildVisibleGraph({ fit: state.settings.behavior.autoFitAfterExpand });
}

function setActiveNode(id) {
  state.activeNodeId = id ? String(id) : null;
  renderPreview();
  refreshEmphasis();
  if (state.followEnabled) {
    alignCameraToActive(260);
  }
}

function syncActiveNodeAfterDataChange() {
  if (!state.activeNodeId) return;
  if (state.visibleNodesById.has(state.activeNodeId)) return;
  state.activeNodeId = findVisibleFallback(state.activeNodeId);
  if (!state.activeNodeId && state.followEnabled) {
    setFollowMode(false);
  }
}

function findVisibleFallback(id) {
  let cursor = id;
  const seen = new Set();
  while (cursor && !seen.has(cursor)) {
    if (state.visibleNodesById.has(cursor)) return cursor;
    seen.add(cursor);
    cursor = state.parentByNodeId.get(cursor) || null;
  }
  return null;
}

function queueHoverUpdate(nextId) {
  if (state.dragging) return;
  state.pendingHoverId = nextId;
  if (state.hoverFrameRequested) return;
  state.hoverFrameRequested = true;
  requestAnimationFrame(() => {
    state.hoverFrameRequested = false;
    if (state.hoverNodeId === state.pendingHoverId) return;
    state.hoverNodeId = state.pendingHoverId;
    refreshEmphasis();
  });
}

function refreshEmphasis() {
  const centerId = state.hoverNodeId || state.activeNodeId || null;
  if (!centerId) {
    state.highlightNodeIds = new Set();
    state.highlightLinkKeys = new Set();
  } else {
    const neighbors = state.adjacentByNodeId.get(centerId) || new Set();
    const links = state.linkKeysByNodeId.get(centerId) || new Set();
    state.highlightNodeIds = new Set([centerId, ...neighbors]);
    state.highlightLinkKeys = new Set(links);
  }

  applyNodeVisualState();
  state.graphsByMode.forEach((graph) => {
    if (typeof graph.refresh === "function") {
      graph.refresh();
    }
  });
}

function applyNodeVisualState() {
  const hasEmphasis = state.settings.behavior.fadeUnrelated && state.highlightNodeIds.size > 0;
  state.nodeObjects.forEach((entry, id) => {
    const node = state.visibleNodesById.get(id);
    if (!node || !entry?.mesh?.material) return;

    const isActive = id === state.activeNodeId;
    const highlighted = !hasEmphasis || state.highlightNodeIds.has(id) || isActive;
    const material = entry.mesh.material;
    material.color.set(nodeColor(node));
    material.opacity = hasEmphasis ? (highlighted ? 0.94 : 0.14) : 0.9;
    material.emissive.setHex(isActive ? 0x1b2f58 : 0x000000);
    material.needsUpdate = true;

    const labelMat = entry.label?.material;
    if (labelMat) {
      if (!state.settings.visual.showLabels) {
        labelMat.opacity = 0;
      } else {
        labelMat.opacity = hasEmphasis ? (highlighted ? 0.95 : 0.2) : 0.9;
      }
      labelMat.transparent = true;
      labelMat.needsUpdate = true;
    }
  });
}

function handleNodeDrag(node, translate) {
  if (!node || !state.graph) return;
  state.dragging = true;
}

function handleNodeDragEnd(node) {
  state.dragging = false;
  state.ignoreClickUntil = performance.now() + 140;
  queueHoverUpdate(null);
}

function handlePreviewClick(evt) {
  const trigger = evt.target.closest("[data-node-id]");
  if (!trigger) return;
  const nodeId = String(trigger.dataset.nodeId || "");
  if (!state.visibleNodesById.has(nodeId)) return;
  setActiveNode(nodeId);
}

function renderPreview() {
  if (!state.ui.previewTitle || !state.ui.previewContent) return;
  if (!state.activeNodeId) {
    state.ui.previewTitle.textContent = "选择节点";
    state.ui.previewContent.innerHTML = '<div class="placeholder">点击图节点查看详情</div>';
    return;
  }
  const node = state.visibleNodesById.get(state.activeNodeId);
  if (!node) {
    state.ui.previewTitle.textContent = "节点不可见";
    state.ui.previewContent.innerHTML = '<div class="placeholder">该节点已折叠或不可见。</div>';
    return;
  }
  state.ui.previewTitle.textContent = panelTitle(node);
  state.ui.previewContent.innerHTML = `${renderNodeTagMeta(node)}${panelBody(node)}${renderRelatedHTML(
    node.id
  )}`;
}

function renderNodeTagMeta(node) {
  if (!node || typeof node !== "object") return "";
  const tags = [];
  if (node.symbol) tags.push(String(node.symbol));
  if (node.type) tags.push(String(node.type));
  if (node.stage && String(node.stage).toLowerCase() !== String(node.type || "").toLowerCase()) {
    tags.push(String(node.stage));
  }
  if (node.direction) tags.push(String(node.direction));
  if (node.roundId) tags.push(`round:${String(node.roundId)}`);
  if (!tags.length) return "";
  return `<div class="note-meta">${tags
    .slice(0, 8)
    .map((tag) => `<span class="tag">${escapeHTML(tag)}</span>`)
    .join("")}</div>`;
}

function renderRelatedHTML(nodeId) {
  const related = [...(state.adjacentByNodeId.get(String(nodeId)) || [])];
  if (!related.length) {
    return `
      <div class="related">
        <div class="related-title">Linked Notes</div>
        <div class="placeholder">无直接关联节点</div>
      </div>
    `;
  }

  related.sort((a, b) => {
    const na = state.visibleNodesById.get(a);
    const nb = state.visibleNodesById.get(b);
    const ta = panelTitle(na || { id: a, name: a, type: "stage" });
    const tb = panelTitle(nb || { id: b, name: b, type: "stage" });
    return ta.localeCompare(tb, "zh-CN");
  });

  return `
    <div class="related">
      <div class="related-title">Linked Notes</div>
      <div class="related-list">
        ${related
          .map((id) => {
            const node = state.visibleNodesById.get(id);
            const label = panelTitle(node || { id, name: id, type: "stage" });
            return `<button class="related-link" type="button" data-node-id="${escapeAttr(
              id
            )}">${escapeHTML(label)}</button>`;
          })
          .join("")}
      </div>
    </div>
  `;
}

function setFollowMode(enabled) {
  const next = Boolean(enabled);
  state.followEnabled = next;
  if (state.ui.followToggle) state.ui.followToggle.checked = next;

  if (!next) return;

  if (!state.activeNodeId) {
    const first = firstVisibleNodeId();
    if (first) {
      setActiveNode(first);
    } else {
      state.followEnabled = false;
      if (state.ui.followToggle) state.ui.followToggle.checked = false;
      return;
    }
  }

  captureFollowOffset();
  alignCameraToActive(260);
}

function alignCameraToActive(duration = 0) {
  if (!state.graph || !state.activeNodeId) return false;
  const node = state.visibleNodesById.get(state.activeNodeId);
  if (!node) return false;

  if (state.graphMode === GRAPH_MODE_2D) {
    if (!state.followOffset || state.followOffset.mode !== GRAPH_MODE_2D) {
      captureFollowOffset();
    }
    const offset =
      state.followOffset && state.followOffset.mode === GRAPH_MODE_2D
        ? state.followOffset
        : { mode: GRAPH_MODE_2D, x: 0, y: 0, zoom: 3.2 };
    if (typeof state.graph.centerAt === "function") {
      state.graph.centerAt(
        Number(node.x || 0) + Number(offset.x || 0),
        Number(node.y || 0) + Number(offset.y || 0),
        duration
      );
    }
    if (typeof state.graph.zoom === "function") {
      state.graph.zoom(Number(offset.zoom || 3.2), duration);
    }
    return true;
  }

  if (!state.followOffset || state.followOffset.mode !== GRAPH_MODE_3D) {
    captureFollowOffset();
  }
  const offset =
    state.followOffset && state.followOffset.mode === GRAPH_MODE_3D
      ? state.followOffset
      : { mode: GRAPH_MODE_3D, x: 0, y: 0, z: 120 };
  state.graph.cameraPosition(
    {
      x: Number(node.x || 0) + Number(offset.x || 0),
      y: Number(node.y || 0) + Number(offset.y || 0),
      z: Number(node.z || 0) + Number(offset.z || 120),
    },
    node,
    duration
  );
  return true;
}

function captureFollowOffset() {
  if (!state.graph || !state.activeNodeId) return;
  const node = state.visibleNodesById.get(state.activeNodeId);
  if (!node) return;

  if (state.graphMode === GRAPH_MODE_2D) {
    const center = state.graph.centerAt?.();
    const zoom = state.graph.zoom?.();
    if (!center || !Number.isFinite(zoom)) {
      state.followOffset = { mode: GRAPH_MODE_2D, x: 0, y: 0, zoom: 3.2 };
      return;
    }
    state.followOffset = {
      mode: GRAPH_MODE_2D,
      x: Number(center.x || 0) - Number(node.x || 0),
      y: Number(center.y || 0) - Number(node.y || 0),
      zoom: Number(zoom),
    };
    return;
  }

  const camera = state.graph.camera?.();
  const pos = camera?.position;
  if (!pos) {
    state.followOffset = { mode: GRAPH_MODE_3D, x: 0, y: 0, z: 120 };
    return;
  }

  const offset = {
    mode: GRAPH_MODE_3D,
    x: Number(pos.x || 0) - Number(node.x || 0),
    y: Number(pos.y || 0) - Number(node.y || 0),
    z: Number(pos.z || 0) - Number(node.z || 0),
  };
  const len = Math.hypot(offset.x, offset.y, offset.z);
  if (!Number.isFinite(len) || len < 20) {
    state.followOffset = { mode: GRAPH_MODE_3D, x: 0, y: 0, z: 120 };
    return;
  }
  state.followOffset = offset;
}

function fitGraph(duration = 500) {
  if (!state.graph) return;
  state.graph.zoomToFit(duration, 50);
}

function resetView() {
  if (state.lastClick?.timer) {
    clearTimeout(state.lastClick.timer);
  }
  state.lastClick = null;

  state.hoverNodeId = null;
  state.pendingHoverId = null;
  state.activeNodeId = null;
  state.followOffset = null;
  state.viewportByMode = new Map();
  state.expandedRounds = new Set(state.defaultExpandedRounds);
  setFollowMode(false);
  rebuildVisibleGraph({ fit: true });
  renderPreview();
}

function captureCurrentNodePositions() {
  const map = new Map();
  if (!state.graph) return map;
  const current = state.graph.graphData();
  const nodes = Array.isArray(current?.nodes) ? current.nodes : [];
  nodes.forEach((n) => {
    map.set(String(n.id), {
      x: Number(n.x),
      y: Number(n.y),
      z: Number(n.z),
      fx: Number(n.fx),
      fy: Number(n.fy),
      fz: Number(n.fz),
    });
  });
  return map;
}

function firstVisibleNodeId() {
  const iter = state.visibleNodesById.keys();
  const first = iter.next();
  return first.done ? null : String(first.value);
}

function nodeRefId(ref) {
  if (ref === null || ref === undefined) return "";
  if (typeof ref === "object") return String(ref.id || "");
  return String(ref);
}

function linkKey(source, target) {
  return `${String(source)}->${String(target)}`;
}

function roundDirection(round) {
  let dir = "hold";
  (round.nodes || []).forEach((n) => {
    const stage = normalizeStage(n.stage);
    if ((stage === "execution" || stage === "gate") && n.output?.direction) {
      dir = mapDirection(n.output.direction);
    }
  });
  return dir;
}

function mapDirection(direction) {
  const raw = String(direction || "").toLowerCase();
  if (raw.includes("long") || raw.includes("buy")) return "long";
  if (raw.includes("short") || raw.includes("sell")) return "short";
  return "hold";
}

function nodeColor(node) {
  if (node.type === "round") {
    return DIRECTION_COLORS[node.direction] || STAGE_COLORS.round;
  }
  return STAGE_COLORS[node.type] || STAGE_COLORS.execution;
}

function stageColor(stage) {
  return STAGE_COLORS[normalizeStage(stage)] || "rgba(255,255,255,0.2)";
}

function panelTitle(node) {
  if (!node) return "详情";
  if (node.type === "origin") return node.name || node.symbol || "Origin";
  if (node.type === "round") return node.name || node.data?.label || "Round";
  return stageDisplayName({
    stage: node.stage || node.type,
    title: node.title || node.name,
    agentKey: node.agentKey,
  });
}

function panelBody(node) {
  if (!node) return '<div class="placeholder">暂无数据</div>';
  if (node.type === "origin") return renderConfigHTML(node.symbol);
  if (node.type === "round") return renderRoundHTML(node.data);
  return renderStageHTML(node.data);
}

function renderConfigHTML(symbol) {
  const cfg = state.configBySymbol.get(symbol);
  if (!cfg) return '<div class="placeholder">暂无配置</div>';
  return `
    <div class="note-title"># ${escapeHTML(symbol)}</div>
    <div class="note-meta">
      <span class="tag">Config</span>
      <span class="tag">kline_limit ${escapeHTML(String(cfg.kline_limit ?? "n/a"))}</span>
      <span class="tag">${escapeHTML((cfg.intervals || []).join(" / ") || "—")}</span>
    </div>
    <div class="block"><strong>Agent 启用</strong> indicator=${cfg.agent_enabled?.indicator ? "on" : "off"} / structure=${cfg.agent_enabled?.structure ? "on" : "off"} / mechanics=${cfg.agent_enabled?.mechanics ? "on" : "off"}</div>
    <div class="block"><strong>LLM 路由</strong> agent(${escapeHTML(cfg.llm?.agent?.indicator || "n/a")} | ${escapeHTML(cfg.llm?.agent?.structure || "n/a")} | ${escapeHTML(cfg.llm?.agent?.mechanics || "n/a")}) · provider(${escapeHTML(cfg.llm?.provider?.indicator || "n/a")} | ${escapeHTML(cfg.llm?.provider?.structure || "n/a")} | ${escapeHTML(cfg.llm?.provider?.mechanics || "n/a")})</div>
    <div class="block"><strong>策略</strong> 风险=${escapeHTML(String(cfg.strategy?.risk_per_trade_pct ?? "n/a"))}%</div>
    <div class="block"><strong>Prompt 摘要</strong>
      <div class="code-block">${escapeHTML(formatPromptSnippet(cfg.prompts))}</div>
    </div>
  `;
}

function renderRoundHTML(round) {
  if (!round) return '<div class="placeholder">暂无轮次信息</div>';
  return `
    <div class="note-title">${escapeHTML(round.label || "Round")}</div>
    <div class="note-meta">
      <span class="tag">Round</span>
      <span class="tag">${escapeHTML(
        round.timestamp ? new Date(round.timestamp * 1000).toLocaleString() : "时间未知"
      )}</span>
    </div>
    <div class="block"><strong>阶段</strong>：${escapeHTML(
      (round.nodes || []).map((n) => n.title || n.stage).join(" → ")
    )}</div>
    <div class="block"><strong>提示</strong> 点击本节点可展开/收起阶段节点</div>
  `;
}

function renderStageHTML(stage) {
  if (!stage || typeof stage !== "object") {
    return '<div class="placeholder">暂无阶段详情</div>';
  }
  const ts = stage.meta?.timestamp ? new Date(stage.meta.timestamp * 1000).toLocaleString() : "—";
  const stageType = normalizeStage(stage.stage);
  if (stageType === "gate") return renderGateHTML(stage, ts);

  const [sysPrompt, userPrompt] = extractPrompts(stage.input);
  return `
    <div class="note-title">${escapeHTML(stage.title || stage.stage || "Stage")}</div>
    <div class="note-meta">
      <span class="tag">${escapeHTML(stage.stage || "—")}</span>
      <span class="tag">时间 ${escapeHTML(ts)}</span>
    </div>
    <div class="block"><strong>LLM 输入</strong>
      <div class="code-block"><strong>System</strong>\n${escapeHTML(sysPrompt)}</div>
      <div class="code-block"><strong>User</strong>\n${escapeHTML(userPrompt)}</div>
    </div>
    <div class="block"><strong>LLM 输出</strong><div class="code-block">${escapeHTML(
      formatData(stage.output)
    )}</div></div>
    <div class="block"><strong>指纹</strong> ${escapeHTML(
      stage.meta?.fingerprint || "—"
    )} | 源 ${escapeHTML(stage.meta?.source || stage.meta?.strategy_config || "—")}</div>
  `;
}

function renderGateHTML(stage, ts) {
  const gate = stage.output || {};
  const overall = gate.overall || {};
  const providers = gate.providers || [];
  const derived = gate.derived || {};
  const report = gate.report || formatData(gate);
  const direction = overall.direction || "—";
  return `
    <div class="note-title">${escapeHTML(stage.title || stage.stage || "Gate")}</div>
    <div class="note-meta">
      <span class="tag">${escapeHTML(stage.stage || "gate")}</span>
      <span class="tag">时间 ${escapeHTML(ts)}</span>
    </div>
    <div class="block"><strong>Gate 判定</strong>
      <div class="code-block">${escapeHTML(report)}</div>
    </div>
    <div class="block"><strong>结论</strong> ${escapeHTML(
      overall.tradeable_text || formatBoolAction(overall.tradeable)
    )}</div>
    <div class="block"><strong>理由</strong> ${escapeHTML(overall.reason || "—")}</div>
    <div class="block"><strong>方向</strong> ${escapeHTML(direction)}</div>
    ${renderGateProcess(derived)}
    ${renderGateSieve(derived)}
    ${renderGateProviders(providers)}
    <div class="block"><strong>指纹</strong> ${escapeHTML(
      stage.meta?.fingerprint || "—"
    )} | 源 ${escapeHTML(stage.meta?.source || stage.meta?.strategy_config || "—")}</div>
  `;
}

function renderGateProcess(derived) {
  if (!derived || typeof derived !== "object") return "";
  const trace = derived.gate_trace;
  if (!Array.isArray(trace) || !trace.length) return "";
  const lines = [];
  trace.forEach((entry) => {
    if (!entry || typeof entry !== "object") return;
    const stepKey = String(entry.step || "").trim();
    if (!stepKey) return;
    const ok = entry.ok === true;
    const statusText = ok ? "通过" : "停止";
    const reasonKey = String(entry.reason || "").trim();
    lines.push(`${stepKey}: ${statusText}${reasonKey ? ` (${reasonKey})` : ""}`);
  });
  if (!lines.length) return "";
  return `
    <div class="block"><strong>Gate 过程</strong>
      <div class="code-block">${lines.map(escapeHTML).join("\n")}</div>
    </div>
  `;
}

function renderGateSieve(derived) {
  if (!derived || typeof derived !== "object") return "";
  const hasAny =
    derived.sieve_action !== undefined ||
    derived.sieve_size_factor !== undefined ||
    derived.sieve_reason !== undefined ||
    derived.sieve_hit !== undefined;
  if (!hasAny) return "";
  const lines = [
    `action: ${escapeHTML(formatData(derived.sieve_action || "—"))}`,
    `size_factor: ${escapeHTML(formatData(derived.sieve_size_factor ?? "—"))}`,
    `reason: ${escapeHTML(formatData(derived.sieve_reason || "—"))}`,
    `hit: ${escapeHTML(formatData(derived.sieve_hit))}`,
  ];
  if (derived.gate_action_before_sieve !== undefined) {
    lines.push(`before: ${escapeHTML(formatData(derived.gate_action_before_sieve))}`);
  }
  if (derived.crowding_align !== undefined) {
    lines.push(`crowding_align: ${escapeHTML(formatData(derived.crowding_align))}`);
  }
  if (derived.sieve_policy_hash) {
    lines.push(`policy_hash: ${escapeHTML(formatData(derived.sieve_policy_hash))}`);
  }
  return `
    <div class="block"><strong>Sieve 筛选</strong>
      <div class="code-block">${lines.join("\n")}</div>
    </div>
  `;
}

function renderGateProviders(providers) {
  if (!Array.isArray(providers) || !providers.length) return "";
  return providers
    .map((p) => {
      const factors = renderGateFactors(p.factors);
      return `<div class="block"><strong>${escapeHTML(gateRoleLabel(p.role))}</strong> ${escapeHTML(
        p.tradeable_text || formatBoolAction(p.tradeable)
      )}${factors}</div>`;
    })
    .join("");
}

function renderGateFactors(factors) {
  if (!Array.isArray(factors) || !factors.length) return "";
  const lines = factors.map((f) => {
    return `${escapeHTML(f.label || f.key || "")}: ${escapeHTML(f.status || formatData(f.raw))}`;
  });
  return `<div class="code-block">${lines.join("\n")}</div>`;
}

function extractPrompts(input) {
  if (!input) return ["—", "—"];
  if (typeof input === "string") return ["—", input];
  if (typeof input === "object") {
    const sys = input.system_prompt || input.system || input.systemPrompt || "—";
    const user = input.user_prompt || input.user || input.prompt || input.userPrompt || "—";
    return [formatData(sys), formatData(user)];
  }
  return ["—", "—"];
}

function gateRoleLabel(role) {
  const r = String(role || "").toLowerCase();
  if (r.includes("indicator")) return "指标";
  if (r.includes("structure")) return "结构";
  if (r.includes("mechanics")) return "力学/风险";
  return role || "Provider";
}

function formatBoolAction(v) {
  if (v === true) return "可交易 (YES)";
  if (v === false) return "不可交易 (NO)";
  return "—";
}

function setStatus(text, danger = false) {
  const el = state.ui.status;
  if (!el) return;
  el.textContent = text;
  el.style.color = danger ? "#ff708f" : "var(--text-dim)";
}

async function fetchJSON(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`fetch ${url} failed ${res.status}`);
  return res.json();
}

function normalizeBase(input) {
  if (!input) return "";
  let base = String(input).trim();
  if (base.endsWith("/")) base = base.slice(0, -1);
  if (!base.startsWith("/")) base = "/" + base;
  return base;
}

function normalizeStage(stage = "") {
  const s = String(stage || "").toLowerCase();
  if (s.includes("provider")) return "provider";
  if (s.includes("indicator")) return "indicator";
  if (s.includes("structure")) return "structure";
  if (s.includes("mechanics")) return "mechanics";
  if (s.includes("gate")) return "gate";
  if (s.includes("exec")) return "execution";
  if (s.includes("round")) return "round";
  if (s.includes("origin")) return "origin";
  return s || "stage";
}

function formatData(value) {
  if (value === undefined || value === null) return "—";
  if (typeof value === "string") {
    const decoded = decodeEscapedText(value);
    const pretty = tryParseJSON(decoded);
    return pretty ?? decoded;
  }
  try {
    return decodeEscapedText(JSON.stringify(value, null, 2));
  } catch (err) {
    return String(value);
  }
}

function formatPromptSnippet(prompts) {
  if (!prompts) return "无 Prompt 配置";
  const parts = [];
  if (prompts.agent) {
    parts.push(
      `Agent: IND(${trimText(prompts.agent.indicator)}) | STR(${trimText(
        prompts.agent.structure
      )}) | MEC(${trimText(prompts.agent.mechanics)})`
    );
  }
  if (prompts.provider) {
    parts.push(
      `Provider: IND(${trimText(prompts.provider.indicator)}) | STR(${trimText(
        prompts.provider.structure
      )}) | MEC(${trimText(prompts.provider.mechanics)})`
    );
  }
  return parts.join("\n");
}

function trimText(text, len = 40) {
  if (!text) return "—";
  const normalized = String(text);
  return normalized.length > len ? `${normalized.slice(0, len)}…` : normalized;
}

function shortenLabel(text, len = 8) {
  if (!text) return "";
  const clean = String(text).replace(/\s+/g, " ").trim();
  return clean.length > len ? `${clean.slice(0, len - 1)}…` : clean;
}

function stageDisplayName(stage) {
  const normalized = normalizeStage(stage.stage || stage.type || "");
  const title = String(stage.title || "").trim();
  const key = String(stage.agentKey || stage.stage || "").toLowerCase();
  const base = title || stage.stage || normalized;
  if (normalized === "round") return base;
  if (normalized === "provider") {
    const suffix = key || base;
    return `P-${shortenLabel(suffix, 8)}`;
  }
  if (isAgentRole(normalized) || normalized === "agent") {
    const suffix = key || base;
    return `A-${shortenLabel(suffix, 8)}`;
  }
  return base;
}

function roundShortLabel(round) {
  const label = String(round?.label || "");
  const byLabel = label.match(/(\d+)/);
  if (byLabel?.[1]) return `R${byLabel[1]}`;
  const byId = String(round?.id || "").match(/(\d+)/);
  if (byId?.[1]) return `R${byId[1]}`;
  return "R";
}

function nodeShortLabel(node) {
  if (!node || typeof node !== "object") return "";
  const type = String(node.type || "").toLowerCase();
  if (type === "origin") {
    return compactSymbol(String(node.symbol || node.name || "")) || "NODE";
  }
  if (type === "round") {
    return roundShortLabel(node.data || node);
  }
  if (type === "provider") {
    return `P-${classifyRoleToken(node)}`;
  }
  if (type === "indicator" || type === "structure" || type === "mechanics") {
    return `A-${capitalize(type)}`;
  }
  const fallback = normalizeToken(String(node.name || node.id || ""), 10);
  return fallback || "NODE";
}

function classifyRoleToken(node) {
  const candidates = [node.role, node.stage, node.name, node.data?.agentKey, node.data?.title]
    .filter(Boolean)
    .map((v) => String(v).toLowerCase());
  if (candidates.some((v) => v.includes("indicator"))) return "Indicator";
  if (candidates.some((v) => v.includes("structure"))) return "Structure";
  if (candidates.some((v) => v.includes("mechanics"))) return "Mechanics";
  return "Provider";
}

function capitalize(text) {
  const str = String(text || "").trim();
  if (!str) return "";
  return str.charAt(0).toUpperCase() + str.slice(1);
}

function normalizeToken(text, maxLen = 10) {
  const clean = String(text || "")
    .replace(/[^a-zA-Z0-9_-]+/g, "")
    .trim();
  if (!clean) return "";
  if (clean.length <= maxLen) return clean.toUpperCase();
  return `${clean.slice(0, maxLen - 1).toUpperCase()}~`;
}

function compactSymbol(text) {
  const raw = String(text || "").trim();
  if (!raw) return "";
  const upper = raw.toUpperCase();
  const suffixes = ["USDT", "USD", "USDC", "PERP", "SWAP"];
  for (const suffix of suffixes) {
    if (upper.endsWith(suffix) && upper.length > suffix.length + 1) {
      return normalizeToken(upper.slice(0, -suffix.length), 6);
    }
  }
  return normalizeToken(upper, 6);
}

function setPanelCollapsed(side, collapsed) {
  const key = side === "right" ? "right" : "left";
  const className = key === "right" ? "right-panel-collapsed" : "left-panel-collapsed";
  document.body.classList.toggle(className, Boolean(collapsed));

  if (key === "left" && state.ui.leftPanelToggle && state.ui.leftPanelHandle) {
    state.ui.leftPanelToggle.setAttribute("aria-expanded", String(!collapsed));
    state.ui.leftPanelHandle.setAttribute("aria-expanded", String(!collapsed));
  }
  if (key === "right" && state.ui.rightPanelToggle && state.ui.rightPanelHandle) {
    state.ui.rightPanelToggle.setAttribute("aria-expanded", String(!collapsed));
    state.ui.rightPanelHandle.setAttribute("aria-expanded", String(!collapsed));
  }

  requestAnimationFrame(() => {
    resizeAllGraphs();
    if (!collapsed && state.currentGraphData.nodes.length) {
      fitGraph(240);
    }
  });
}

function syncModeToggleLabel() {
  if (!state.ui.modeToggleLabel) return;
  state.ui.modeToggleLabel.textContent = state.ui.modeToggle?.checked ? "3D" : "2D";
}

function decodeEscapedText(text) {
  return String(text || "")
    .replace(/\r/g, "")
    .replace(/\\n/g, "\n")
    .replace(/\\t/g, "  ")
    .replace(/\\"/g, '"');
}

function tryParseJSON(text) {
  const str = String(text || "").trim();
  if (!str.startsWith("{") && !str.startsWith("[")) return null;
  try {
    return JSON.stringify(JSON.parse(str), null, 2);
  } catch (err) {
    return null;
  }
}

function escapeHTML(str) {
  return String(str || "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function escapeAttr(str) {
  return escapeHTML(str).replace(/"/g, "&quot;");
}

function formatExternalLink(url) {
  const raw = String(url || "").trim();
  if (!raw) return "—";
  if (!/^https?:\/\//i.test(raw)) return escapeHTML(raw);
  return `<a href="${escapeAttr(raw)}" target="_blank" rel="noopener noreferrer">${escapeHTML(raw)}</a>`;
}

function countRounds() {
  let total = 0;
  state.roundsBySymbol.forEach((v) => {
    total += v.length;
  });
  return total;
}

function findLatestRound(rounds) {
  if (!Array.isArray(rounds) || !rounds.length) return null;
  let latest = rounds[0];
  rounds.forEach((r, idx) => {
    const best = latest?.timestamp || 0;
    const curr = r?.timestamp || 0;
    if (curr > best || (curr === best && idx === rounds.length - 1)) {
      latest = r;
    }
  });
  return latest;
}

function stageOrder(stage) {
  const order = ["provider", "indicator", "structure", "mechanics", "gate", "execution"];
  const idx = order.indexOf(stage);
  return idx >= 0 ? idx : order.length + 1;
}

function isAgentRole(role) {
  const r = String(role || "").toLowerCase();
  if (r.includes("agent")) return true;
  return ["indicator", "structure", "mechanics"].includes(r);
}
