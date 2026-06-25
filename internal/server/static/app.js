const state = {
  actor: localStorage.getItem("agora.actor") || "human",
  thread: "",
  replyTo: "",
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
  replyContext: document.querySelector("#replyContext"),
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
  renderReplyContext();
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
  const { roots, childrenByParent } = buildEventTree(state.events);
  const latestActivityByID = new Map();
  const sortedRoots = [...roots].sort((a, b) => {
    const activityDiff =
      latestActivityTime(b, childrenByParent, latestActivityByID) -
      latestActivityTime(a, childrenByParent, latestActivityByID);
    if (activityDiff !== 0) return activityDiff;
    return eventTimestamp(b) - eventTimestamp(a);
  });
  for (const event of sortedRoots) {
    els.feed.appendChild(renderEventThread(event, childrenByParent));
  }
}

function eventTimestamp(event) {
  const timestamp = Date.parse(event.created_at);
  return Number.isNaN(timestamp) ? 0 : timestamp;
}

function latestActivityTime(event, childrenByParent, latestActivityByID) {
  if (latestActivityByID.has(event.id)) {
    return latestActivityByID.get(event.id);
  }

  let latest = eventTimestamp(event);
  for (const reply of childrenByParent.get(event.id) || []) {
    latest = Math.max(latest, latestActivityTime(reply, childrenByParent, latestActivityByID));
  }
  latestActivityByID.set(event.id, latest);
  return latest;
}

function buildEventTree(events) {
  const eventsByID = new Map(events.map((event) => [event.id, event]));
  const childrenByParent = new Map();
  const roots = [];
  for (const event of events) {
    if (event.reply_to && eventsByID.has(event.reply_to)) {
      if (!childrenByParent.has(event.reply_to)) {
        childrenByParent.set(event.reply_to, []);
      }
      childrenByParent.get(event.reply_to).push(event);
      continue;
    }
    roots.push(event);
  }
  return { roots, childrenByParent };
}

function renderEventThread(event, childrenByParent) {
  const group = document.createElement("div");
  group.className = "eventThread";
  group.appendChild(renderEvent(event));

  const replies = childrenByParent.get(event.id) || [];
  if (replies.length > 0) {
    const replyList = document.createElement("div");
    replyList.className = "eventReplies";
    for (const reply of replies) {
      replyList.appendChild(renderEventThread(reply, childrenByParent));
    }
    group.appendChild(replyList);
  }
  return group;
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
  state.replyTo = event.id;
  els.type.value = "comment";
  els.thread.value = event.thread;
  els.targets.value = `@${event.actor}`;
  els.title.value = `Re: ${eventTitle(event)}`;
  renderReplyContext();
  els.body.focus();
}

function renderReplyContext() {
  els.replyContext.replaceChildren();
  if (!state.replyTo) {
    els.replyContext.hidden = true;
    return;
  }

  const parent = state.events.find((event) => event.id === state.replyTo);
  const label = document.createElement("span");
  label.textContent = parent
    ? `Replying to @${parent.actor}: ${eventTitle(parent)}`
    : `Replying to ${state.replyTo}`;

  const cancel = document.createElement("button");
  cancel.type = "button";
  cancel.className = "secondary";
  cancel.textContent = "Cancel";
  cancel.addEventListener("click", clearReply);

  els.replyContext.hidden = false;
  els.replyContext.append(label, cancel);
}

function clearReply() {
  state.replyTo = "";
  renderReplyContext();
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
  const replyParent = state.replyTo ? state.events.find((item) => item.id === state.replyTo) : null;
  await api("/api/events", {
    method: "POST",
    body: JSON.stringify({
      actor: state.actor,
      body: els.body.value,
      repo: els.repo.value,
      reply_to: state.replyTo,
      severity: els.severity.value,
      targets: splitTargets(els.targets.value),
      task: els.task.value,
      thread: replyParent ? replyParent.thread : els.thread.value,
      title: els.title.value,
      type: els.type.value,
    }),
  });
  els.title.value = "";
  els.body.value = "";
  state.replyTo = "";
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
