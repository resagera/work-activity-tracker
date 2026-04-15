    const state = { status: null, activityTypes: [], inactivityTypes: [], selectedActivityType: "", selectedInactivityType: "", titleTriggers: [], appTriggers: [] };
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

    function setTriggerList(id, items) {
      const node = el(id);
      if (!Array.isArray(items) || !items.length) {
        node.textContent = "-";
        return;
      }
      node.innerHTML = '<div class="trigger-list">' + items.map((item) => '<span class="trigger-chip">' + escapeHtml(item) + '</span>').join("") + '</div>';
    }

    function setUsageSummary(id, label, currentValue, count, isOpen) {
      const node = el(id);
      if (!count) {
        node.textContent = currentValue || "-";
        return;
      }
      node.innerHTML =
        escapeHtml(currentValue || "-") +
        ' <span class="detail-inline-toggle" data-target="' + escapeHtml(id) + '-stats">' +
        'уникальные: ' + count + ' ' + (isOpen ? '(скрыть)' : '(показать)') +
        '</span>';
    }

    function buildUsageStatsList(items, emptyText) {
      if (!Array.isArray(items) || !items.length) {
        return '<div class="period-strip-empty">' + escapeHtml(emptyText) + '</div>';
      }
      return items.map((item) => {
        const percent = Number.isFinite(item.percent) && item.percent > 0 ? ' · ' + item.percent.toFixed(1) + '%' : '';
        return '' +
          '<div class="period-item">' +
            '<div class="period-head">' +
              '<strong>' + escapeHtml(item.name) + '</strong>' +
              '<span>' + formatDurationFromNs(item.active_ns) + percent + '</span>' +
            '</div>' +
          '</div>';
      }).join("");
    }

    function bindUsageToggle(summaryId) {
      const root = el(summaryId);
      const toggle = root.querySelector(".detail-inline-toggle");
      if (!toggle) {
        return;
      }
      toggle.onclick = () => {
        const target = el(toggle.dataset.target);
        target.classList.toggle("is-hidden");
        renderStatus(state.status);
      };
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

    function formatHoursMinutesFromNs(ns) {
      if (!ns || ns < 0) return "0ч 0м";
      const totalMinutes = Math.round(ns / 1e9 / 60);
      const h = Math.floor(totalMinutes / 60);
      const m = totalMinutes - h * 60;
      return h + "ч " + m + "м";
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
        button.disabled = !state.status?.started || state.status?.ended;
        button.innerHTML =
          '<span class="type-chip-color" style="background:' + escapeHtml(item.color || "var(--line)") + ';"></span>' +
          '<span>' + escapeHtml(item.name) + '</span>';
        button.onclick = () => setCurrentActivityType(item.name);
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
        button.disabled = !(state.status?.started && !state.status?.ended);
        button.innerHTML =
          '<span class="type-chip-color" style="background:' + escapeHtml(item.color || "var(--line)") + ';"></span>' +
          '<span>' + escapeHtml(item.name) + '</span>';
        button.onclick = () => activateInactivityType(item.name);
        root.appendChild(button);
      });
    }

    function syncActivityTypeControls() {
      const selected = state.activityTypes.find((item) => item.name === state.selectedActivityType);
      const color = selected?.color || "";
      el("activity-type-color").value = color;
      el("activity-type-color-picker").value = normalizePickerColor(color, "#20256a");
      Array.from(el("activity-type-buttons").children).forEach((node, index) => {
        node.classList.toggle("is-active", state.activityTypes[index]?.name === state.selectedActivityType);
      });
    }

    function syncInactivityTypeControls() {
      const selected = state.inactivityTypes.find((item) => item.name === state.selectedInactivityType);
      const color = selected?.color || "";
      el("inactivity-type-color").value = color;
      el("inactivity-type-color-picker").value = normalizePickerColor(color, "#c96c2b");
      Array.from(el("inactivity-type-buttons").children).forEach((node, index) => {
        node.classList.toggle("is-active", state.inactivityTypes[index]?.name === state.selectedInactivityType);
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

    function historySessionName(item) {
      return item.session_name || "Сессия";
    }

    function renderStatus(s) {
      state.status = s;
      setText("session-name-title", s.session_name || "Сессия");
      el("session-name-input").value = s.session_name || "";
      setText("state-text", stateText(s));
      el("state-dot").style.background = stateColor(s);
      setText("started-at", s.started ? new Date(s.session_started_at).toLocaleString() : "-");
      setText("active-total", formatDurationFromNs(s.total_active));
      setText("inactive-total", formatDurationFromNs(s.total_inactive));
      setText("added-total", formatDurationFromNs(s.total_added));
      setTypeText("current-activity-type", s.current_activity_type, s.current_activity_color);
      setTypeText("current-inactivity-type", s.current_inactivity_type, s.current_inactivity_color);
      el("window-title-stats").innerHTML = buildUsageStatsList(s.window_stats, "Нет данных по окнам");
      el("window-class-stats").innerHTML = buildUsageStatsList(s.app_stats, "Нет данных по приложениям");
      setUsageSummary("window-title", "окна", s.window?.title || "-", s.window_count || 0, !el("window-title-stats").classList.contains("is-hidden"));
      setUsageSummary("window-class", "приложения", s.window?.wm_class || "-", s.app_count || 0, !el("window-class-stats").classList.contains("is-hidden"));
      bindUsageToggle("window-title");
      bindUsageToggle("window-class");
      setTriggerList("window-title-triggers", state.titleTriggers);
      setTriggerList("window-app-triggers", state.appTriggers);
      setText("last-change", s.last_state_change ? new Date(s.last_state_change).toLocaleString() : "-");
      el("current-periods-strip").innerHTML = buildPeriodStrip(s.periods, "Сессия пока не начата.");
      el("current-periods-list").innerHTML = buildPeriodsList(s.periods, "Нет данных по периодам");
      setText("current-periods-note", Array.isArray(s.periods) && s.periods.length ? "Периодов: " + s.periods.length : "Нет данных");
      el("current-periods-toggle").textContent = Array.isArray(s.periods) && s.periods.length ? (el("current-periods-list").classList.contains("is-hidden") ? "(показать)" : "(скрыть)") : "";

      el("btn-pause").disabled = !s.started || s.ended;
      el("btn-end").disabled = !s.started || s.ended;
      el("btn-add-10").disabled = !s.started || s.ended;
      el("btn-add-30").disabled = !s.started || s.ended;
      el("btn-add-60").disabled = !s.started || s.ended;
      el("btn-add-120").disabled = !s.started || s.ended;
      el("btn-sub-10").disabled = !s.started || s.ended;
      el("btn-sub-20").disabled = !s.started || s.ended;
      el("btn-sub-30").disabled = !s.started || s.ended;
      el("btn-sub-60").disabled = !s.started || s.ended;
      el("btn-continue-day").disabled = !(s.can_continue_day && (!s.started || s.ended));
      el("btn-set-activity-color").disabled = !state.selectedActivityType;
      el("btn-set-inactivity-color").disabled = !state.selectedInactivityType;
      el("btn-set-session-name").disabled = !s.started || s.ended;
      el("btn-edit-session-name").disabled = !s.started || s.ended;
      renderActivityTypeButtons(state.activityTypes, state.selectedActivityType);
      renderInactivityTypeButtons(state.inactivityTypes, state.selectedInactivityType);
    }

    function renderActivityTypes(payload) {
      state.activityTypes = payload.types || [];
      state.selectedActivityType = payload.current_type || state.selectedActivityType;
      if (!state.selectedActivityType && state.activityTypes.length) {
        state.selectedActivityType = state.activityTypes[0].name;
      }
      renderActivityTypeButtons(state.activityTypes, state.selectedActivityType);
      el("activity-type-color").value = payload.current_color || state.activityTypes.find((item) => item.name === state.selectedActivityType)?.color || "";
      el("activity-type-color-picker").value = normalizePickerColor(el("activity-type-color").value, "#20256a");
    }

    function renderInactivityTypes(payload) {
      state.inactivityTypes = payload.types || [];
      state.selectedInactivityType = payload.current_type || state.selectedInactivityType;
      if (!state.selectedInactivityType && state.inactivityTypes.length) {
        state.selectedInactivityType = state.inactivityTypes[0].name;
      }
      renderInactivityTypeButtons(state.inactivityTypes, state.selectedInactivityType);
      el("inactivity-type-color").value = payload.current_color || state.inactivityTypes.find((item) => item.name === state.selectedInactivityType)?.color || "";
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
        const windows = Array.isArray(item.top_windows) ? item.top_windows : [];
        const apps = Array.isArray(item.top_apps) ? item.top_apps : [];
        const periodsHtml = buildPeriodsList(periods, "Нет данных по периодам");
        const stripHtml = buildPeriodStrip(periods, "Нет данных по периодам");
        const windowsHtml = buildUsageStatsList(windows, "Нет данных по окнам");
        const appsHtml = buildUsageStatsList(apps, "Нет данных по приложениям");
        const sessionName = historySessionName(item);

        node.innerHTML =
          '<div class="history-summary">' +
            '<div class="history-top">' +
              '<div class="history-top-main">' +
                '<span class="history-session-name">' + escapeHtml(sessionName) + '</span>' +
                '<button class="ghost icon-button history-name-edit" title="Изменить имя">&#9998;</button>' +
              '</div>' +
              '<div class="history-top-times">' +
                'Активность: ' + formatHoursMinutesFromNs(item.total_active) +
                ' · Неактивность: ' + formatHoursMinutesFromNs(item.total_inactive) +
                ' · ' + new Date(item.session_started_at).toLocaleString() + ' - ' + new Date(item.session_ended_at).toLocaleString() +
              '</div>' +
            '</div>' +
            '<div class="history-edit-row is-hidden">' +
              '<input class="history-name-input" value="' + escapeHtml(sessionName) + '">' +
              '<button class="ghost history-name-save">Сохранить имя</button>' +
            '</div>' +
            '<div class="history-metrics">' +
              '<strong>Активность: ' + formatDurationFromNs(item.total_active) + '</strong>' +
              '<span>Неактивность: ' + formatDurationFromNs(item.total_inactive) + '</span>' +
              '<span>Добавлено: ' + formatDurationFromNs(item.total_added) + '</span>' +
              '<span>Периодов: ' + periods.length + (periods.length ? ' <span class="history-link history-periods-toggle">(показать)</span>' : '') + '</span>' +
              '<span>Окон: ' + (item.window_count || 0) + (windows.length ? ' <span class="history-link history-windows-toggle">(показать)</span>' : '') + '</span>' +
              '<span>Приложений: ' + (item.app_count || 0) + (apps.length ? ' <span class="history-link history-apps-toggle">(показать)</span>' : '') + '</span>' +
            '</div>' +
            '<div class="history-visual">' +
              '<div class="history-visual-label">Периоды сессии</div>' +
              stripHtml +
            '</div>' +
          '</div>' +
          (periods.length ? '<div class="history-periods is-hidden">' + periodsHtml + '</div>' : '') +
          (windows.length ? '<div class="history-stats-panel history-windows is-hidden"><div class="history-visual-label">Активные окна</div>' + windowsHtml + '</div>' : '') +
          (apps.length ? '<div class="history-stats-panel history-apps is-hidden"><div class="history-visual-label">Активные приложения</div>' + appsHtml + '</div>' : '');
        const periodsToggle = node.querySelector(".history-periods-toggle");
        const periodsBody = node.querySelector(".history-periods");
        const windowsToggle = node.querySelector(".history-windows-toggle");
        const windowsBody = node.querySelector(".history-windows");
        const appsToggle = node.querySelector(".history-apps-toggle");
        const appsBody = node.querySelector(".history-apps");
        const editButton = node.querySelector(".history-name-edit");
        const editRow = node.querySelector(".history-edit-row");
        const saveButton = node.querySelector(".history-name-save");
        const nameInput = node.querySelector(".history-name-input");
        if (periodsToggle && periodsBody) {
          periodsToggle.onclick = () => {
            periodsBody.classList.toggle("is-hidden");
            periodsToggle.textContent = periodsBody.classList.contains("is-hidden") ? "(показать)" : "(скрыть)";
          };
        }
        if (windowsToggle && windowsBody) {
          windowsToggle.onclick = () => {
            windowsBody.classList.toggle("is-hidden");
            windowsToggle.textContent = windowsBody.classList.contains("is-hidden") ? "(показать)" : "(скрыть)";
          };
        }
        if (appsToggle && appsBody) {
          appsToggle.onclick = () => {
            appsBody.classList.toggle("is-hidden");
            appsToggle.textContent = appsBody.classList.contains("is-hidden") ? "(показать)" : "(скрыть)";
          };
        }
        if (editButton && editRow && nameInput) {
          editButton.onclick = () => {
            editRow.classList.toggle("is-hidden");
            if (!editRow.classList.contains("is-hidden")) {
              nameInput.focus();
              nameInput.select();
            }
          };
        }
        if (saveButton && nameInput) {
          saveButton.onclick = async () => {
            try {
              await api("/history/session-name?started_at=" + encodeURIComponent(item.session_started_at) + "&name=" + encodeURIComponent(nameInput.value.trim()), { method: "POST" });
              await refreshAll();
            } catch (err) {
              setMessage(err.message || String(err), true);
            }
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
        const [status, history, activityTypes, inactivityTypes, windowTriggers] = await Promise.all([
          api("/status"),
          api("/history"),
          api("/activity-types"),
          api("/inactivity-types"),
          api("/window-triggers"),
        ]);
        state.titleTriggers = windowTriggers.title_triggers || [];
        state.appTriggers = windowTriggers.app_triggers || [];
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

    async function setCurrentActivityType(name = state.selectedActivityType) {
      if (!name) {
        setMessage("Выберите тип активности", true);
        return;
      }
      try {
        await api("/activity-type/set?name=" + encodeURIComponent(name), { method: "POST" });
        state.selectedActivityType = name;
        await refreshAll();
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    async function setCurrentInactivityType(name = state.selectedInactivityType) {
      if (!name) {
        setMessage("Выберите тип неактивности", true);
        return;
      }
      try {
        await api("/inactivity-type/set?name=" + encodeURIComponent(name), { method: "POST" });
        state.selectedInactivityType = name;
        await refreshAll();
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    async function activateInactivityType(name) {
      if (!name) {
        setMessage("Выберите тип неактивности", true);
        return;
      }
      try {
        if (!(state.status?.paused_manually)) {
          await api("/pause", { method: "POST" });
        }
        await api("/inactivity-type/set?name=" + encodeURIComponent(name), { method: "POST" });
        state.selectedInactivityType = name;
        await refreshAll();
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    async function setActivityTypeColor() {
      const name = state.selectedActivityType;
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
      const name = state.selectedInactivityType;
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

    async function setSessionName() {
      const name = el("session-name-input").value.trim();
      if (!name) {
        setMessage("Введите имя сессии", true);
        return;
      }
      try {
        await api("/session-name?name=" + encodeURIComponent(name), { method: "POST" });
        await refreshAll();
      } catch (err) {
        setMessage(err.message || String(err), true);
      }
    }

    function toggleSettings() {
      const panel = el("settings-panel");
      panel.classList.toggle("is-hidden");
      el("btn-settings").textContent = panel.classList.contains("is-hidden") ? "Настройки" : "Скрыть настройки";
    }

    function toggleSessionNameEdit() {
      const form = el("session-name-form");
      form.classList.toggle("is-hidden");
      if (!form.classList.contains("is-hidden")) {
        el("session-name-input").focus();
        el("session-name-input").select();
      }
    }

    el("btn-refresh").onclick = () => refreshAll(true);
    el("btn-start").onclick = () => doAction("/start");
    el("btn-pause").onclick = () => doAction("/pause");
    el("btn-new-day").onclick = () => doAction("/new-day");
    el("btn-continue-day").onclick = () => doAction("/continue-day");
    el("btn-add-10").onclick = () => doAction("/add?minutes=10", "GET");
    el("btn-add-30").onclick = () => doAction("/add?minutes=30", "GET");
    el("btn-add-60").onclick = () => doAction("/add?minutes=60", "GET");
    el("btn-add-120").onclick = () => doAction("/add?minutes=120", "GET");
    el("btn-sub-10").onclick = () => doAction("/subtract?minutes=10", "GET");
    el("btn-sub-20").onclick = () => doAction("/subtract?minutes=20", "GET");
    el("btn-sub-30").onclick = () => doAction("/subtract?minutes=30", "GET");
    el("btn-sub-60").onclick = () => doAction("/subtract?minutes=60", "GET");
    el("btn-end").onclick = () => doAction("/end");
    el("btn-theme").onclick = toggleTheme;
    el("btn-settings").onclick = toggleSettings;
    el("btn-add-activity-type").onclick = addActivityType;
    el("btn-set-activity-color").onclick = setActivityTypeColor;
    el("btn-add-inactivity-type").onclick = addInactivityType;
    el("btn-set-inactivity-color").onclick = setInactivityTypeColor;
    el("btn-edit-session-name").onclick = toggleSessionNameEdit;
    el("btn-set-session-name").onclick = setSessionName;
    el("current-periods-toggle").onclick = () => {
      const body = el("current-periods-list");
      if (!body || !body.innerHTML.trim()) {
        return;
      }
      body.classList.toggle("is-hidden");
      el("current-periods-toggle").textContent = body.classList.contains("is-hidden") ? "(показать)" : "(скрыть)";
    };
    syncColorPair("activity-type-color", "activity-type-color-picker", "#20256a");
    syncColorPair("new-activity-color", "new-activity-color-picker", "#4f46e5");
    syncColorPair("inactivity-type-color", "inactivity-type-color-picker", "#c96c2b");
    syncColorPair("new-inactivity-color", "new-inactivity-color-picker", "#ef4444");

    applyTheme(localStorage.getItem(themeKey) || "light");
    refreshAll();
    setInterval(refreshAll, 5000);
