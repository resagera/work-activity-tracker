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
    .swatch {
      width: 12px;
      height: 12px;
      border-radius: 50%;
      display: inline-block;
      border: 1px solid color-mix(in srgb, var(--ink) 20%, transparent);
      margin-right: 8px;
      vertical-align: middle;
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
    .session-strip-card {
      display: grid;
      gap: 10px;
      margin-bottom: 16px;
      padding: 14px 16px;
      border: 1px solid var(--line);
      border-radius: 18px;
      background: linear-gradient(180deg, color-mix(in srgb, var(--card) 78%, #fff 22%), color-mix(in srgb, var(--card) 95%, #f2e8d7 5%));
    }
    .session-strip-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: center;
    }
    .session-strip-note {
      font-size: 12px;
      color: var(--muted);
    }
    .period-strip {
      display: flex;
      width: 100%;
      height: 18px;
      overflow: hidden;
      background: color-mix(in srgb, var(--card) 72%, #ddd 28%);
      box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.05);
    }
    .period-segment {
      min-width: 2px;
      height: 100%;
      flex-basis: 0;
    }
    .period-strip-empty {
      font-size: 12px;
      color: var(--muted);
    }
    .history-visual {
      margin-top: 12px;
      display: grid;
      gap: 6px;
    }
    .history-visual-label {
      font-size: 12px;
      color: var(--muted);
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
    .row {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      margin-top: 12px;
    }
    .type-buttons {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-top: 10px;
    }
    .type-chip {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 10px 12px;
      border: 1px solid var(--line);
      border-radius: 14px;
      background: color-mix(in srgb, var(--card) 86%, #fff 14%);
      color: var(--ink);
      cursor: pointer;
    }
    .type-chip.is-active {
      border-color: color-mix(in srgb, var(--accent) 58%, var(--line));
      box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent) 40%, transparent);
      background: color-mix(in srgb, var(--card) 76%, var(--accent) 24%);
    }
    .type-chip-color {
      width: 12px;
      height: 12px;
      border-radius: 4px;
      border: 1px solid color-mix(in srgb, var(--ink) 18%, transparent);
      flex: 0 0 auto;
    }
    .color-field {
      display: grid;
      grid-template-columns: minmax(0, 1fr) 56px;
      gap: 10px;
      flex: 1 1 260px;
      min-width: 0;
    }
    .color-picker {
      width: 56px;
      min-width: 56px;
      padding: 4px;
      border-radius: 14px;
      cursor: pointer;
    }
    select, input {
      width: 100%;
      border: 1px solid var(--line);
      border-radius: 14px;
      padding: 14px 16px;
      background: color-mix(in srgb, var(--card) 86%, #fff 14%);
      color: var(--ink);
      font: inherit;
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
      background: color-mix(in srgb, var(--card) 86%, #fff 14%);
      overflow: hidden;
    }
    .history-summary {
      padding: 14px;
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
    .history-link {
      color: var(--accent);
      cursor: pointer;
      text-decoration: underline;
      text-underline-offset: 2px;
    }
    .history-periods {
      border-top: 1px dashed var(--line);
      padding: 14px;
      display: grid;
      gap: 10px;
    }
    .is-hidden {
      display: none !important;
    }
    .period-item {
      border: 1px solid var(--line);
      border-radius: 12px;
      padding: 10px 12px;
      background: color-mix(in srgb, var(--card) 78%, #fff 22%);
    }
    .period-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 6px;
      font-size: 13px;
    }
    .period-meta {
      color: var(--muted);
      font-size: 13px;
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

        <div class="session-strip-card">
          <div class="session-strip-head">
            <div class="metric-label" style="margin:0;">Периоды текущей сессии <span class="history-link" id="current-periods-toggle">(показать)</span></div>
            <div class="session-strip-note" id="current-periods-note">Нет данных</div>
          </div>
          <div id="current-periods-strip" class="period-strip-empty">Сессия пока не начата.</div>
          <div id="current-periods-list" class="history-periods is-hidden"></div>
        </div>

        <div class="details">
          <div class="details-row"><div class="details-key">Тип активности</div><div id="current-activity-type">-</div></div>
          <div class="details-row"><div class="details-key">Тип неактивности</div><div id="current-inactivity-type">-</div></div>
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
          <button class="secondary" id="btn-new-day">Начать новый день</button>
          <button class="warn" id="btn-continue-day">Продолжить день</button>
          <button class="ghost" id="btn-add-30">+30м</button>
          <button class="ghost" id="btn-add-60">+1ч</button>
          <button class="ghost" id="btn-add-120">+2ч</button>
          <button class="warn" id="btn-sub-10">-10м в неактивное</button>
          <button class="warn" id="btn-sub-20">-20м в неактивное</button>
          <button class="warn" id="btn-sub-30">-30м в неактивное</button>
          <button class="danger" id="btn-end">Завершить день</button>
        </div>
        <div class="row">
          <select id="activity-type-select"></select>
          <button class="ghost" id="btn-set-activity-type">Установить тип активности</button>
        </div>
        <div class="type-buttons" id="activity-type-buttons"></div>
        <div class="row">
          <div class="color-field">
            <input id="activity-type-color" placeholder="#20256a">
            <input id="activity-type-color-picker" class="color-picker" type="color" value="#20256a">
          </div>
          <button class="ghost" id="btn-set-activity-color">Установить цвет активности</button>
        </div>
        <div class="row">
          <input id="new-activity-type" placeholder="Новый тип активности, например: проектирование">
          <div class="color-field">
            <input id="new-activity-color" placeholder="#4f46e5">
            <input id="new-activity-color-picker" class="color-picker" type="color" value="#4f46e5">
          </div>
          <button class="ghost" id="btn-add-activity-type">Добавить тип активности</button>
        </div>
        <div class="row">
          <select id="inactivity-type-select"></select>
          <button class="ghost" id="btn-set-inactivity-type">Установить тип</button>
        </div>
        <div class="type-buttons" id="inactivity-type-buttons"></div>
        <div class="row">
          <div class="color-field">
            <input id="inactivity-type-color" placeholder="#c96c2b">
            <input id="inactivity-type-color-picker" class="color-picker" type="color" value="#c96c2b">
          </div>
          <button class="ghost" id="btn-set-inactivity-color">Установить цвет неактивности</button>
        </div>
        <div class="row">
          <input id="new-inactivity-type" placeholder="Новый тип неактивности, например: перекус">
          <div class="color-field">
            <input id="new-inactivity-color" placeholder="#ef4444">
            <input id="new-inactivity-color-picker" class="color-picker" type="color" value="#ef4444">
          </div>
          <button class="ghost" id="btn-add-inactivity-type">Добавить тип</button>
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
    const state = { status: null, activityTypes: [], inactivityTypes: [] };
    const themeKey = "wat-webui-theme";

    const el = (id) => document.getElementById(id);
    const setText = (id, value) => { el(id).textContent = value ?? "-"; };
    function setTypeText(id, name, color) {
      const node = el(id);
      if (!name) {
        node.textContent = "-";
        return;
      }
      const swatch = color ? '<span class="swatch" style="background:' + color + '"></span>' : "";
      node.innerHTML = swatch + name;
    }

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

    function escapeHtml(value) {
      return String(value ?? "")
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#39;");
    }

    function normalizePickerColor(value, fallback = "#808080") {
      return /^#[0-9a-fA-F]{6}$/.test(String(value || "").trim()) ? String(value).trim() : fallback;
    }

    function syncColorPair(textId, pickerId, fallback) {
      const text = el(textId);
      const picker = el(pickerId);
      if (!text || !picker) {
        return;
      }
      const applyToPicker = () => {
        picker.value = normalizePickerColor(text.value, fallback);
      };
      const applyToText = () => {
        text.value = picker.value;
      };
      text.addEventListener("input", applyToPicker);
      picker.addEventListener("input", applyToText);
      applyToPicker();
    }

    function renderActivityTypeButtons(items, selectedName) {
      const root = el("activity-type-buttons");
      root.innerHTML = "";
      items.forEach((item) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "type-chip" + (item.name === selectedName ? " is-active" : "");
        button.innerHTML =
          '<span class="type-chip-color" style="background:' + escapeHtml(item.color || "var(--line)") + ';"></span>' +
          '<span>' + escapeHtml(item.name) + '</span>';
        button.onclick = () => {
          el("activity-type-select").value = item.name;
          syncActivityTypeControls();
        };
        root.appendChild(button);
      });
    }

    function renderInactivityTypeButtons(items, selectedName) {
      const root = el("inactivity-type-buttons");
      root.innerHTML = "";
      items.forEach((item) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "type-chip" + (item.name === selectedName ? " is-active" : "");
        button.innerHTML =
          '<span class="type-chip-color" style="background:' + escapeHtml(item.color || "var(--line)") + ';"></span>' +
          '<span>' + escapeHtml(item.name) + '</span>';
        button.onclick = () => {
          el("inactivity-type-select").value = item.name;
          syncInactivityTypeControls();
        };
        root.appendChild(button);
      });
    }

    function syncActivityTypeControls() {
      const select = el("activity-type-select");
      const selected = select.selectedOptions[0];
      const color = selected?.dataset.color || "";
      el("activity-type-color").value = color;
      el("activity-type-color-picker").value = normalizePickerColor(color, "#20256a");
      Array.from(el("activity-type-buttons").children).forEach((node, index) => {
        node.classList.toggle("is-active", state.activityTypes[index]?.name === select.value);
      });
    }

    function syncInactivityTypeControls() {
      const select = el("inactivity-type-select");
      const selected = select.selectedOptions[0];
      const color = selected?.dataset.color || "";
      el("inactivity-type-color").value = color;
      el("inactivity-type-color-picker").value = normalizePickerColor(color, "#c96c2b");
      Array.from(el("inactivity-type-buttons").children).forEach((node, index) => {
        node.classList.toggle("is-active", state.inactivityTypes[index]?.name === select.value);
      });
    }

    function periodDurationMs(period) {
      const started = new Date(period.started_at).getTime();
      const ended = new Date(period.ended_at).getTime();
      return Math.max(ended - started, 1);
    }

    function fallbackPeriodColor(period) {
      return period.kind === "activity" ? "var(--accent)" : "var(--accent-2)";
    }

    function buildPeriodStrip(periods, emptyText) {
      const items = Array.isArray(periods) ? periods.filter((period) => period?.started_at && period?.ended_at) : [];
      if (!items.length) {
        return '<div class="period-strip-empty">' + escapeHtml(emptyText) + '</div>';
      }

      const totalMs = items.reduce((sum, period) => sum + periodDurationMs(period), 0);
      const segments = items.map((period) => {
        const durationMs = periodDurationMs(period);
        const percent = totalMs > 0 ? (durationMs / totalMs) * 100 : 0;
        const color = period.color || fallbackPeriodColor(period);
        const title =
          (period.kind === "activity" ? "Активность" : "Неактивность") +
          ": " + (period.type || "-") +
          " • " + formatDurationFromNs(durationMs * 1e6) +
          " • " + percent.toFixed(1) + "%";
        return '<span class="period-segment" title="' + escapeHtml(title) + '" style="flex-grow:' + durationMs + ';background:' + escapeHtml(color) + ';"></span>';
      }).join("");

      return '<div class="period-strip">' + segments + '</div>';
    }

    function buildPeriodsList(periods, emptyText) {
      const items = Array.isArray(periods) ? periods.filter((period) => period?.started_at && period?.ended_at) : [];
      if (!items.length) {
        return '<div class="period-strip-empty">' + escapeHtml(emptyText) + '</div>';
      }

      return items.map((period) => {
        const swatch = period.color ? '<span class="swatch" style="background:' + period.color + '"></span>' : "";
        return '' +
          '<div class="period-item">' +
            '<div class="period-head">' +
              '<strong>' + swatch + escapeHtml(period.type) + '</strong>' +
              '<span>' + (period.kind === "activity" ? "Активность" : "Неактивность") + '</span>' +
            '</div>' +
            '<div class="period-meta">' +
              new Date(period.started_at).toLocaleString() + ' - ' +
              new Date(period.ended_at).toLocaleString() +
              ' · ' + formatDurationFromNs(periodDurationMs(period) * 1e6) +
            '</div>' +
          '</div>';
      }).join("");
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
      setTypeText("current-activity-type", s.current_activity_type, s.current_activity_color);
      setTypeText("current-inactivity-type", s.current_inactivity_type, s.current_inactivity_color);
      setText("window-title", s.window?.title || "-");
      setText("window-gtk", s.window?.gtk_application_id || "-");
      setText("window-kde", s.window?.kde_net_wm_desktop_file || "-");
      setText("window-class", s.window?.wm_class || "-");
      setText("last-change", s.last_state_change ? new Date(s.last_state_change).toLocaleString() : "-");
      el("current-periods-strip").innerHTML = buildPeriodStrip(s.periods, "Сессия пока не начата.");
      el("current-periods-list").innerHTML = buildPeriodsList(s.periods, "Нет данных по периодам");
      setText("current-periods-note", Array.isArray(s.periods) && s.periods.length ? "Периодов: " + s.periods.length : "Нет данных");
      el("current-periods-toggle").textContent = Array.isArray(s.periods) && s.periods.length ? (el("current-periods-list").classList.contains("is-hidden") ? "(показать)" : "(скрыть)") : "";

      el("btn-pause").disabled = !s.started || s.ended;
      el("btn-end").disabled = !s.started || s.ended;
      el("btn-add-30").disabled = !s.started || s.ended;
      el("btn-add-60").disabled = !s.started || s.ended;
      el("btn-add-120").disabled = !s.started || s.ended;
      el("btn-sub-10").disabled = !s.started || s.ended;
      el("btn-sub-20").disabled = !s.started || s.ended;
      el("btn-sub-30").disabled = !s.started || s.ended;
      el("btn-continue-day").disabled = !(s.can_continue_day && (!s.started || s.ended));
      el("btn-set-activity-type").disabled = !s.started || s.ended;
      el("btn-set-activity-color").disabled = !el("activity-type-select").value;
      el("btn-set-inactivity-type").disabled = !(s.started && !s.ended && s.paused_manually);
      el("btn-set-inactivity-color").disabled = !el("inactivity-type-select").value;
    }

    function renderActivityTypes(payload) {
      state.activityTypes = payload.types || [];
      const select = el("activity-type-select");
      select.innerHTML = "";
      state.activityTypes.forEach((item) => {
        const option = document.createElement("option");
        option.value = item.name;
        option.textContent = item.color ? item.name + " [" + item.color + "]" : item.name;
        option.dataset.color = item.color || "";
        if (item.name === payload.current_type) {
          option.selected = true;
        }
        select.appendChild(option);
      });
      if (!select.value && state.activityTypes.length) {
        select.value = state.activityTypes[0].name;
      }
      if (payload.current_type) {
        select.value = payload.current_type;
      }
      renderActivityTypeButtons(state.activityTypes, select.value);
      el("activity-type-color").value = payload.current_color || select.selectedOptions[0]?.dataset.color || "";
      el("activity-type-color-picker").value = normalizePickerColor(el("activity-type-color").value, "#20256a");
    }

    function renderInactivityTypes(payload) {
      state.inactivityTypes = payload.types || [];
      const select = el("inactivity-type-select");
      select.innerHTML = "";
      state.inactivityTypes.forEach((item) => {
        const option = document.createElement("option");
        option.value = item.name;
        option.textContent = item.color ? item.name + " [" + item.color + "]" : item.name;
        option.dataset.color = item.color || "";
        if (item.name === payload.current_type) {
          option.selected = true;
        }
        select.appendChild(option);
      });
      if (!select.value && state.inactivityTypes.length) {
        select.value = state.inactivityTypes[0].name;
      }
      if (payload.current_type) {
        select.value = payload.current_type;
      }
      renderInactivityTypeButtons(state.inactivityTypes, select.value);
      el("inactivity-type-color").value = payload.current_color || select.selectedOptions[0]?.dataset.color || "";
      el("inactivity-type-color-picker").value = normalizePickerColor(el("inactivity-type-color").value, "#c96c2b");
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
        const periods = Array.isArray(item.periods) ? item.periods : [];
        const periodsHtml = buildPeriodsList(periods, "Нет данных по периодам");
        const stripHtml = buildPeriodStrip(periods, "Нет данных по периодам");

        node.innerHTML =
          '<div class="history-summary">' +
            '<div class="history-top">' +
              '<span>' + new Date(item.session_started_at).toLocaleString() + '</span>' +
              '<span>' + new Date(item.session_ended_at).toLocaleString() + '</span>' +
            '</div>' +
            '<div class="history-metrics">' +
              '<strong>Активность: ' + formatDurationFromNs(item.total_active) + '</strong>' +
              '<span>Неактивность: ' + formatDurationFromNs(item.total_inactive) + '</span>' +
              '<span>Добавлено: ' + formatDurationFromNs(item.total_added) + '</span>' +
              '<span>Периодов: ' + periods.length + (periods.length ? ' <span class="history-link">(показать)</span>' : '') + '</span>' +
            '</div>' +
            '<div class="history-visual">' +
              '<div class="history-visual-label">Периоды сессии</div>' +
              stripHtml +
            '</div>' +
          '</div>' +
          (periods.length ? '<div class="history-periods is-hidden">' + periodsHtml + '</div>' : '');
        const toggle = node.querySelector(".history-link");
        const body = node.querySelector(".history-periods");
        if (toggle && body) {
          toggle.onclick = () => {
            body.classList.toggle("is-hidden");
            toggle.textContent = body.classList.contains("is-hidden") ? "(показать)" : "(скрыть)";
          };
        }
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
        const [status, history, activityTypes, inactivityTypes] = await Promise.all([
          api("/status"),
          api("/history"),
          api("/activity-types"),
          api("/inactivity-types"),
        ]);
        renderStatus(status);
        renderHistory(history);
        renderActivityTypes(activityTypes);
        renderInactivityTypes(inactivityTypes);
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

    async function addInactivityType() {
      const input = el("new-inactivity-type");
      const name = input.value.trim();
      const color = el("new-inactivity-color").value.trim();
      if (!name) {
        setMessage("Введите название типа", true);
        return;
      }
      try {
        await api("/inactivity-types/add?name=" + encodeURIComponent(name) + "&color=" + encodeURIComponent(color), { method: "POST" });
        input.value = "";
        el("new-inactivity-color").value = "";
        await refreshAll();
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    async function addActivityType() {
      const input = el("new-activity-type");
      const name = input.value.trim();
      const color = el("new-activity-color").value.trim();
      if (!name) {
        setMessage("Введите название типа активности", true);
        return;
      }
      try {
        await api("/activity-types/add?name=" + encodeURIComponent(name) + "&color=" + encodeURIComponent(color), { method: "POST" });
        input.value = "";
        el("new-activity-color").value = "";
        await refreshAll();
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    async function setCurrentActivityType() {
      const select = el("activity-type-select");
      const name = select.value;
      if (!name) {
        setMessage("Выберите тип активности", true);
        return;
      }
      try {
        await api("/activity-type/set?name=" + encodeURIComponent(name), { method: "POST" });
        await refreshAll();
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    async function setCurrentInactivityType() {
      const select = el("inactivity-type-select");
      const name = select.value;
      if (!name) {
        setMessage("Выберите тип неактивности", true);
        return;
      }
      try {
        await api("/inactivity-type/set?name=" + encodeURIComponent(name), { method: "POST" });
        await refreshAll();
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    async function setActivityTypeColor() {
      const name = el("activity-type-select").value;
      const color = el("activity-type-color").value.trim();
      if (!name || !color) {
        setMessage("Выберите тип активности и цвет", true);
        return;
      }
      try {
        await api("/activity-type/color?name=" + encodeURIComponent(name) + "&color=" + encodeURIComponent(color), { method: "POST" });
        await refreshAll();
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    async function setInactivityTypeColor() {
      const name = el("inactivity-type-select").value;
      const color = el("inactivity-type-color").value.trim();
      if (!name || !color) {
        setMessage("Выберите тип неактивности и цвет", true);
        return;
      }
      try {
        await api("/inactivity-type/color?name=" + encodeURIComponent(name) + "&color=" + encodeURIComponent(color), { method: "POST" });
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
    el("btn-add-activity-type").onclick = addActivityType;
    el("btn-set-activity-type").onclick = setCurrentActivityType;
    el("btn-set-activity-color").onclick = setActivityTypeColor;
    el("btn-add-inactivity-type").onclick = addInactivityType;
    el("btn-set-inactivity-type").onclick = setCurrentInactivityType;
    el("btn-set-inactivity-color").onclick = setInactivityTypeColor;
    el("current-periods-toggle").onclick = () => {
      const body = el("current-periods-list");
      if (!body || !body.innerHTML.trim()) {
        return;
      }
      body.classList.toggle("is-hidden");
      el("current-periods-toggle").textContent = body.classList.contains("is-hidden") ? "(показать)" : "(скрыть)";
    };
    el("activity-type-select").onchange = syncActivityTypeControls;
    el("inactivity-type-select").onchange = syncInactivityTypeControls;

    syncColorPair("activity-type-color", "activity-type-color-picker", "#20256a");
    syncColorPair("new-activity-color", "new-activity-color-picker", "#4f46e5");
    syncColorPair("inactivity-type-color", "inactivity-type-color-picker", "#c96c2b");
    syncColorPair("new-inactivity-color", "new-inactivity-color-picker", "#ef4444");

    applyTheme(localStorage.getItem(themeKey) || "light");
    refreshAll();
    setInterval(refreshAll, 5000);
  </script>
</body>
</html>`
