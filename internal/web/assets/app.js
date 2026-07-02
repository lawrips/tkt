(function () {
  const MOBILE_QUERY = window.matchMedia("(max-width: 720px)");

  const SIDEBAR_KEY = "tkt-web-sidebar-collapsed";
  const PANE_WIDTHS_KEY = "tkt-web-pane-widths";
  const PROJECT_KEY = "tkt-web-selected-project";
  const VIEW_MODES_KEY = "tkt-web-view-modes";
  const BOARD_COLUMNS = [
    { status: "open", label: "Open" },
    { status: "in_progress", label: "In progress" },
    { status: "needs_testing", label: "Needs testing" },
    { status: "closed", label: "Closed" }
  ];
  const PANE_DEFAULTS = { sidebar: 260, list: 420 };
  const PANE_LIMITS = {
    sidebar: { min: 180, max: 420 },
    list: { min: 280, max: 960 },
    detail: { min: 320 }
  };
  const HANDLE_SIZE = 8;
  const SORT_DEFAULTS = {
    title: "asc",
    priority: "asc",
    modified: "desc",
    type: "asc"
  };

  const state = {
    token: "",
    projects: [],
    overview: null,
    selectedProject: "",
    tickets: [],
    selectedTicket: "",
    ticketHistory: [],
    detail: null,
    health: null,
    healthError: "",
    healthLoaded: false,
    dashboard: null,
    dashboardError: "",
    dashboardProject: "",
    timelineWeeks: 8,
    expandedWeek: "",
    editMessage: "",
    editing: false,
    activeView: "tickets",
    mobileView: "list",
    viewMode: "list",
    boardDetailOpen: false,
    boardNotice: "",
    sidebarCollapsed: false,
    sidebarDrawerOpen: false,
    paneWidths: { sidebar: PANE_DEFAULTS.sidebar, list: PANE_DEFAULTS.list },
    filters: {
      search: "",
      sort: "modified:desc",
      parent: "",
      status: "",
      type: ""
    }
  };

  const els = {
    workspace: document.getElementById("workspace"),
    sidebar: document.getElementById("sidebar"),
    sidebarBackdrop: document.getElementById("sidebar-backdrop"),
    openSidebar: document.getElementById("open-sidebar"),
    toggleSidebar: document.getElementById("toggle-sidebar"),
    navTickets: document.getElementById("nav-tickets"),
    navDashboard: document.getElementById("nav-dashboard"),
    navHealth: document.getElementById("nav-health"),
    healthColumn: document.getElementById("health-column"),
    dashboardColumn: document.getElementById("dashboard-column"),
    dashboard: document.getElementById("dashboard-panel"),
    dashboardHeading: document.getElementById("dashboard-heading"),
    refreshDashboard: document.getElementById("refresh-dashboard"),
    ticketColumn: document.getElementById("ticket-column"),
    detailColumn: document.getElementById("detail-column"),
    resizeSidebar: document.getElementById("resize-sidebar"),
    resizeList: document.getElementById("resize-list"),
    detailToolbar: document.getElementById("detail-toolbar"),
    detailBack: document.getElementById("detail-back"),
    status: document.getElementById("session-status"),
    setup: document.getElementById("setup-panel"),
    projects: document.getElementById("project-list"),
    refreshProjects: document.getElementById("refresh-projects"),
    health: document.getElementById("health-panel"),
    refreshHealth: document.getElementById("refresh-health"),
    ticketHeading: document.getElementById("ticket-heading"),
    ticketCount: document.getElementById("ticket-count"),
    viewModeList: document.getElementById("view-mode-list"),
    viewModeBoard: document.getElementById("view-mode-board"),
    boardBackdrop: document.getElementById("board-detail-backdrop"),
    filters: document.getElementById("filters"),
    search: document.getElementById("search"),
    statusFilter: document.getElementById("status"),
    typeFilter: document.getElementById("type"),
    tickets: document.getElementById("ticket-list"),
    detail: document.getElementById("ticket-detail")
  };

  function isMobile() {
    return MOBILE_QUERY.matches;
  }

  function setMobileView(view) {
    state.mobileView = view === "detail" ? "detail" : "list";
    updateLayoutClasses();
  }

  function resetTicketHistory() {
    state.ticketHistory = [];
  }

  function pushTicketHistory(ticketID) {
    if (!ticketID) return;
    const history = state.ticketHistory;
    if (history.length && history[history.length - 1] === ticketID) return;
    history.push(ticketID);
  }

  function updateBackToolbar() {
    if (!els.detailToolbar) return;
    const showMobileDetail = state.activeView === "tickets" && isMobile() && state.mobileView === "detail" && !!state.selectedTicket;
    const showHistoryBack = state.ticketHistory.length > 0 && !!state.selectedTicket;
    els.detailToolbar.hidden = !(showMobileDetail || showHistoryBack);
  }

  async function goBack() {
    if (state.ticketHistory.length) {
      const previous = state.ticketHistory.pop();
      state.editMessage = "";
      state.editing = false;
      await loadDetail(previous, { preserveMobileView: true, fromHistory: true });
      return;
    }
    if (isMobile()) {
      setMobileView("list");
    }
  }

  function parseSort(sort) {
    if (!sort) return { field: "modified", dir: "desc" };
    if (sort.endsWith(":desc")) return { field: sort.slice(0, -5), dir: "desc" };
    if (sort.endsWith(":asc")) return { field: sort.slice(0, -4), dir: "asc" };
    return { field: sort, dir: "asc" };
  }

  function formatSort(sort) {
    const parsed = parseSort(sort);
    return parsed.dir === "desc" ? parsed.field + ":desc" : parsed.field;
  }

  function toggleSortField(field) {
    const current = parseSort(state.filters.sort);
    if (current.field === field) {
      state.filters.sort = current.dir === "desc" ? field : field + ":desc";
      return;
    }
    const defaultDir = SORT_DEFAULTS[field] || "asc";
    state.filters.sort = defaultDir === "desc" ? field + ":desc" : field;
  }

  function sortIndicator(field) {
    const current = parseSort(state.filters.sort);
    if (current.field !== field) return "";
    return current.dir === "desc" ? " ↓" : " ↑";
  }

  function formatTicketDate(value) {
    if (!value) return "—";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return String(value).slice(0, 10);
    return date.toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric"
    });
  }

  function clamp(value, min, max) {
    return Math.min(max, Math.max(min, value));
  }

  function loadPaneWidths() {
    try {
      const raw = window.localStorage.getItem(PANE_WIDTHS_KEY);
      if (!raw) return;
      const parsed = JSON.parse(raw);
      if (typeof parsed.sidebar === "number") {
        state.paneWidths.sidebar = clamp(parsed.sidebar, PANE_LIMITS.sidebar.min, PANE_LIMITS.sidebar.max);
      }
      if (typeof parsed.list === "number") {
        state.paneWidths.list = clamp(parsed.list, PANE_LIMITS.list.min, PANE_LIMITS.list.max);
      }
    } catch (_err) {
      // Ignore invalid stored layout.
    }
  }

  function savePaneWidths() {
    window.localStorage.setItem(PANE_WIDTHS_KEY, JSON.stringify(state.paneWidths));
  }

  function sidebarWidthPx() {
    return state.sidebarCollapsed ? 0 : state.paneWidths.sidebar;
  }

  function maxListWidthPx() {
    if (!els.workspace) return PANE_LIMITS.list.max;
    const workspaceWidth = els.workspace.getBoundingClientRect().width;
    const handles = state.activeView === "health"
      ? (state.sidebarCollapsed ? 0 : HANDLE_SIZE)
      : (state.sidebarCollapsed ? HANDLE_SIZE : HANDLE_SIZE * 2);
    const available = workspaceWidth - sidebarWidthPx() - handles - PANE_LIMITS.detail.min;
    return clamp(available, PANE_LIMITS.list.min, PANE_LIMITS.list.max);
  }

  function applyPaneSizes() {
    if (!els.workspace) return;
    if (isMobile()) {
      els.workspace.style.gridTemplateColumns = "";
      if (els.resizeSidebar) els.resizeSidebar.hidden = true;
      if (els.resizeList) els.resizeList.hidden = true;
      return;
    }

    state.paneWidths.list = clamp(state.paneWidths.list, PANE_LIMITS.list.min, maxListWidthPx());

    if (els.resizeSidebar) {
      els.resizeSidebar.hidden = state.sidebarCollapsed;
    }

    if (state.activeView === "health" || state.activeView === "dashboard" || boardModeActive()) {
      if (els.resizeList) els.resizeList.hidden = true;
      if (state.sidebarCollapsed) {
        els.workspace.style.gridTemplateColumns = "minmax(" + PANE_LIMITS.detail.min + "px, 1fr)";
      } else {
        els.workspace.style.gridTemplateColumns = sidebarWidthPx() + "px " + HANDLE_SIZE + "px minmax(" + PANE_LIMITS.detail.min + "px, 1fr)";
      }
      return;
    }

    if (els.resizeList) els.resizeList.hidden = false;

    if (state.sidebarCollapsed) {
      els.workspace.style.gridTemplateColumns = [
        state.paneWidths.list + "px",
        HANDLE_SIZE + "px",
        "minmax(" + PANE_LIMITS.detail.min + "px, 1fr)"
      ].join(" ");
      return;
    }

    els.workspace.style.gridTemplateColumns = [
      state.paneWidths.sidebar + "px",
      HANDLE_SIZE + "px",
      state.paneWidths.list + "px",
      HANDLE_SIZE + "px",
      "minmax(" + PANE_LIMITS.detail.min + "px, 1fr)"
    ].join(" ");
  }

  function initPaneResize() {
    loadPaneWidths();
    applyPaneSizes();

    if (els.resizeSidebar) {
      setupResizeHandle(els.resizeSidebar, "sidebar");
    }
    if (els.resizeList) {
      setupResizeHandle(els.resizeList, "list");
    }

    window.addEventListener("resize", () => {
      applyPaneSizes();
    });
  }

  function setupResizeHandle(handle, target) {
    handle.addEventListener("mousedown", event => {
      if (isMobile() || event.button !== 0) return;
      if (target === "sidebar" && state.sidebarCollapsed) return;
      event.preventDefault();

      const startX = event.clientX;
      const startSidebar = state.paneWidths.sidebar;
      const startList = state.paneWidths.list;

      document.body.classList.add("is-resizing");
      handle.classList.add("active");

      function onMove(moveEvent) {
        const delta = moveEvent.clientX - startX;
        if (target === "sidebar") {
          state.paneWidths.sidebar = clamp(
            startSidebar + delta,
            PANE_LIMITS.sidebar.min,
            PANE_LIMITS.sidebar.max
          );
        } else {
          state.paneWidths.list = clamp(
            startList + delta,
            PANE_LIMITS.list.min,
            maxListWidthPx()
          );
        }
        applyPaneSizes();
      }

      function onUp() {
        document.body.classList.remove("is-resizing");
        handle.classList.remove("active");
        savePaneWidths();
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
      }

      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    });
  }

  function initSidebarState() {
    const stored = storedValue(SIDEBAR_KEY);
    state.sidebarCollapsed = stored === "" ? true : stored === "1";
    updateSidebarLayout();
  }

  function setSidebarCollapsed(collapsed) {
    state.sidebarCollapsed = !!collapsed;
    state.sidebarDrawerOpen = false;
    saveBoolean(SIDEBAR_KEY, state.sidebarCollapsed);
    updateSidebarLayout();
  }

  function sidebarDrawerMode() {
    return state.sidebarCollapsed || isMobile();
  }

  function setSidebarDrawerOpen(open) {
    state.sidebarDrawerOpen = sidebarDrawerMode() && !!open;
    updateSidebarLayout();
  }

  function updateSidebarLayout() {
    const drawerMode = sidebarDrawerMode();
    if (!drawerMode) {
      state.sidebarDrawerOpen = false;
    }
    if (els.workspace) {
      els.workspace.classList.toggle("sidebar-collapsed", drawerMode);
      els.workspace.classList.toggle("sidebar-drawer-open", state.sidebarDrawerOpen);
    }
    if (els.toggleSidebar) {
      const title = drawerMode ? "Close projects" : "Hide projects";
      els.toggleSidebar.setAttribute("aria-expanded", drawerMode ? String(state.sidebarDrawerOpen) : "true");
      els.toggleSidebar.title = title;
      els.toggleSidebar.setAttribute("aria-label", title);
      els.toggleSidebar.textContent = drawerMode ? "×" : "⊟";
    }
    if (els.openSidebar) {
      els.openSidebar.hidden = !drawerMode;
      els.openSidebar.setAttribute("aria-expanded", String(state.sidebarDrawerOpen));
    }
    if (els.sidebarBackdrop) {
      els.sidebarBackdrop.hidden = !state.sidebarDrawerOpen;
    }
    applyPaneSizes();
  }

  function isEpic(ticket) {
    return ticket && ticket.type === "epic";
  }

  function storedViewModes() {
    try {
      const raw = window.localStorage.getItem(VIEW_MODES_KEY);
      const parsed = raw ? JSON.parse(raw) : {};
      return parsed && typeof parsed === "object" ? parsed : {};
    } catch (_err) {
      return {};
    }
  }

  function viewModeFor(projectName) {
    return storedViewModes()[projectName] === "board" ? "board" : "list";
  }

  function saveViewMode(projectName, mode) {
    if (!projectName) return;
    const modes = storedViewModes();
    modes[projectName] = mode;
    saveValue(VIEW_MODES_KEY, JSON.stringify(modes));
  }

  function boardModeActive() {
    return state.activeView === "tickets" && state.viewMode === "board";
  }

  function setViewMode(mode) {
    const next = mode === "board" ? "board" : "list";
    if (next === state.viewMode) return;
    state.viewMode = next;
    state.boardDetailOpen = false;
    state.boardNotice = "";
    saveViewMode(state.selectedProject, state.viewMode);
    syncViewToggle();
    updateLayoutClasses();
    renderTickets();
  }

  function syncViewToggle() {
    if (els.viewModeList) {
      els.viewModeList.setAttribute("aria-pressed", String(state.viewMode !== "board"));
    }
    if (els.viewModeBoard) {
      els.viewModeBoard.setAttribute("aria-pressed", String(state.viewMode === "board"));
    }
  }

  function closeBoardDetail() {
    if (!state.boardDetailOpen) return;
    state.boardDetailOpen = false;
    updateLayoutClasses();
  }

  function childCountsFor(tickets) {
    const counts = {};
    for (const ticket of tickets) {
      if (!ticket.parent) continue;
      counts[ticket.parent] = (counts[ticket.parent] || 0) + 1;
    }
    return counts;
  }

  function setActiveView(view) {
    if (view === "health" || view === "dashboard") {
      state.activeView = view;
    } else {
      state.activeView = "tickets";
    }
    if (els.navTickets) {
      els.navTickets.setAttribute("aria-current", state.activeView === "tickets" ? "page" : "false");
    }
    if (els.navDashboard) {
      els.navDashboard.setAttribute("aria-current", state.activeView === "dashboard" ? "page" : "false");
    }
    if (els.navHealth) {
      els.navHealth.setAttribute("aria-current", state.activeView === "health" ? "page" : "false");
    }
    if (els.healthColumn) {
      els.healthColumn.hidden = state.activeView !== "health";
    }
    if (els.dashboardColumn) {
      els.dashboardColumn.hidden = state.activeView !== "dashboard";
    }
    updateLayoutClasses();
    if (state.activeView === "health" && !state.healthLoaded) {
      loadHealth();
    }
    if (state.activeView === "dashboard" && state.dashboardProject !== state.selectedProject) {
      loadDashboard();
    }
  }

  function applyParentFilter(parentID) {
    state.filters.parent = parentID;
    setActiveView("tickets");
    loadTickets();
  }

  function updateLayoutClasses() {
    if (!els.workspace) return;
    els.workspace.classList.toggle("view-tickets", state.activeView === "tickets");
    els.workspace.classList.toggle("view-health", state.activeView === "health");
    els.workspace.classList.toggle("view-dashboard", state.activeView === "dashboard");
    const showDetail = state.activeView === "tickets" && isMobile() && state.mobileView === "detail" && !!state.selectedTicket;
    els.workspace.classList.toggle("view-detail", showDetail);
    els.workspace.classList.toggle("view-list", state.activeView === "tickets" && !showDetail);
    els.workspace.classList.toggle("view-board", boardModeActive());
    const drawerOpen = boardModeActive() && state.boardDetailOpen && !isMobile();
    els.workspace.classList.toggle("board-detail-open", drawerOpen);
    if (els.boardBackdrop) {
      els.boardBackdrop.hidden = !drawerOpen;
    }
    updateBackToolbar();
    applyPaneSizes();
  }

  function scrollTicketIntoView() {
    if (!state.selectedTicket || !els.tickets) return;
    const selected = els.tickets.querySelector('[data-ticket="' + CSS.escape(state.selectedTicket) + '"]');
    if (selected) {
      selected.scrollIntoView({ block: "nearest" });
    }
  }

  function sortPill(field, label) {
    const active = parseSort(state.filters.sort).field === field;
    return `
      <button type="button" class="sort-pill${active ? " active" : ""}" data-sort-field="${field}" title="Sort by ${escapeHTML(label.toLowerCase())}">
        ${escapeHTML(label)}${sortIndicator(field)}
      </button>
    `;
  }

  function initToken() {
    const params = new URLSearchParams(window.location.search);
    const token = params.get("token") || window.sessionStorage.getItem("tkt-web-token") || "";
    if (token) {
      window.sessionStorage.setItem("tkt-web-token", token);
      state.token = token;
    }
    if (params.has("token")) {
      window.history.replaceState({}, document.title, window.location.pathname);
    }
  }

  async function api(path, options) {
    const response = await fetch(path, {
      ...options,
      headers: {
        "Authorization": "Bearer " + state.token,
        "Content-Type": "application/json",
        ...(options && options.headers ? options.headers : {})
      }
    });
    const text = await response.text();
    const payload = text ? JSON.parse(text) : null;
    if (!response.ok) {
      const error = payload && payload.error ? payload.error : { message: response.statusText };
      const failure = new Error(error.message || "Request failed");
      failure.code = error.code || "";
      throw failure;
    }
    return payload;
  }

  async function mutate(path, payload, options) {
    return api(path, {
      method: options && options.method ? options.method : "POST",
      body: payload == null ? undefined : JSON.stringify(payload)
    });
  }

  function escapeHTML(value) {
    return String(value == null ? "" : value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function renderMarkdown(content) {
    if (window.tktMarkdown && typeof window.tktMarkdown.render === "function") {
      return window.tktMarkdown.render(content);
    }
    const text = String(content || "").trim();
    if (!text) {
      return '<p class="markdown-empty muted">No content.</p>';
    }
    return `<p class="markdown-text">${escapeHTML(text)}</p>`;
  }

  function sectionHeading(value) {
    return String(value || "").trim().toLowerCase();
  }

  function storedValue(key) {
    try {
      return window.localStorage.getItem(key) || "";
    } catch (_err) {
      return "";
    }
  }

  function saveValue(key, value) {
    try {
      if (value) {
        window.localStorage.setItem(key, value);
      } else {
        window.localStorage.removeItem(key);
      }
    } catch (_err) {
      // Ignore storage failures; the UI still works for this page load.
    }
  }

  function storedBoolean(key) {
    return storedValue(key) === "1";
  }

  function saveBoolean(key, value) {
    saveValue(key, value ? "1" : "0");
  }

  function storedProjectName() {
    return storedValue(PROJECT_KEY);
  }

  function saveSelectedProject(name) {
    saveValue(PROJECT_KEY, name);
  }

  function chooseInitialProject(projects, overview) {
    const names = new Set(projects.map(project => project.name));
    const stored = storedProjectName();
    if (stored && names.has(stored)) {
      return stored;
    }
    if (stored) {
      saveSelectedProject("");
    }
    if (overview.initialized && names.has(overview.resolved_project)) {
      return overview.resolved_project;
    }
    return (projects[0] && projects[0].name) || "";
  }

  function notesContent(detail) {
    const sections = detail.other_sections || [];
    const notes = sections.find(item => sectionHeading(item.Heading || item.heading) === "notes");
    return notes ? (notes.Content || notes.content || "") : "";
  }

  function notesTimeline(content) {
    const trimmed = String(content || "").trim();
    if (!trimmed) {
      return `
        <section class="notes-log">
          <h3 class="section-label">Notes</h3>
          <p class="muted">No notes yet.</p>
        </section>
      `;
    }
    const entries = splitNoteEntries(trimmed);
    const items = (entries.length > 1 ? entries : [trimmed]).map(entry => `
      <div class="note-entry">
        <div class="note-marker" aria-hidden="true"></div>
        <div class="note-body markdown-body">${renderMarkdown(entry)}</div>
      </div>
    `).join("");
    return `
      <section class="notes-log">
        <h3 class="section-label">Notes</h3>
        <div class="note-timeline">${items}</div>
      </section>
    `;
  }

  function splitNoteEntries(content) {
    const ruleSeparated = content.split(/\n---\n/).map(entry => entry.trim()).filter(Boolean);
    if (ruleSeparated.length > 1) return ruleSeparated;
    const headerSeparated = content
      .split(/\n{2,}(?=\*\*\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})/)
      .map(entry => entry.trim())
      .filter(Boolean);
    return headerSeparated.length ? headerSeparated : [content];
  }

  function docSection(title, content) {
    const trimmed = String(content || "").trim();
    if (!trimmed) return "";
    return `
      <section class="doc-section">
        <h3 class="section-label">${escapeHTML(title)}</h3>
        <div class="markdown-body">${renderMarkdown(trimmed)}</div>
      </section>
    `;
  }

  function asideGroup(title, body) {
    return `
      <section class="aside-group">
        <h3 class="aside-label">${escapeHTML(title)}</h3>
        ${body}
      </section>
    `;
  }

  function setStatus(text, className) {
    els.status.textContent = text;
    els.status.className = "status-pill" + (className ? " " + className : "");
  }

  async function loadSession() {
    if (!state.token) {
      setStatus("Missing token", "warn");
      els.setup.innerHTML = notice("Open TKT Web from the authenticated URL printed by `tkt web`.", "warning");
      return;
    }
    setStatus("Loading");
    try {
      const session = await api("/api/session");
      state.overview = session.projects;
      state.projects = session.projects.projects || [];
      state.selectedProject = chooseInitialProject(state.projects, session.projects);
      state.viewMode = viewModeFor(state.selectedProject);
      syncViewToggle();
      updateLayoutClasses();
      setStatus("Connected", "ok");
      renderProjects();
      renderSetup();
      syncFilterControls();
      if (state.selectedProject) {
        await loadTickets();
      } else {
        renderTickets();
      }
    } catch (err) {
      setStatus("Cannot connect", "warn");
      els.setup.innerHTML = notice(err.message, "error");
    }
  }

  async function loadHealth() {
    if (!els.health) return;
    state.healthError = "";
    els.health.innerHTML = "<p class=\"loading\">Checking setup...</p>";
    try {
      state.health = await api("/api/health");
      state.healthLoaded = true;
      renderHealth();
    } catch (err) {
      state.health = null;
      state.healthLoaded = true;
      state.healthError = err.message || "Health check unavailable.";
      renderHealth();
    }
  }

  function syncFilterControls() {
    if (els.search) els.search.value = state.filters.search;
    if (els.statusFilter) els.statusFilter.value = state.filters.status;
    if (els.typeFilter) els.typeFilter.value = state.filters.type;
  }

  function readFiltersFromControls() {
    state.filters.search = els.search ? els.search.value.trim() : "";
    state.filters.status = els.statusFilter ? els.statusFilter.value : "";
    state.filters.type = els.typeFilter ? els.typeFilter.value : "";
  }

  function renderHealth() {
    if (state.healthError) {
      els.health.innerHTML = notice(escapeHTML(state.healthError), "error");
      return;
    }
    const report = state.health;
    if (!report) {
      els.health.innerHTML = "";
      return;
    }
    const summary = report.summary || {};
    const checks = report.checks || [];
    const status = report.status || "warn";
    els.health.innerHTML = `
      <div class="health-summary">
        <span class="status-pill ${statusClass(status)}">${escapeHTML(status.toUpperCase())}</span>
        <span class="row-meta">${Number(summary.pass || 0)} pass / ${Number(summary.warn || 0)} warn / ${Number(summary.fail || 0)} fail</span>
      </div>
      ${healthGroups(checks)}
    `;
  }

  function healthGroups(checks) {
    const categories = ["global", "project", "sync", "agent"];
    return categories.map(category => {
      const items = checks.filter(check => check.category === category);
      if (!items.length) return "";
      return `
        <section class="health-group">
          <h3>${escapeHTML(categoryTitle(category))}</h3>
          <div class="health-checks">
            ${items.map(healthCheck).join("")}
          </div>
        </section>
      `;
    }).join("");
  }

  function healthCheck(check) {
    const status = check.status || "warn";
    return `
      <article class="health-check ${escapeHTML(status)}">
        <div class="health-check-heading">
          <span class="health-status">${escapeHTML(status.toUpperCase())}</span>
          <span>${escapeHTML(check.message || "")}</span>
        </div>
        ${check.remediation ? `<p>${escapeHTML(check.remediation)}</p>` : ""}
        ${check.command ? `<code>${escapeHTML(check.command)}</code>` : ""}
      </article>
    `;
  }

  function categoryTitle(category) {
    if (category === "global") return "Global";
    if (category === "project") return "Project";
    if (category === "sync") return "Sync";
    if (category === "agent") return "Agent";
    return category || "Other";
  }

  function statusClass(status) {
    if (status === "pass") return "ok";
    if (status === "fail") return "fail";
    return "warn";
  }

  /* ── Dashboard ── */

  const TIMELINE_WEEK_OPTIONS = [4, 8, 12, 26];
  const STATUS_LABELS = {
    open: "Open",
    in_progress: "In progress",
    needs_testing: "Needs testing",
    closed: "Closed"
  };

  async function loadDashboard() {
    if (!els.dashboard) return;
    state.dashboardError = "";
    state.expandedWeek = "";
    if (els.dashboardHeading) {
      els.dashboardHeading.textContent = state.selectedProject || "Dashboard";
    }
    if (!state.selectedProject) {
      state.dashboard = null;
      state.dashboardProject = "";
      els.dashboard.innerHTML = "<p class=\"muted\">Choose a configured project to see its dashboard.</p>";
      return;
    }
    els.dashboard.innerHTML = "<p class=\"loading\">Loading dashboard...</p>";
    const project = state.selectedProject;
    const base = `/api/projects/${encodeURIComponent(project)}`;
    try {
      const [stats, timeline, epics, ready, blocked, all] = await Promise.all([
        api(`${base}/insights/stats`),
        api(`${base}/insights/timeline?weeks=${state.timelineWeeks}`),
        api(`${base}/insights/epics`),
        api(`${base}/tickets?ready=true&sort=priority`),
        api(`${base}/tickets?blocked=true&sort=priority`),
        api(`${base}/tickets`)
      ]);
      if (state.selectedProject !== project) return;
      const byID = {};
      for (const item of all.items || []) {
        byID[item.id] = item;
      }
      state.dashboard = {
        stats,
        timeline: timeline.weeks || [],
        epics: epics.epics || [],
        ready: ready.items || [],
        blocked: blocked.items || [],
        byID
      };
      state.dashboardProject = project;
      renderDashboard();
    } catch (err) {
      if (state.selectedProject !== project) return;
      state.dashboard = null;
      state.dashboardProject = "";
      state.dashboardError = err.message || "Dashboard unavailable.";
      renderDashboard();
    }
  }

  function renderDashboard() {
    if (!els.dashboard) return;
    if (els.dashboardHeading) {
      els.dashboardHeading.textContent = state.selectedProject || "Dashboard";
    }
    if (state.dashboardError) {
      els.dashboard.innerHTML = notice(escapeHTML(state.dashboardError), "error");
      return;
    }
    const data = state.dashboard;
    if (!data) {
      els.dashboard.innerHTML = "";
      return;
    }
    els.dashboard.innerHTML = `
      ${overviewSection(data.stats)}
      ${queueSection("Ready to start", data.ready, "ready")}
      ${queueSection("Blocked", data.blocked, "blocked")}
      ${timelineSection(data.timeline)}
      ${epicsSection(data.epics)}
    `;
    applyBarWidths();
  }

  function applyBarWidths() {
    els.dashboard.querySelectorAll("[data-bar-width]").forEach(el => {
      el.style.width = el.dataset.barWidth + "%";
    });
  }

  function barWidth(count, max) {
    if (!max || !count) return 0;
    return Math.max(2, Math.round((count / max) * 100));
  }

  function overviewSection(stats) {
    const byStatus = stats.by_status || {};
    const cards = [
      { label: "Total", value: stats.total || 0 },
      { label: "Open", value: byStatus.open || 0 },
      { label: "In progress", value: byStatus.in_progress || 0 },
      { label: "Needs testing", value: byStatus.needs_testing || 0 },
      { label: "Closed", value: byStatus.closed || 0 },
      { label: "Ready", value: stats.ready || 0, kind: "ready" },
      { label: "Blocked", value: stats.blocked || 0, kind: "blocked" }
    ];
    const cardHTML = cards.map(card => `
      <div class="stat-card${card.kind ? " stat-card-" + card.kind : ""}">
        <span class="stat-value">${Number(card.value)}</span>
        <span class="stat-label">${escapeHTML(card.label)}</span>
      </div>
    `).join("");
    return `
      <section class="insight-section">
        <h3 class="section-label">Overview</h3>
        <div class="stat-cards">${cardHTML}</div>
        ${distributionBlock("By type", stats.by_type || {})}
        ${distributionBlock("By priority", priorityEntries(stats.by_priority || {}))}
      </section>
    `;
  }

  function priorityEntries(byPriority) {
    const entries = {};
    for (let p = 0; p <= 4; p++) {
      const count = Number(byPriority[p] || byPriority[String(p)] || 0);
      if (count > 0) entries["p" + p] = count;
    }
    return entries;
  }

  function distributionBlock(title, counts) {
    const entries = Object.keys(counts)
      .map(key => ({ key, count: Number(counts[key] || 0) }))
      .filter(entry => entry.count > 0)
      .sort((a, b) => b.count - a.count || (a.key < b.key ? -1 : 1));
    if (!entries.length) return "";
    const max = entries[0].count;
    const rows = entries.map(entry => `
      <div class="insight-dist-row">
        <span class="insight-dist-label">${escapeHTML(entry.key)}</span>
        <div class="insight-bar-track"><div class="insight-bar-fill" data-bar-width="${barWidth(entry.count, max)}"></div></div>
        <span class="insight-dist-count">${entry.count}</span>
      </div>
    `).join("");
    return `
      <div class="insight-distribution">
        <h4 class="aside-label">${escapeHTML(title)}</h4>
        ${rows}
      </div>
    `;
  }

  function queueSection(title, items, kind) {
    let body;
    if (!items.length) {
      body = kind === "ready"
        ? '<p class="muted">Nothing is ready to start.</p>'
        : '<p class="muted">No blocked tickets.</p>';
    } else {
      body = `<div class="queue-list">${items.map(item => queueItem(item, kind)).join("")}</div>`;
    }
    return `
      <section class="insight-section">
        <div class="insight-section-head">
          <h3 class="section-label">${escapeHTML(title)}</h3>
          <span class="insight-section-count">${items.length}</span>
        </div>
        ${body}
      </section>
    `;
  }

  function queueItem(item, kind) {
    const epic = isEpic(item);
    const meta = [item.id, item.type, "p" + Number(item.priority || 0)].filter(Boolean).join(" · ");
    const blockers = kind === "blocked" ? blockingDeps(item) : [];
    const blockerHTML = blockers.length
      ? `<span class="queue-blockers">blocked by ${blockers.map(dep => escapeHTML(dep)).join(", ")}</span>`
      : "";
    return `
      <button class="queue-item" type="button" data-nav-ticket="${escapeHTML(item.id)}">
        <span class="ticket-row-top">
          ${epic ? '<span class="type-mark type-mark-epic">Epic</span>' : ""}
          <span class="row-title">${escapeHTML(item.title || "(untitled)")}</span>
        </span>
        <span class="row-meta">${escapeHTML(meta)}</span>
        ${blockerHTML}
      </button>
    `;
  }

  function blockingDeps(item) {
    const byID = state.dashboard ? state.dashboard.byID : {};
    return (item.deps || []).filter(dep => {
      const target = byID[dep];
      return !target || target.status !== "closed";
    });
  }

  function formatWeekLabel(key) {
    const date = new Date(key + "T00:00:00Z");
    if (Number.isNaN(date.getTime())) return key;
    return date.toLocaleDateString(undefined, { month: "short", day: "numeric", timeZone: "UTC" });
  }

  function mondayKey(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "";
    const day = date.getUTCDay() === 0 ? 7 : date.getUTCDay();
    const monday = new Date(Date.UTC(date.getUTCFullYear(), date.getUTCMonth(), date.getUTCDate() - (day - 1)));
    return monday.toISOString().slice(0, 10);
  }

  function timelineSection(weeks) {
    const pills = TIMELINE_WEEK_OPTIONS.map(option => `
      <button type="button" class="sort-pill${option === state.timelineWeeks ? " active" : ""}" data-weeks="${option}">${option}w</button>
    `).join("");
    const max = weeks.reduce((acc, week) => Math.max(acc, week.closed_count || 0), 0);
    const hasClosed = max > 0;
    const rows = weeks.map(week => {
      const key = week.week_start;
      const expanded = state.expandedWeek === key;
      return `
        <button type="button" class="timeline-row${expanded ? " expanded" : ""}" data-week-toggle="${escapeHTML(key)}" aria-expanded="${expanded}">
          <span class="timeline-week" title="Week of ${escapeHTML(key)}">${escapeHTML(formatWeekLabel(key))}</span>
          <span class="insight-bar-track"><span class="insight-bar-fill" data-bar-width="${barWidth(week.closed_count || 0, max)}"></span></span>
          <span class="timeline-count">${Number(week.closed_count || 0)}</span>
        </button>
        ${expanded ? weekTicketList(key) : ""}
      `;
    }).join("");
    return `
      <section class="insight-section">
        <div class="insight-section-head">
          <h3 class="section-label">Closed per week</h3>
          <div class="weeks-toggle" role="group" aria-label="Timeline window">${pills}</div>
        </div>
        ${hasClosed ? "" : '<p class="muted">No tickets closed in this window.</p>'}
        <div class="timeline-rows">${rows}</div>
      </section>
    `;
  }

  function weekTicketList(weekKey) {
    const byID = state.dashboard ? state.dashboard.byID : {};
    const closed = Object.values(byID)
      .filter(item => item.status === "closed" && mondayKey(item.created) === weekKey)
      .sort((a, b) => (a.created < b.created ? 1 : -1));
    if (!closed.length) {
      return '<p class="muted timeline-week-empty">No closed tickets recorded for this week.</p>';
    }
    const items = closed.map(item => `
      <button class="queue-item" type="button" data-nav-ticket="${escapeHTML(item.id)}">
        <span class="ticket-row-top"><span class="row-title">${escapeHTML(item.title || "(untitled)")}</span></span>
        <span class="row-meta">${escapeHTML(item.id)} · ${escapeHTML(item.type || "")}</span>
      </button>
    `).join("");
    return `<div class="timeline-week-tickets">${items}</div>`;
  }

  function epicsSection(epics) {
    let body;
    if (!epics.length) {
      body = '<p class="muted">No epics in this project.</p>';
    } else {
      body = `<div class="epic-rows">${epics.map(epicRow).join("")}</div>`;
    }
    return `
      <section class="insight-section">
        <div class="insight-section-head">
          <h3 class="section-label">Epics</h3>
          <span class="insight-section-count">${epics.length}</span>
        </div>
        ${body}
      </section>
    `;
  }

  function epicRow(epic) {
    const total = Number(epic.total_children || 0);
    const closed = Number(epic.closed_children || 0);
    const pct = total ? Math.round((closed / total) * 100) : 0;
    const statusCounts = epic.children_by_status || {};
    const chips = Object.keys(STATUS_LABELS)
      .filter(status => statusCounts[status])
      .map(status => badge(`${statusCounts[status]} ${STATUS_LABELS[status].toLowerCase()}`, "status-" + status))
      .join("");
    const progress = total
      ? `
        <div class="epic-progress">
          <span class="insight-bar-track"><span class="insight-bar-fill" data-bar-width="${pct}"></span></span>
          <span class="epic-progress-count">${closed}/${total}</span>
        </div>
      `
      : '<p class="muted epic-no-children">No children yet.</p>';
    return `
      <div class="epic-row">
        <div class="epic-row-head">
          <button class="epic-link" type="button" data-nav-ticket="${escapeHTML(epic.id)}">
            <span class="row-title">${escapeHTML(epic.title || "(untitled)")}</span>
            <span class="row-meta">${escapeHTML(epic.id)} · ${escapeHTML(epic.status || "")}</span>
          </button>
          ${total ? `<button class="small-button" type="button" data-parent-filter="${escapeHTML(epic.id)}">Children</button>` : ""}
        </div>
        ${progress}
        ${chips ? `<div class="badge-row">${chips}</div>` : ""}
      </div>
    `;
  }

  async function openTicketFromDashboard(ticketID) {
    if (!ticketID) return;
    setActiveView("tickets");
    state.editMessage = "";
    await loadDetail(ticketID, { resetHistory: true });
  }

  function renderSetup() {
    if (!state.overview) {
      els.setup.innerHTML = "";
      return;
    }
    if (state.overview.initialized) {
      els.setup.innerHTML = "";
      return;
    }
    const message = state.overview.message || "No TKT project is configured for this directory.";
    els.setup.innerHTML = notice(
      escapeHTML(message) + "<br><br><code>tkt init</code>",
      "warning"
    );
  }

  function renderProjects() {
    if (!state.projects.length) {
      els.projects.innerHTML = "<p class=\"muted\">No configured projects.</p>";
      return;
    }
    els.projects.innerHTML = state.projects.map(project => {
      const current = project.name === state.selectedProject;
      const meta = [project.store, project.ticket_dir].filter(Boolean).join(" / ");
      return `
        <button class="project-button" type="button" data-project="${escapeHTML(project.name)}" aria-current="${current}">
          <span class="row-title">${escapeHTML(project.name)}</span>
          <span class="row-meta">${escapeHTML(meta || "No store")}</span>
        </button>
      `;
    }).join("");
  }

  async function loadTickets() {
    if (!state.selectedProject) {
      renderTickets();
      updateLayoutClasses();
      return;
    }
    state.boardNotice = "";
    els.tickets.innerHTML = "<p class=\"loading\">Loading tickets...</p>";
    const params = new URLSearchParams();
    if (state.filters.search) params.set("search", state.filters.search);
    if (state.filters.sort) params.set("sort", formatSort(state.filters.sort));
    if (state.filters.parent) params.set("parent", state.filters.parent);
    if (state.filters.status) params.set("status", state.filters.status);
    if (state.filters.type) params.set("type", state.filters.type);
    try {
      const list = await api(`/api/projects/${encodeURIComponent(state.selectedProject)}/tickets?${params}`);
      state.tickets = list.items || [];
      renderTickets();
      if (state.selectedTicket && state.tickets.some(t => t.id === state.selectedTicket)) {
        await loadDetail(state.selectedTicket, { preserveMobileView: true });
      } else {
        state.selectedTicket = "";
        state.detail = null;
        state.editing = false;
        resetTicketHistory();
        setMobileView("list");
        renderDetail();
      }
    } catch (err) {
      els.tickets.innerHTML = notice(escapeHTML(err.message), "error");
    }
  }

  function renderTickets() {
    const projectLabel = state.selectedProject || "No project";
    els.ticketHeading.textContent = projectLabel;
    const countLabel = `${state.tickets.length} ticket${state.tickets.length === 1 ? "" : "s"}`;
    if (state.filters.parent) {
      els.ticketCount.innerHTML = `
        ${escapeHTML(countLabel)}
        <span class="filter-chip">children of ${escapeHTML(state.filters.parent)}
          <button type="button" class="filter-chip-clear" data-clear-parent-filter title="Show all tickets" aria-label="Show all tickets">&times;</button>
        </span>
      `;
    } else {
      els.ticketCount.textContent = countLabel;
    }
    if (!state.selectedProject) {
      els.tickets.innerHTML = "<p class=\"muted\">Choose a configured project or run `tkt init`.</p>";
      return;
    }
    if (boardModeActive()) {
      els.tickets.innerHTML = boardView();
      return;
    }
    if (!state.tickets.length) {
      els.tickets.innerHTML = "<p class=\"muted\">No tickets match the current filters.</p>";
      return;
    }
    const childCounts = childCountsFor(state.tickets);
    els.tickets.innerHTML = `
      <div class="ticket-inbox">
        <div class="ticket-sort-strip" aria-label="Ticket sorting">
          <span class="sort-label">Sort</span>
          ${sortPill("modified", "Date")}
          ${sortPill("priority", "Priority")}
          ${sortPill("title", "Title")}
          ${sortPill("type", "Type")}
        </div>
        <div class="ticket-inbox-list" role="list">
          ${state.tickets.map(ticket => {
            const selected = ticket.id === state.selectedTicket;
            const epic = isEpic(ticket);
            const childCount = epic ? childCounts[ticket.id] || 0 : 0;
            const meta = [
              ticket.id,
              ticket.status,
              ticket.type,
              childCount ? childCount + " children" : ""
            ].filter(Boolean).join(" · ");
            return `
              <div class="ticket-inbox-row${selected ? " selected" : ""}${epic ? " type-epic" : ""}" data-ticket="${escapeHTML(ticket.id)}" tabindex="0" aria-current="${selected}" role="listitem">
                <div class="ticket-row-main">
                  <div class="ticket-cell-title-main">
                    ${epic ? '<span class="type-mark type-mark-epic">Epic</span>' : ""}
                    <span class="row-title">${escapeHTML(ticket.title || "(untitled)")}</span>
                  </div>
                  <div class="row-meta">${escapeHTML(meta)}</div>
                </div>
                <div class="ticket-row-aside">
                  <span class="priority-token">p${Number(ticket.priority || 0)}</span>
                  <span class="ticket-row-date">${escapeHTML(formatTicketDate(ticket.modified))}</span>
                </div>
              </div>
            `;
          }).join("")}
        </div>
      </div>
    `;
  }

  function boardView() {
    const columns = BOARD_COLUMNS.map(column => {
      const tickets = state.tickets.filter(ticket => ticket.status === column.status);
      return `
        <section class="board-column" data-status="${column.status}">
          <header class="board-column-header">
            <h3>${escapeHTML(column.label)}</h3>
            <span class="board-count">${tickets.length}</span>
          </header>
          <div class="board-column-body">
            ${tickets.length ? tickets.map(boardCard).join("") : '<p class="board-empty">No tickets</p>'}
          </div>
        </section>
      `;
    }).join("");
    return `
      <div class="ticket-board">
        <div class="ticket-sort-strip" aria-label="Ticket sorting">
          <span class="sort-label">Sort</span>
          ${sortPill("modified", "Date")}
          ${sortPill("priority", "Priority")}
          ${sortPill("title", "Title")}
          ${sortPill("type", "Type")}
        </div>
        ${state.boardNotice ? `<div class="board-notice notice warning">${escapeHTML(state.boardNotice)}</div>` : ""}
        <div class="board-columns">${columns}</div>
      </div>
    `;
  }

  function boardCard(ticket) {
    const selected = ticket.id === state.selectedTicket;
    const epic = isEpic(ticket);
    return `
      <article class="board-card${selected ? " selected" : ""}${epic ? " type-epic" : ""}" draggable="true" data-ticket="${escapeHTML(ticket.id)}" tabindex="0" aria-current="${selected}">
        <div class="board-card-title-row">
          ${epic ? '<span class="type-mark type-mark-epic">Epic</span>' : ""}
          <span class="row-title">${escapeHTML(ticket.title || "(untitled)")}</span>
        </div>
        <span class="board-card-id">${escapeHTML(ticket.id)}</span>
        <div class="board-card-foot">
          <span class="priority-token">p${Number(ticket.priority || 0)}</span>
          <span class="ticket-row-date">${escapeHTML(formatTicketDate(ticket.modified))}</span>
        </div>
      </article>
    `;
  }

  async function moveTicketStatus(ticketID, status) {
    const ticket = state.tickets.find(item => item.id === ticketID);
    if (!ticket || !status || ticket.status === status) return;
    try {
      const detail = await mutate(
        `/api/projects/${encodeURIComponent(state.selectedProject)}/tickets/${encodeURIComponent(ticketID)}`,
        { source: "web", revision: ticket.revision, fields: { status } },
        { method: "PATCH" }
      );
      state.boardNotice = "";
      ticket.status = detail.status;
      ticket.revision = detail.revision;
      if (detail.revision && detail.revision.mod_time) {
        ticket.modified = detail.revision.mod_time;
      }
      if (state.detail && state.detail.id === ticketID) {
        state.detail = detail;
        renderDetail();
      }
      renderTickets();
    } catch (err) {
      if (err.code === "stale_revision") {
        await loadTickets();
        state.boardNotice = "This ticket changed on disk, so the move was not applied. The board has been refreshed; try again.";
      } else {
        state.boardNotice = err.message || "Could not update ticket status.";
      }
      renderTickets();
    }
  }

  async function loadDetail(ticketID, options) {
    options = options || {};
    if (!state.selectedProject || !ticketID) return;
    if (options.resetHistory) {
      resetTicketHistory();
    }
    const preserveMobileView = options.preserveMobileView;
    state.selectedTicket = ticketID;
    state.editing = false;
    renderTickets();
    scrollTicketIntoView();
    if (boardModeActive() && !isMobile() && !preserveMobileView) {
      state.boardDetailOpen = true;
    }
    if (!preserveMobileView && isMobile()) {
      setMobileView("detail");
    } else {
      updateLayoutClasses();
    }
    els.detail.className = "ticket-detail";
    els.detail.innerHTML = "<p class=\"loading\">Loading ticket...</p>";
    try {
      state.detail = await api(`/api/projects/${encodeURIComponent(state.selectedProject)}/tickets/${encodeURIComponent(ticketID)}`);
      renderDetail();
    } catch (err) {
      els.detail.innerHTML = notice(escapeHTML(err.message), "error");
    }
    updateBackToolbar();
  }

  async function navigateToTicket(ticketID) {
    if (!ticketID || ticketID === state.selectedTicket) return;
    pushTicketHistory(state.selectedTicket);
    state.editMessage = "";
    await loadDetail(ticketID, { preserveMobileView: true });
  }

  function renderDetail() {
    const detail = state.detail;
    if (!detail) {
      els.detail.className = "ticket-detail empty-state";
      els.detail.textContent = "Select a ticket to view details.";
      updateLayoutClasses();
      return;
    }
    const epic = isEpic(detail);
    els.detail.className = "ticket-detail" + (epic ? " type-epic" : "");
    if (state.editing) {
      els.detail.innerHTML = editDetailView(detail);
      return;
    }
    els.detail.innerHTML = `
      <header class="detail-header${epic ? " type-epic" : ""}">
        <div class="detail-header-main">
          <div>
            <p class="muted ticket-id">${escapeHTML(detail.id)}</p>
            <div class="detail-title-row">
              ${epic ? '<span class="type-mark type-mark-epic">Epic</span>' : ""}
              <h2 class="detail-title">${escapeHTML(detail.title || "(untitled)")}</h2>
            </div>
            ${detail.parent ? parentLink(detail.parent) : ""}
          </div>
          <div class="detail-actions">
            ${epic ? `<button id="filter-children" class="button-secondary" type="button" data-parent-filter="${escapeHTML(detail.id)}">List children</button>` : ""}
            <button id="toggle-edit" class="button-primary" type="button">Edit</button>
            <button id="refresh-detail" class="button-secondary" type="button">Refresh</button>
          </div>
        </div>
        <div class="badge-row">
          ${badge(detail.status, "status-" + detail.status)}
          ${badge(detail.type, isEpic(detail) ? "type-epic" : "")}
          ${badge("p" + detail.priority)}
          ${detail.assignee ? badge(detail.assignee) : ""}
          ${epic && detail.children && detail.children.length ? badge(detail.children.length + " children", "child-count") : ""}
        </div>
      </header>
      ${state.editMessage ? notice(escapeHTML(state.editMessage), state.editMessage.indexOf("saved") >= 0 ? "" : "warning") : ""}
      ${relationshipsView(detail)}
    `;
  }

  function relationshipsView(detail) {
    const hasDeps = !!(detail.deps && detail.deps.length);
    const hasLinks = !!(detail.links && detail.links.length);
    const hasChildren = !!(detail.children && detail.children.length);
    const hasAside = hasDeps || hasLinks || hasChildren;
    const document = `
      <article class="detail-document">
        ${docSection("Description", detail.description)}
        ${docSection("Design", detail.design)}
        ${docSection("Acceptance Criteria", detail.acceptance_criteria)}
        ${otherSections(detail.other_sections)}
        ${notesTimeline(notesContent(detail))}
        ${noteSection()}
        ${commitsSection(detail.recent_commits)}
        ${addRelationshipBlock()}
      </article>
    `;
    if (!hasAside) {
      return `<div class="detail-body single-column">${document}</div>`;
    }
    const aside = `
      <aside class="detail-meta">
        ${hasDeps ? asideGroup("Dependencies", asideRelatedBody(detail.deps, "dep")) : ""}
        ${hasLinks ? asideGroup("Links", asideRelatedBody(detail.links, "link")) : ""}
        ${hasChildren ? asideGroup("Children", asideRelatedBody(detail.children, "")) : ""}
      </aside>
    `;
    return `<div class="detail-body">${document}${aside}</div>`;
  }

  function addRelationshipBlock() {
    return `
      <section class="doc-section add-relationship">
        <button id="add-relationship-toggle" class="add-relationship-toggle" type="button" aria-expanded="false" aria-controls="add-relationship-forms">
          <span class="add-relationship-plus" aria-hidden="true">+</span> Add relationship
        </button>
        <div id="add-relationship-forms" class="add-relationship-forms" hidden>
          <form id="dep-input" class="inline-form">
            <input name="target_id" placeholder="Ticket id" aria-label="Dependency ticket id">
            <button class="button-secondary" type="submit">Add dependency</button>
          </form>
          <form id="link-input" class="inline-form">
            <input name="target_id" placeholder="Ticket id" aria-label="Link ticket id">
            <button class="button-secondary" type="submit">Link ticket</button>
          </form>
        </div>
      </section>
    `;
  }

  function editDetailView(detail) {
    return `
      <header class="detail-header">
        <div class="detail-header-main">
          <div>
            <p class="muted ticket-id">${escapeHTML(detail.id)}</p>
            <h2 class="detail-title">Edit ticket</h2>
          </div>
          <div class="detail-actions">
            <button id="cancel-edit" class="button-secondary" type="button">Cancel</button>
          </div>
        </div>
      </header>
      ${state.editMessage ? notice(escapeHTML(state.editMessage), state.editMessage.indexOf("saved") >= 0 ? "" : "warning") : ""}
      <form id="edit-form" class="edit-form edit-mode">
        <label>
          <span>Title</span>
          <input name="title" value="${escapeHTML(detail.title || "")}">
        </label>
        <label>
          <span>Status</span>
          <select name="status">
            ${option("open", detail.status)}
            ${option("in_progress", detail.status)}
            ${option("needs_testing", detail.status)}
            ${option("closed", detail.status)}
          </select>
        </label>
        <label>
          <span>Type</span>
          <select name="type">
            ${option("bug", detail.type)}
            ${option("feature", detail.type)}
            ${option("task", detail.type)}
            ${option("epic", detail.type)}
            ${option("chore", detail.type)}
          </select>
        </label>
        <label>
          <span>Priority</span>
          <input name="priority" type="number" min="0" max="4" value="${Number(detail.priority || 0)}">
        </label>
        <label>
          <span>Assignee</span>
          <input name="assignee" value="${escapeHTML(detail.assignee || "")}">
        </label>
        <label>
          <span>Parent</span>
          <input name="parent" value="${escapeHTML(detail.parent || "")}">
        </label>
        <label class="full">
          <span>Description</span>
          <textarea name="description" rows="8">${escapeHTML(detail.description || "")}</textarea>
        </label>
        <label class="full">
          <span>Design</span>
          <textarea name="design" rows="8">${escapeHTML(detail.design || "")}</textarea>
        </label>
        <label class="full">
          <span>Acceptance Criteria</span>
          <textarea name="acceptance_criteria" rows="8">${escapeHTML(detail.acceptance_criteria || "")}</textarea>
        </label>
        <div class="form-actions full">
          <button class="button-primary" type="submit">Save changes</button>
        </div>
      </form>
    `;
  }

  function parentLink(parentID) {
    return `
      <button class="parent-link" type="button" data-nav-ticket="${escapeHTML(parentID)}">
        Parent: ${escapeHTML(parentID)}
      </button>
    `;
  }

  function noteSection() {
    return `
      <section class="note-form-block">
        <h3 class="section-label">Add note</h3>
        <form id="note-form" class="stack-form">
          <textarea name="text" rows="3" placeholder="Durable context (markdown supported)"></textarea>
          <div class="form-actions">
            <button class="button-primary" type="submit">Add note</button>
          </div>
        </form>
      </section>
    `;
  }

  function asideRelatedBody(items, removeKind) {
    if (!items || !items.length) return "";
    return `<div class="related-list">${items.map(item => relatedTicketItem(item, removeKind)).join("")}</div>`;
  }

  function asideCommitBody(items) {
    if (!items || !items.length) return "";
    return `<div class="commit-list">${items.map(item => `
      <div class="commit-item">
        <div class="row-title">${escapeHTML((item.sha || "").slice(0, 7))} ${escapeHTML(item.action || "")}</div>
        <div class="row-meta">${escapeHTML(item.ts || "")}</div>
        ${item.msg ? `<p class="markdown-text">${escapeHTML(item.msg)}</p>` : ""}
      </div>
    `).join("")}</div>`;
  }

  function commitsSection(items) {
    if (!items || !items.length) return "";
    return `
      <section class="doc-section">
        <h3 class="section-label">Recent commits</h3>
        ${asideCommitBody(items)}
      </section>
    `;
  }

  function option(value, selected) {
    return `<option value="${escapeHTML(value)}"${value === selected ? " selected" : ""}>${escapeHTML(value)}</option>`;
  }

  function badge(text, extraClass) {
    if (!text) return "";
    return `<span class="badge ${escapeHTML(extraClass || "")}">${escapeHTML(text)}</span>`;
  }

  function relatedTicketItem(item, removeKind) {
    const meta = [item.status, item.type, item.priority ? "p" + item.priority : ""].filter(Boolean).join(" / ");
    const removeButton = removeKind === "dep"
      ? `<button type="button" class="small-button" data-remove-dep="${escapeHTML(item.id)}">Remove</button>`
      : removeKind === "link"
        ? `<button type="button" class="small-button" data-remove-link="${escapeHTML(item.id)}">Unlink</button>`
        : "";
    const epic = isEpic(item);
    if (item.missing) {
      return `
        <div class="related-item related-item-static">
          <div class="row-title">${escapeHTML(item.id)} (missing)</div>
          <div class="related-item-foot">
            <span class="row-meta">${escapeHTML(meta)}</span>
            ${removeButton}
          </div>
        </div>
      `;
    }
    return `
      <div class="related-item${epic ? " type-epic" : ""}">
        <button class="related-link" type="button" data-nav-ticket="${escapeHTML(item.id)}">
          <span class="ticket-row-top">
            ${epic ? '<span class="type-mark type-mark-epic">Epic</span>' : ""}
            <span class="row-title">${escapeHTML(item.id)} ${escapeHTML(item.title || "")}</span>
          </span>
        </button>
        <div class="related-item-foot">
          <span class="row-meta">${escapeHTML(meta)}</span>
          ${removeButton}
        </div>
      </div>
    `;
  }

  function otherSections(items) {
    if (!items || !items.length) return "";
    return items
      .filter(item => sectionHeading(item.Heading || item.heading) !== "notes")
      .map(item => docSection(item.Heading || item.heading || "Section", item.Content || item.content || ""))
      .join("");
  }

  function notice(message, kind) {
    return `<div class="notice ${kind || ""}">${message}</div>`;
  }

  if (els.toggleSidebar) {
    els.toggleSidebar.addEventListener("click", () => {
      if (sidebarDrawerMode()) {
        setSidebarDrawerOpen(false);
      } else {
        setSidebarCollapsed(true);
      }
    });
  }
  if (els.openSidebar) {
    els.openSidebar.addEventListener("click", () => {
      setSidebarDrawerOpen(true);
    });
  }
  if (els.sidebarBackdrop) {
    els.sidebarBackdrop.addEventListener("click", () => {
      setSidebarDrawerOpen(false);
    });
  }

  if (els.ticketColumn) {
    els.ticketColumn.addEventListener("click", event => {
      const clearParent = event.target.closest("[data-clear-parent-filter]");
      if (clearParent) {
        state.filters.parent = "";
        loadTickets();
        return;
      }
      const sortButton = event.target.closest("[data-sort-field]");
      if (sortButton) {
        toggleSortField(sortButton.dataset.sortField);
        loadTickets();
        return;
      }
      const row = event.target.closest("[data-ticket]");
      if (!row) return;
      state.editMessage = "";
      loadDetail(row.dataset.ticket, { resetHistory: true });
    });

    els.ticketColumn.addEventListener("keydown", event => {
      if (event.key !== "Enter" && event.key !== " ") return;
      const row = event.target.closest("[data-ticket]");
      if (!row) return;
      event.preventDefault();
      state.editMessage = "";
      loadDetail(row.dataset.ticket, { resetHistory: true });
    });
  }

  let draggedTicketID = "";

  function clearDropTargets() {
    els.tickets.querySelectorAll(".board-column.drag-over").forEach(column => {
      column.classList.remove("drag-over");
    });
  }

  els.tickets.addEventListener("dragstart", event => {
    const card = event.target.closest(".board-card");
    if (!card) return;
    draggedTicketID = card.dataset.ticket || "";
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = "move";
      event.dataTransfer.setData("text/plain", draggedTicketID);
    }
    card.classList.add("dragging");
  });

  els.tickets.addEventListener("dragend", event => {
    const card = event.target.closest(".board-card");
    if (card) card.classList.remove("dragging");
    draggedTicketID = "";
    clearDropTargets();
  });

  els.tickets.addEventListener("dragover", event => {
    if (!draggedTicketID) return;
    const column = event.target.closest(".board-column");
    if (!column) return;
    event.preventDefault();
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = "move";
    }
    if (!column.classList.contains("drag-over")) {
      clearDropTargets();
      column.classList.add("drag-over");
    }
  });

  els.tickets.addEventListener("dragleave", event => {
    const column = event.target.closest(".board-column");
    if (!column) return;
    if (event.relatedTarget && column.contains(event.relatedTarget)) return;
    column.classList.remove("drag-over");
  });

  els.tickets.addEventListener("drop", event => {
    if (!draggedTicketID) return;
    const column = event.target.closest(".board-column");
    if (!column) return;
    event.preventDefault();
    const ticketID = draggedTicketID;
    draggedTicketID = "";
    clearDropTargets();
    moveTicketStatus(ticketID, column.dataset.status || "");
  });

  if (els.viewModeList) {
    els.viewModeList.addEventListener("click", () => setViewMode("list"));
  }
  if (els.viewModeBoard) {
    els.viewModeBoard.addEventListener("click", () => setViewMode("board"));
  }
  if (els.boardBackdrop) {
    els.boardBackdrop.addEventListener("click", () => closeBoardDetail());
  }
  document.addEventListener("keydown", event => {
    if (event.key === "Escape" && state.boardDetailOpen) {
      closeBoardDetail();
    }
  });

  els.projects.addEventListener("click", event => {
    const button = event.target.closest("[data-project]");
    if (!button) return;
    state.selectedProject = button.dataset.project;
    saveSelectedProject(state.selectedProject);
    resetTicketHistory();
    state.selectedTicket = "";
    state.detail = null;
    state.editing = false;
    state.editMessage = "";
    state.viewMode = viewModeFor(state.selectedProject);
    state.boardDetailOpen = false;
    state.boardNotice = "";
    state.dashboard = null;
    state.dashboardProject = "";
    state.dashboardError = "";
    state.expandedWeek = "";
    syncViewToggle();
    setMobileView("list");
    setSidebarCollapsed(true);
    renderProjects();
    loadTickets();
    if (state.activeView === "dashboard") {
      loadDashboard();
    }
  });

  els.detail.addEventListener("submit", async event => {
    event.preventDefault();
    if (!state.detail) return;
    const form = event.target;
    try {
      if (form.id === "edit-form") {
        await saveEdit(form);
      } else if (form.id === "note-form") {
        await addNote(form);
      } else if (form.id === "dep-input") {
        await addEdge(form, "deps");
      } else if (form.id === "link-input") {
        await addEdge(form, "links");
      }
    } catch (err) {
      state.editMessage = err.code === "stale_revision"
        ? "Stale edit: this ticket changed on disk. Refresh before saving."
        : err.message;
      renderDetail();
    }
  });

  els.detail.addEventListener("click", async event => {
    if (!state.detail) return;

    const navButton = event.target.closest("[data-nav-ticket]");
    if (navButton) {
      await navigateToTicket(navButton.dataset.navTicket);
      return;
    }

    const toggleEdit = event.target.closest("#toggle-edit");
    if (toggleEdit) {
      state.editing = true;
      state.editMessage = "";
      renderDetail();
      return;
    }

    const cancelEdit = event.target.closest("#cancel-edit");
    if (cancelEdit) {
      state.editing = false;
      state.editMessage = "";
      renderDetail();
      return;
    }

    const refresh = event.target.closest("#refresh-detail");
    if (refresh) {
      state.editMessage = "";
      state.editing = false;
      await loadDetail(state.detail.id, { preserveMobileView: true });
      return;
    }

    const filterChildren = event.target.closest("#filter-children");
    if (filterChildren) {
      applyParentFilter(filterChildren.dataset.parentFilter || state.detail.id);
      return;
    }

    const depButton = event.target.closest("[data-remove-dep]");
    if (depButton) {
      await removeEdge("deps", depButton.dataset.removeDep);
      return;
    }

    const linkButton = event.target.closest("[data-remove-link]");
    if (linkButton) {
      await removeEdge("links", linkButton.dataset.removeLink);
      return;
    }

    const addRelToggle = event.target.closest("#add-relationship-toggle");
    if (addRelToggle) {
      const forms = document.getElementById("add-relationship-forms");
      if (forms) {
        const open = forms.hasAttribute("hidden");
        forms.toggleAttribute("hidden", !open);
        addRelToggle.setAttribute("aria-expanded", String(open));
      }
    }
  });

  if (els.detailBack) {
    els.detailBack.addEventListener("click", () => {
      goBack();
    });
  }

  els.refreshProjects.addEventListener("click", () => loadSession());
  if (els.refreshHealth) {
    els.refreshHealth.addEventListener("click", () => {
      state.healthLoaded = false;
      loadHealth();
    });
  }
  if (els.refreshDashboard) {
    els.refreshDashboard.addEventListener("click", () => loadDashboard());
  }
  if (els.navTickets) {
    els.navTickets.addEventListener("click", () => setActiveView("tickets"));
  }
  if (els.navDashboard) {
    els.navDashboard.addEventListener("click", () => setActiveView("dashboard"));
  }
  if (els.navHealth) {
    els.navHealth.addEventListener("click", () => setActiveView("health"));
  }

  if (els.dashboard) {
    els.dashboard.addEventListener("click", event => {
      const weeksButton = event.target.closest("[data-weeks]");
      if (weeksButton) {
        state.timelineWeeks = Number(weeksButton.dataset.weeks) || 8;
        loadDashboard();
        return;
      }
      const weekToggle = event.target.closest("[data-week-toggle]");
      if (weekToggle) {
        const key = weekToggle.dataset.weekToggle;
        state.expandedWeek = state.expandedWeek === key ? "" : key;
        renderDashboard();
        return;
      }
      const childrenButton = event.target.closest("[data-parent-filter]");
      if (childrenButton) {
        applyParentFilter(childrenButton.dataset.parentFilter);
        return;
      }
      const navButton = event.target.closest("[data-nav-ticket]");
      if (navButton) {
        openTicketFromDashboard(navButton.dataset.navTicket);
      }
    });
  }

  els.filters.addEventListener("input", () => {
    readFiltersFromControls();
    loadTickets();
  });
  els.filters.addEventListener("change", () => {
    readFiltersFromControls();
    loadTickets();
  });
  els.filters.addEventListener("submit", event => {
    event.preventDefault();
    readFiltersFromControls();
    loadTickets();
  });

  MOBILE_QUERY.addEventListener("change", () => {
    if (!isMobile()) {
      setMobileView("list");
    } else if (state.selectedTicket) {
      setMobileView(state.mobileView);
    } else {
      setMobileView("list");
    }
    updateSidebarLayout();
  });

  initToken();
  initSidebarState();
  initPaneResize();
  setActiveView("tickets");
  loadSession();

  async function saveEdit(form) {
    const data = new FormData(form);
    const priority = Number(data.get("priority"));
    const fields = {
      title: String(data.get("title") || ""),
      status: String(data.get("status") || ""),
      type: String(data.get("type") || ""),
      priority: Number.isFinite(priority) ? priority : 2,
      assignee: String(data.get("assignee") || ""),
      parent: String(data.get("parent") || ""),
      description: String(data.get("description") || ""),
      design: String(data.get("design") || ""),
      acceptance_criteria: String(data.get("acceptance_criteria") || "")
    };
    const detail = await mutate(
      `/api/projects/${encodeURIComponent(state.selectedProject)}/tickets/${encodeURIComponent(state.detail.id)}`,
      { source: "web", revision: state.detail.revision, fields },
      { method: "PATCH" }
    );
    state.detail = detail;
    state.editing = false;
    state.editMessage = "Changes saved.";
    await loadTickets();
    renderDetail();
  }

  async function addNote(form) {
    const data = new FormData(form);
    const text = String(data.get("text") || "").trim();
    if (!text) throw new Error("Note text is required.");
    const detail = await mutate(
      `/api/projects/${encodeURIComponent(state.selectedProject)}/tickets/${encodeURIComponent(state.detail.id)}/notes`,
      { source: "web", revision: state.detail.revision, text }
    );
    state.detail = detail;
    state.editMessage = "Note saved.";
    await loadTickets();
    renderDetail();
  }

  async function addEdge(form, kind) {
    const data = new FormData(form);
    const targetID = String(data.get("target_id") || "").trim();
    if (!targetID) throw new Error("Ticket id is required.");
    const detail = await mutate(
      `/api/projects/${encodeURIComponent(state.selectedProject)}/tickets/${encodeURIComponent(state.detail.id)}/${kind}`,
      { source: "web", revision: state.detail.revision, target_id: targetID }
    );
    state.detail = detail;
    state.editMessage = kind === "deps" ? "Dependency added." : "Link added.";
    await loadTickets();
    renderDetail();
  }

  async function removeEdge(kind, targetID) {
    const params = new URLSearchParams({
      source: "web",
      revision_hash: state.detail.revision.hash,
      revision_mod_time: state.detail.revision.mod_time
    });
    const detail = await mutate(
      `/api/projects/${encodeURIComponent(state.selectedProject)}/tickets/${encodeURIComponent(state.detail.id)}/${kind}/${encodeURIComponent(targetID)}?${params}`,
      null,
      { method: "DELETE" }
    );
    state.detail = detail;
    state.editMessage = kind === "deps" ? "Dependency removed." : "Link removed.";
    await loadTickets();
    renderDetail();
  }
})();
