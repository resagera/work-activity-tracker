package app

const webUIHTML = `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Work Activity Tracker</title>
  <style>
    :root {
      --bg: #f2efe8;
      --card: #fffaf2;
      --ink: #1c1b18;
      --muted: #6d675f;
      --line: #d9cfbf;
      --accent: #0d7a5f;
      --accent-2: #c96c2b;
      --danger: #b42318;
      --shadow: 0 18px 50px rgba(37, 27, 8, 0.08);
    }
    [data-theme="dark"] {
      --bg: #181614;
      --card: #221f1b;
      --ink: #f3eee6;
      --muted: #b4aa9b;
      --line: #3a332d;
      --accent: #28b28c;
      --accent-2: #d58a4f;
      --danger: #e35d5d;
      --shadow: 0 18px 50px rgba(0, 0, 0, 0.28);
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: "IBM Plex Sans", "Segoe UI", sans-serif;
      color: var(--ink);
      background:
        radial-gradient(circle at top left, #fff7e8 0, transparent 32%),
        linear-gradient(135deg, #efe7d8, #f7f2e8 55%, #ebe4d5);
      min-height: 100vh;
    }
    [data-theme="dark"] body {
      background:
        radial-gradient(circle at top left, #2b251f 0, transparent 30%),
        linear-gradient(135deg, #151311, #1b1815 55%, #13110f);
    }
    .wrap {
      width: min(1100px, calc(100% - 32px));
      margin: 0 auto;
      padding: 32px 0 48px;
    }
    .hero {
      display: grid;
      gap: 14px;
      margin-bottom: 24px;
    }
    h1 {
      margin: 0;
      font-size: clamp(30px, 5vw, 54px);
      line-height: 0.95;
      letter-spacing: -0.04em;
    }
    .sub {
      color: var(--muted);
      max-width: 720px;
      font-size: 15px;
    }
    .grid {
      display: grid;
      grid-template-columns: 1.2fr 0.8fr;
      gap: 18px;
    }
    .card {
      background: color-mix(in srgb, var(--card) 92%, transparent);
      border: 1px solid var(--line);
      border-radius: 22px;
      padding: 20px;
      box-shadow: var(--shadow);
      backdrop-filter: blur(8px);
    }
    .status-line {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      flex-wrap: wrap;
      align-items: center;
      margin-bottom: 18px;
    }
    .pill {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 8px 12px;
      border-radius: 999px;
      border: 1px solid var(--line);
      background: color-mix(in srgb, var(--card) 70%, #fff 30%);
      font-size: 13px;
    }
    .dot {
      width: 9px;
      height: 9px;
      border-radius: 50%;
      background: var(--muted);
    }
    .metrics {
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 14px;
      margin-bottom: 16px;
    }
    .metric {
      padding: 16px;
      border: 1px solid var(--line);
      border-radius: 18px;
      background: linear-gradient(180deg, color-mix(in srgb, var(--card) 78%, #fff 22%), color-mix(in srgb, var(--card) 95%, #f2e8d7 5%));
    }
    .metric-label {
      font-size: 12px;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--muted);
      margin-bottom: 10px;
    }
    .metric-value {
      font-size: clamp(22px, 3vw, 34px);
      font-weight: 700;
      letter-spacing: -0.04em;
    }
    .details {
      display: grid;
      gap: 10px;
      font-size: 14px;
    }
    .details-row {
      display: grid;
      grid-template-columns: 170px 1fr;
      gap: 12px;
      padding-bottom: 10px;
      border-bottom: 1px dashed var(--line);
    }
    .details-row:last-child { border-bottom: 0; padding-bottom: 0; }
    .details-key { color: var(--muted); }
    .actions {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
      margin-bottom: 12px;
    }
    button {
      border: 0;
      border-radius: 14px;
      padding: 14px 16px;
      background: var(--ink);
      color: #fff;
      font: inherit;
      cursor: pointer;
      transition: transform .12s ease, opacity .12s ease, background .12s ease;
    }
    button:hover { transform: translateY(-1px); }
    button.secondary { background: var(--accent); }
    button.ghost { background: color-mix(in srgb, var(--card) 88%, #d9c8ad 12%); color: var(--ink); }
    button.warn { background: var(--accent-2); }
    button.danger { background: var(--danger); }
    button:disabled { opacity: .5; cursor: not-allowed; transform: none; }
    .history {
      margin-top: 10px;
      display: grid;
      gap: 10px;
      max-height: 420px;
      overflow: auto;
      padding-right: 4px;
    }
    .history-item {
      border: 1px solid var(--line);
      border-radius: 16px;
      padding: 14px;
      background: color-mix(in srgb, var(--card) 86%, #fff 14%);
    }
    .history-top {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 8px;
      font-size: 13px;
      color: var(--muted);
    }
    .history-metrics {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      font-size: 14px;
    }
    .message {
      margin-top: 10px;
      min-height: 20px;
      color: var(--muted);
      font-size: 14px;
    }
    .error { color: var(--danger); }
    @media (max-width: 900px) {
      .grid { grid-template-columns: 1fr; }
      .metrics { grid-template-columns: 1fr; }
      .details-row { grid-template-columns: 1fr; }
      .actions { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <div class="wrap">
    <section class="hero">
      <div class="status-line">
        <h1>Work Activity<br>Tracker</h1>
        <button class="ghost" id="btn-theme">Тёмная тема</button>
      </div>
      <div class="sub">Тот же HTTP сервер обслуживает API и интерфейс управления. Статус и история обновляются прямо из текущего состояния трекера.</div>
    </section>

    <div class="grid">
      <section class="card">
        <div class="status-line">
          <div class="pill"><span class="dot" id="state-dot"></span><span id="state-text">Загрузка...</span></div>
          <div class="pill">Старт дня: <strong id="started-at">-</strong></div>
        </div>

        <div class="metrics">
          <div class="metric">
            <div class="metric-label">Активность</div>
            <div class="metric-value" id="active-total">0s</div>
          </div>
          <div class="metric">
            <div class="metric-label">Неактивность</div>
            <div class="metric-value" id="inactive-total">0s</div>
          </div>
          <div class="metric">
            <div class="metric-label">Добавлено вручную</div>
            <div class="metric-value" id="added-total">0s</div>
          </div>
        </div>

        <div class="details">
          <div class="details-row"><div class="details-key">Активное окно</div><div id="window-title">-</div></div>
          <div class="details-row"><div class="details-key">GTK App ID</div><div id="window-gtk">-</div></div>
          <div class="details-row"><div class="details-key">KDE Desktop File</div><div id="window-kde">-</div></div>
          <div class="details-row"><div class="details-key">WM_CLASS</div><div id="window-class">-</div></div>
          <div class="details-row"><div class="details-key">Последнее изменение</div><div id="last-change">-</div></div>
        </div>
      </section>

      <section class="card">
        <div class="actions">
          <button class="secondary" id="btn-refresh">Обновить</button>
          <button class="secondary" id="btn-start">Старт / Возобновить</button>
          <button class="ghost" id="btn-pause">Пауза</button>
          <button class="ghost" id="btn-new-day">Начать новый день</button>
          <button class="warn" id="btn-continue-day">Продолжить день</button>
          <button class="ghost" id="btn-add-30">+30м</button>
          <button class="ghost" id="btn-add-60">+1ч</button>
          <button class="ghost" id="btn-add-120">+2ч</button>
          <button class="warn" id="btn-sub-10">-10м в неактивное</button>
          <button class="warn" id="btn-sub-20">-20м в неактивное</button>
          <button class="warn" id="btn-sub-30">-30м в неактивное</button>
          <button class="danger" id="btn-end">Завершить день</button>
        </div>
        <div class="message" id="message"></div>
      </section>
    </div>

    <section class="card" style="margin-top:18px;">
      <div class="status-line">
        <h2 style="margin:0;font-size:22px;">История</h2>
        <div class="pill">Источник: <strong>/history</strong></div>
      </div>
      <div class="history" id="history"></div>
    </section>
  </div>

  <script>
    const state = { status: null };
    const themeKey = "wat-webui-theme";

    const el = (id) => document.getElementById(id);
    const setText = (id, value) => { el(id).textContent = value ?? "-"; };

    function formatDurationFromNs(ns) {
      if (!ns || ns < 0) return "0s";
      let total = Math.round(ns / 1e9);
      const h = Math.floor(total / 3600);
      total -= h * 3600;
      const m = Math.floor(total / 60);
      total -= m * 60;
      const s = total;
      const parts = [];
      if (h > 0) parts.push(h + "h");
      if (m > 0 || h > 0) parts.push(m + "m");
      if (s > 0 || parts.length === 0) parts.push(s + "s");
      return parts.join(" ");
    }

    function stateText(s) {
      if (!s.started && s.can_continue_day) return "можно продолжить день";
      if (!s.started) return "день не начат";
      if (s.ended && s.can_continue_day) return "день завершен, можно продолжить";
      if (s.ended) return "сессия завершена";
      if (s.paused_manually) return "ручная пауза";
      if (s.locked) return "экран заблокирован";
      if (s.blocked_by_window) return "остановлено по активному окну";
      if (s.running) return "идет подсчет";
      return "ожидание активности";
    }

    function stateColor(s) {
      if (!s.started || s.ended) return "#7f8c8d";
      if (s.running) return "#0d7a5f";
      return "#c96c2b";
    }

    function renderStatus(s) {
      state.status = s;
      setText("state-text", stateText(s));
      el("state-dot").style.background = stateColor(s);
      setText("started-at", s.started ? new Date(s.session_started_at).toLocaleString() : "-");
      setText("active-total", formatDurationFromNs(s.total_active));
      setText("inactive-total", formatDurationFromNs(s.total_inactive));
      setText("added-total", formatDurationFromNs(s.total_added));
      setText("window-title", s.window?.title || "-");
      setText("window-gtk", s.window?.gtk_application_id || "-");
      setText("window-kde", s.window?.kde_net_wm_desktop_file || "-");
      setText("window-class", s.window?.wm_class || "-");
      setText("last-change", s.last_state_change ? new Date(s.last_state_change).toLocaleString() : "-");

      el("btn-pause").disabled = !s.started || s.ended;
      el("btn-end").disabled = !s.started || s.ended;
      el("btn-add-30").disabled = !s.started || s.ended;
      el("btn-add-60").disabled = !s.started || s.ended;
      el("btn-add-120").disabled = !s.started || s.ended;
      el("btn-sub-10").disabled = !s.started || s.ended;
      el("btn-sub-20").disabled = !s.started || s.ended;
      el("btn-sub-30").disabled = !s.started || s.ended;
      el("btn-continue-day").disabled = !(s.can_continue_day && (!s.started || s.ended));
    }

    function applyTheme(theme) {
      document.documentElement.setAttribute("data-theme", theme);
      localStorage.setItem(themeKey, theme);
      el("btn-theme").textContent = theme === "dark" ? "Светлая тема" : "Тёмная тема";
    }

    function toggleTheme() {
      const current = localStorage.getItem(themeKey) || "light";
      applyTheme(current === "dark" ? "light" : "dark");
    }

    function renderHistory(items) {
      const root = el("history");
      root.innerHTML = "";
      if (!items.length) {
        root.innerHTML = '<div class="history-item">История пока пуста.</div>';
        return;
      }

      items.slice().reverse().forEach((item) => {
        const node = document.createElement("div");
        node.className = "history-item";
        node.innerHTML =
          '<div class="history-top">' +
            '<span>' + new Date(item.session_started_at).toLocaleString() + '</span>' +
            '<span>' + new Date(item.session_ended_at).toLocaleString() + '</span>' +
          '</div>' +
          '<div class="history-metrics">' +
            '<strong>Активность: ' + formatDurationFromNs(item.total_active) + '</strong>' +
            '<span>Неактивность: ' + formatDurationFromNs(item.total_inactive) + '</span>' +
            '<span>Добавлено: ' + formatDurationFromNs(item.total_added) + '</span>' +
          '</div>';
        root.appendChild(node);
      });
    }

    async function api(path, options = {}) {
      const res = await fetch(path, options);
      const payload = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(payload.error || "request failed");
      }
      return payload;
    }

    async function refreshAll(showMessage = false) {
      try {
        const [status, history] = await Promise.all([
          api("/status"),
          api("/history"),
        ]);
        renderStatus(status);
        renderHistory(history);
        if (showMessage) {
          setMessage("Данные обновлены");
        }
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    function setMessage(text, isError = false) {
      const node = el("message");
      node.textContent = text;
      node.className = "message" + (isError ? " error" : "");
    }

    async function doAction(path, method = "POST") {
      try {
        await api(path, { method });
        await refreshAll();
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    el("btn-refresh").onclick = () => refreshAll(true);
    el("btn-start").onclick = () => doAction("/start");
    el("btn-pause").onclick = () => doAction("/pause");
    el("btn-new-day").onclick = () => doAction("/new-day");
    el("btn-continue-day").onclick = () => doAction("/continue-day");
    el("btn-add-30").onclick = () => doAction("/add?minutes=30", "GET");
    el("btn-add-60").onclick = () => doAction("/add?minutes=60", "GET");
    el("btn-add-120").onclick = () => doAction("/add?minutes=120", "GET");
    el("btn-sub-10").onclick = () => doAction("/subtract?minutes=10", "GET");
    el("btn-sub-20").onclick = () => doAction("/subtract?minutes=20", "GET");
    el("btn-sub-30").onclick = () => doAction("/subtract?minutes=30", "GET");
    el("btn-end").onclick = () => doAction("/end");
    el("btn-theme").onclick = toggleTheme;

    applyTheme(localStorage.getItem(themeKey) || "light");
    refreshAll();
    setInterval(refreshAll, 5000);
  </script>
</body>
</html>`
