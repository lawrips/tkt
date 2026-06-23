(function () {
  const state = {
    token: "",
    projects: [],
    overview: null,
    selectedProject: "",
    tickets: [],
    selectedTicket: "",
    detail: null,
    editMessage: "",
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
        ${editSection(detail)}
        ${noteSection()}
        ${edgeSection("Dependency ticket id", "dep-input", "Add dependency")}
        ${edgeSection("Linked ticket id", "link-input", "Add link")}
        ${relatedSection("Dependencies", detail.deps)}
        ${relatedSection("Links", detail.links)}
        ${childrenSection(detail.children)}
        ${commitSection(detail.recent_commits)}
        ${otherSections(detail.other_sections)}
      </div>
    `;
  }

  function editSection(detail) {
    return `
      <section class="detail-section full">
        <div class="section-heading-row">
          <h3>Edit Ticket</h3>
          <span class="muted">Structured fields only</span>
        </div>
        ${state.editMessage ? notice(escapeHTML(state.editMessage), state.editMessage.indexOf("saved") >= 0 ? "" : "warning") : ""}
        <form id="edit-form" class="edit-form">
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
            <textarea name="description" rows="5">${escapeHTML(detail.description || "")}</textarea>
          </label>
          <label class="full">
            <span>Design</span>
            <textarea name="design" rows="5">${escapeHTML(detail.design || "")}</textarea>
          </label>
          <label class="full">
            <span>Acceptance Criteria</span>
            <textarea name="acceptance_criteria" rows="5">${escapeHTML(detail.acceptance_criteria || "")}</textarea>
          </label>
          <div class="form-actions full">
            <button class="primary-button" type="submit">Save changes</button>
            <button type="button" id="refresh-detail">Refresh</button>
          </div>
        </form>
      </section>
    `;
  }

  function noteSection() {
    return `
      <section class="detail-section full">
        <h3>Add Note</h3>
        <form id="note-form" class="stack-form">
          <textarea name="text" rows="4" placeholder="Add durable context"></textarea>
          <div class="form-actions">
            <button type="submit">Add note</button>
          </div>
        </form>
      </section>
    `;
  }

  function edgeSection(placeholder, id, buttonText) {
    return `
      <section class="detail-section">
        <h3>${escapeHTML(buttonText)}</h3>
        <form id="${id}" class="inline-form">
          <input name="target_id" placeholder="${escapeHTML(placeholder)}">
          <button type="submit">Add</button>
        </form>
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
            ${title === "Dependencies" ? `<button type="button" class="small-button" data-remove-dep="${escapeHTML(item.id)}">Remove</button>` : ""}
            ${title === "Links" ? `<button type="button" class="small-button" data-remove-link="${escapeHTML(item.id)}">Unlink</button>` : ""}
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
    const refresh = event.target.closest("#refresh-detail");
    if (refresh) {
      state.editMessage = "";
      await loadDetail(state.detail.id);
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
    }
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
