(function () {
  const state = {
    token: "",
    projects: [],
    overview: null,
    selectedProject: "",
    tickets: [],
    selectedTicket: "",
    detail: null,
    filters: {
      search: "",
      status: "",
      type: "",
      readiness: ""
    }
  };

  const els = {
    status: document.getElementById("session-status"),
    setup: document.getElementById("setup-panel"),
    projects: document.getElementById("project-list"),
    refreshProjects: document.getElementById("refresh-projects"),
    ticketHeading: document.getElementById("ticket-heading"),
    ticketCount: document.getElementById("ticket-count"),
    filters: document.getElementById("filters"),
    search: document.getElementById("search"),
    statusFilter: document.getElementById("status"),
    typeFilter: document.getElementById("type"),
    stateFilter: document.getElementById("state-filter"),
    diagnostics: document.getElementById("diagnostics"),
    tickets: document.getElementById("ticket-list"),
    detail: document.getElementById("ticket-detail")
  };

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
      throw new Error(error.message || "Request failed");
    }
    return payload;
  }

  function escapeHTML(value) {
    return String(value == null ? "" : value)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
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
      state.selectedProject = session.projects.initialized
        ? session.projects.resolved_project
        : (state.projects[0] && state.projects[0].name) || "";
      setStatus("Connected", "ok");
      renderProjects();
      renderSetup();
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
      return;
    }
    els.tickets.innerHTML = "<p class=\"loading\">Loading tickets...</p>";
    els.diagnostics.innerHTML = "";
    const params = new URLSearchParams();
    if (state.filters.search) params.set("search", state.filters.search);
    if (state.filters.status) params.set("status", state.filters.status);
    if (state.filters.type) params.set("type", state.filters.type);
    if (state.filters.readiness === "ready") params.set("ready", "true");
    if (state.filters.readiness === "blocked") params.set("blocked", "true");
    try {
      const list = await api(`/api/projects/${encodeURIComponent(state.selectedProject)}/tickets?${params}`);
      state.tickets = list.items || [];
      renderDiagnostics(list.diagnostics || []);
      renderTickets();
      if (state.selectedTicket && state.tickets.some(t => t.id === state.selectedTicket)) {
        await loadDetail(state.selectedTicket);
      } else {
        state.selectedTicket = "";
        state.detail = null;
        renderDetail();
      }
    } catch (err) {
      els.tickets.innerHTML = notice(escapeHTML(err.message), "error");
    }
  }

  function renderDiagnostics(diagnostics) {
    if (!diagnostics.length) {
      els.diagnostics.innerHTML = "";
      return;
    }
    els.diagnostics.innerHTML = diagnostics.map(diagnostic => {
      const file = diagnostic.file ? `<strong>${escapeHTML(diagnostic.file)}</strong>: ` : "";
      return notice(file + escapeHTML(diagnostic.message), "warning");
    }).join("");
  }

  function renderTickets() {
    const projectLabel = state.selectedProject || "No project";
    els.ticketHeading.textContent = projectLabel;
    els.ticketCount.textContent = `${state.tickets.length} ticket${state.tickets.length === 1 ? "" : "s"}`;
    if (!state.selectedProject) {
      els.tickets.innerHTML = "<p class=\"muted\">Choose a configured project or run `tkt init`.</p>";
      return;
    }
    if (!state.tickets.length) {
      els.tickets.innerHTML = "<p class=\"muted\">No tickets match the current filters.</p>";
      return;
    }
    els.tickets.innerHTML = state.tickets.map(ticket => {
      const selected = ticket.id === state.selectedTicket;
      const meta = [ticket.status, ticket.type, "p" + ticket.priority, ticket.assignee].filter(Boolean).join(" / ");
      return `
        <button class="ticket-button" type="button" data-ticket="${escapeHTML(ticket.id)}" aria-current="${selected}">
          <span class="row-title">${escapeHTML(ticket.id)} ${escapeHTML(ticket.title || "(untitled)")}</span>
          <span class="row-meta">${escapeHTML(meta)}</span>
        </button>
      `;
    }).join("");
  }

  async function loadDetail(ticketID) {
    if (!state.selectedProject || !ticketID) return;
    state.selectedTicket = ticketID;
    renderTickets();
    els.detail.className = "ticket-detail";
    els.detail.innerHTML = "<p class=\"loading\">Loading ticket...</p>";
    try {
      state.detail = await api(`/api/projects/${encodeURIComponent(state.selectedProject)}/tickets/${encodeURIComponent(ticketID)}`);
      renderDetail();
    } catch (err) {
      els.detail.innerHTML = notice(escapeHTML(err.message), "error");
    }
  }

  function renderDetail() {
    const detail = state.detail;
    if (!detail) {
      els.detail.className = "ticket-detail empty-state";
      els.detail.textContent = "Select a ticket to inspect its durable context.";
      return;
    }
    els.detail.className = "ticket-detail";
    els.detail.innerHTML = `
      <header class="detail-header">
        <div>
          <p class="muted">${escapeHTML(detail.id)}</p>
          <h2 class="detail-title">${escapeHTML(detail.title || "(untitled)")}</h2>
        </div>
        <div class="badge-row">
          ${badge(detail.status, "status-" + detail.status)}
          ${badge(detail.type)}
          ${badge("p" + detail.priority)}
          ${detail.assignee ? badge(detail.assignee) : ""}
        </div>
      </header>
      <div class="detail-grid">
        ${section("Description", detail.description, true)}
        ${section("Design", detail.design, true)}
        ${section("Acceptance Criteria", detail.acceptance_criteria, true)}
        ${relatedSection("Dependencies", detail.deps)}
        ${relatedSection("Links", detail.links)}
        ${childrenSection(detail.children)}
        ${commitSection(detail.recent_commits)}
        ${otherSections(detail.other_sections)}
      </div>
    `;
  }

  function badge(text, extraClass) {
    if (!text) return "";
    return `<span class="badge ${escapeHTML(extraClass || "")}">${escapeHTML(text)}</span>`;
  }

  function section(title, content, full) {
    return `
      <section class="detail-section ${full ? "full" : ""}">
        <h3>${escapeHTML(title)}</h3>
        <p class="markdown-text">${escapeHTML(content || "No content.")}</p>
      </section>
    `;
  }

  function relatedSection(title, items) {
    const body = items && items.length
      ? `<div class="related-list">${items.map(item => `
          <div class="related-item">
            <div class="row-title">${escapeHTML(item.id)} ${item.missing ? "(missing)" : escapeHTML(item.title || "")}</div>
            <div class="row-meta">${escapeHTML([item.status, item.type, item.priority ? "p" + item.priority : ""].filter(Boolean).join(" / "))}</div>
          </div>
        `).join("")}</div>`
      : "<p class=\"muted\">None.</p>";
    return `<section class="detail-section"><h3>${escapeHTML(title)}</h3>${body}</section>`;
  }

  function childrenSection(items) {
    const body = items && items.length
      ? `<div class="related-list">${items.map(item => `
          <div class="related-item">
            <div class="row-title">${escapeHTML(item.id)} ${escapeHTML(item.title || "")}</div>
            <div class="row-meta">${escapeHTML([item.status, item.type, "p" + item.priority].join(" / "))}</div>
          </div>
        `).join("")}</div>`
      : "<p class=\"muted\">None.</p>";
    return `<section class="detail-section"><h3>Children</h3>${body}</section>`;
  }

  function commitSection(items) {
    const body = items && items.length
      ? `<div class="commit-list">${items.map(item => `
          <div class="commit-item">
            <div class="row-title">${escapeHTML((item.sha || "").slice(0, 7))} ${escapeHTML(item.action || "")}</div>
            <div class="row-meta">${escapeHTML(item.ts || "")}</div>
            <p class="markdown-text">${escapeHTML(item.msg || "")}</p>
          </div>
        `).join("")}</div>`
      : "<p class=\"muted\">None.</p>";
    return `<section class="detail-section full"><h3>Recent Commits</h3>${body}</section>`;
  }

  function otherSections(items) {
    if (!items || !items.length) return "";
    return items.map(item => section(item.Heading || item.heading || "Section", item.Content || item.content || "", true)).join("");
  }

  function notice(message, kind) {
    return `<div class="notice ${kind || ""}">${message}</div>`;
  }

  els.projects.addEventListener("click", event => {
    const button = event.target.closest("[data-project]");
    if (!button) return;
    state.selectedProject = button.dataset.project;
    state.selectedTicket = "";
    state.detail = null;
    renderProjects();
    loadTickets();
  });

  els.tickets.addEventListener("click", event => {
    const button = event.target.closest("[data-ticket]");
    if (!button) return;
    loadDetail(button.dataset.ticket);
  });

  els.refreshProjects.addEventListener("click", () => loadSession());

  els.filters.addEventListener("input", () => {
    state.filters.search = els.search.value.trim();
    state.filters.status = els.statusFilter.value;
    state.filters.type = els.typeFilter.value;
    state.filters.readiness = els.stateFilter.value;
    loadTickets();
  });

  initToken();
  loadSession();
})();
