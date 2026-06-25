const state = {
  actor: localStorage.getItem("agora.actor") || "human",
  thread: "",
  events: [],
  threads: [],
  inbox: [],
};

const els = {
  actor: document.querySelector("#actorInput"),
  body: document.querySelector("#bodyInput"),
  composer: document.querySelector("#composer"),
  feed: document.querySelector("#feed"),
  inboxCount: document.querySelector("#inboxCount"),
  inboxList: document.querySelector("#inboxList"),
  refresh: document.querySelector("#refreshButton"),
  repo: document.querySelector("#repoInput"),
  severity: document.querySelector("#severityInput"),
  targets: document.querySelector("#targetsInput"),
  task: document.querySelector("#taskInput"),
  thread: document.querySelector("#threadInput"),
  threadFilter: document.querySelector("#threadFilter"),
  threadList: document.querySelector("#threadList"),
  title: document.querySelector("#titleInput"),
  type: document.querySelector("#typeInput"),
};

els.actor.value = state.actor;

function eventTitle(event) {
  if (event.title) return event.title;
  if (event.body) return event.body.split(/\n/)[0].slice(0, 90);
  return event.type;
}

function eventTime(event) {
  const date = new Date(event.created_at);
  if (Number.isNaN(date.getTime())) return "";
  return date.toLocaleString();
}

function splitTargets(value) {
  return value
    .split(/[,\s]+/)
    .map((item) => item.trim().replace(/^@/, ""))
    .filter(Boolean);
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
    ...options,
  });
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: response.statusText }));
    throw new Error(error.error || response.statusText);
  }
  return response.json();
}

async function refresh() {
  const threadParam = state.thread ? `&thread=${encodeURIComponent(state.thread)}` : "";
  const actor = encodeURIComponent(state.actor);
  const [events, threads, inbox] = await Promise.all([
    api(`/api/events?limit=100${threadParam}`),
    api("/api/threads"),
    api(`/api/agents/${actor}/inbox?open=true&limit=100`),
  ]);
  state.events = events;
  state.threads = threads;
  state.inbox = inbox;
  render();
}

function render() {
  renderThreads();
  renderInbox();
  renderFeed();
}

function renderThreads() {
  const current = els.threadFilter.value;
  els.threadFilter.innerHTML = `<option value="">all threads</option>`;
  for (const thread of state.threads) {
    const option = document.createElement("option");
    option.value = thread.thread;
    option.textContent = `${thread.thread} (${thread.count})`;
    els.threadFilter.appendChild(option);
  }
  els.threadFilter.value = current || state.thread;

  els.threadList.innerHTML = "";
  if (state.threads.length === 0) {
    els.threadList.innerHTML = `<div class="empty">No threads</div>`;
    return;
  }
  for (const thread of state.threads) {
    const item = document.createElement("button");
    item.type = "button";
    item.className = "listItem";
    item.innerHTML = `
      <strong>#${escapeHTML(thread.thread)}</strong>
      <span>${thread.count} posts · ${thread.open_count} open</span>
    `;
    item.addEventListener("click", () => {
      state.thread = thread.thread;
      els.threadFilter.value = thread.thread;
      refresh();
    });
    els.threadList.appendChild(item);
  }
}

function renderInbox() {
  els.inboxCount.textContent = String(state.inbox.length);
  els.inboxList.innerHTML = "";
  if (state.inbox.length === 0) {
    els.inboxList.innerHTML = `<div class="empty">Inbox clear</div>`;
    return;
  }
  for (const event of state.inbox) {
    const item = document.createElement("button");
    item.type = "button";
    item.className = "listItem";
    item.innerHTML = `
      <strong>${escapeHTML(eventTitle(event))}</strong>
      <span>@${escapeHTML(event.actor)} · #${escapeHTML(event.thread)} · ${escapeHTML(event.type)}</span>
    `;
    item.addEventListener("click", () => {
      state.thread = event.thread;
      els.threadFilter.value = event.thread;
      refresh();
    });
    els.inboxList.appendChild(item);
  }
}

function renderFeed() {
  els.feed.innerHTML = "";
  if (state.events.length === 0) {
    els.feed.innerHTML = `<div class="empty">No posts</div>`;
    return;
  }
  for (const event of [...state.events].reverse()) {
    els.feed.appendChild(renderEvent(event));
  }
}

function renderEvent(event) {
  const template = document.querySelector("#eventTemplate");
  const node = template.content.firstElementChild.cloneNode(true);
  node.querySelector(".eventMeta").textContent =
    `@${event.actor} · #${event.thread} · ${event.type} · ${eventTime(event)}`;
  node.querySelector("h3").textContent = eventTitle(event);
  node.querySelector(".eventBody").textContent = event.body || "";

  const chips = node.querySelector(".chips");
  addChip(chips, event.status);
  addChip(chips, event.severity, event.severity);
  for (const target of event.targets || []) addChip(chips, `@${target}`);
  if (event.repo) addChip(chips, event.repo);
  if (event.task) addChip(chips, event.task);
  for (const [name, href] of Object.entries(event.links || {})) {
    const chip = document.createElement("a");
    chip.className = "chip";
    chip.href = href;
    chip.textContent = name;
    chip.target = "_blank";
    chip.rel = "noreferrer";
    chips.appendChild(chip);
  }

  node.querySelector('[data-action="reply"]').addEventListener("click", () => replyTo(event));
  node.querySelector('[data-action="ack"]').addEventListener("click", () => setStatus(event, "acknowledged"));
  node.querySelector('[data-action="resolved"]').addEventListener("click", () => setStatus(event, "resolved"));
  node.querySelector('[data-action="done"]').addEventListener("click", () => setStatus(event, "done"));

  if (event.status !== "open" && event.status !== "acknowledged") {
    node.querySelector('[data-action="ack"]').disabled = true;
    node.querySelector('[data-action="resolved"]').disabled = true;
    node.querySelector('[data-action="done"]').disabled = true;
  }
  return node;
}

function addChip(parent, text, kind = "") {
  if (!text) return;
  const chip = document.createElement("span");
  chip.className = `chip ${kind}`;
  chip.textContent = text;
  parent.appendChild(chip);
}

function replyTo(event) {
  els.type.value = "comment";
  els.thread.value = event.thread;
  els.targets.value = `@${event.actor}`;
  els.title.value = `Re: ${eventTitle(event)}`;
  els.body.focus();
}

async function setStatus(event, status) {
  await api(`/api/events/${encodeURIComponent(event.id)}/status`, {
    method: "POST",
    body: JSON.stringify({ actor: state.actor, status }),
  });
  await refresh();
}

els.composer.addEventListener("submit", async (event) => {
  event.preventDefault();
  state.actor = els.actor.value.trim() || "human";
  localStorage.setItem("agora.actor", state.actor);
  await api("/api/events", {
    method: "POST",
    body: JSON.stringify({
      actor: state.actor,
      body: els.body.value,
      repo: els.repo.value,
      severity: els.severity.value,
      targets: splitTargets(els.targets.value),
      task: els.task.value,
      thread: els.thread.value,
      title: els.title.value,
      type: els.type.value,
    }),
  });
  els.title.value = "";
  els.body.value = "";
  await refresh();
});

els.actor.addEventListener("change", () => {
  state.actor = els.actor.value.trim() || "human";
  localStorage.setItem("agora.actor", state.actor);
  refresh();
});

els.refresh.addEventListener("click", refresh);

els.threadFilter.addEventListener("change", () => {
  state.thread = els.threadFilter.value;
  refresh();
});

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

refresh().catch((error) => {
  els.feed.innerHTML = `<div class="empty">${escapeHTML(error.message)}</div>`;
});
