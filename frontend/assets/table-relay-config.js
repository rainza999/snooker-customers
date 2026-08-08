(() => {
  const PANEL_ID = "ys-no-relay-panel";
  const STORAGE_PREFIX = "ys-setting-table-no-relay:";
  const settingTableRoutePattern = /^#\/setting-tables\/(?:create|\d+\/edit)$/;
  const updateRoutePattern = /\/setting-tables\/\d+\/update(?:\?|$)/;

  const state = {
    initialized: false,
    routeKey: "",
    loadingEditValue: false,
  };

  const isSettingTableForm = () => settingTableRoutePattern.test(window.location.hash);

  const isSaveRequest = (method, url) => {
    const methodName = String(method || "GET").toUpperCase();
    const target = String(url || "");
    if (!["POST", "PUT", "PATCH"].includes(methodName)) return false;
    return target.includes("/setting-tables/store") || updateRoutePattern.test(target);
  };

  const getRouteKey = () => `${STORAGE_PREFIX}${window.location.hash || "unknown"}`;

  const getEditTableId = () => {
    const match = window.location.hash.match(/^#\/setting-tables\/(\d+)\/edit$/);
    return match ? match[1] : "";
  };

  const readDisabledState = () => {
    const checkbox = document.querySelector(`#${PANEL_ID} input[type="checkbox"]`);
    if (checkbox) return checkbox.checked;
    return window.localStorage.getItem(getRouteKey()) === "1";
  };

  const writeDisabledState = (checked) => {
    window.localStorage.setItem(getRouteKey(), checked ? "1" : "0");
  };

  const getTokenHeaders = () => {
    const headers = {};
    const token = window.localStorage.getItem("token") || window.localStorage.getItem("accessToken") || window.localStorage.getItem("jwt");
    if (token) headers.Authorization = token.startsWith("Bearer ") ? token : `Bearer ${token}`;
    return headers;
  };

  const requestEditRelayState = async () => {
    const tableId = getEditTableId();
    if (!tableId || state.loadingEditValue || window.localStorage.getItem(getRouteKey()) !== null) return;
    state.loadingEditValue = true;
    try {
      const response = await window.fetch(`/api/setting-tables/${tableId}/edit`, { headers: getTokenHeaders() });
      if (!response.ok) return;
      const table = await response.json();
      const relay = Number(table.Relay ?? table.relay ?? table.relayNumber ?? table.relay_number ?? 0);
      if (relay === 0) writeDisabledState(true);
    } catch (error) {
      console.debug("Unable to load table relay state", error);
    } finally {
      state.loadingEditValue = false;
      renderPanel();
      syncRelayFields();
    }
  };

  const isInsidePanel = (element) => Boolean(element.closest(`#${PANEL_ID}`));

  const getControlText = (element) => {
    const labelByFor = element.id ? document.querySelector(`label[for="${CSS.escape(element.id)}"]`) : null;
    const wrapper = element.closest(".MuiFormControl-root, .MuiGrid-root, .MuiBox-root, div");
    return [
      element.name,
      element.id,
      element.placeholder,
      element.getAttribute("aria-label"),
      labelByFor?.textContent,
      wrapper?.textContent,
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
  };

  const isRelayControl = (element) => {
    const text = getControlText(element);
    return text.includes("relay") || text.includes("รีเลย์");
  };

  const isAddressControl = (element) => {
    const text = getControlText(element);
    return text.includes("address") || text.includes("board") || text.includes("addr") || text.includes("บอร์ด");
  };

  const setNativeValue = (element, value) => {
    const proto = element instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    const descriptor = Object.getOwnPropertyDescriptor(proto, "value");
    if (descriptor?.set) descriptor.set.call(element, value);
    else element.value = value;
    element.dispatchEvent(new Event("input", { bubbles: true }));
    element.dispatchEvent(new Event("change", { bubbles: true }));
  };

  const getRelayControls = () =>
    Array.from(document.querySelectorAll("input, textarea"))
      .filter((element) => !isInsidePanel(element))
      .filter((element) => isRelayControl(element) || isAddressControl(element))
      .map((element) => ({
        element,
        kind: isRelayControl(element) ? "relay" : "address",
      }));

  const syncRelayFields = () => {
    if (!isSettingTableForm()) return;
    const disabled = readDisabledState();
    getRelayControls().forEach(({ element, kind }) => {
      element.disabled = disabled;
      element.setAttribute("aria-disabled", disabled ? "true" : "false");
      const container = element.closest(".MuiFormControl-root, .MuiGrid-root, .MuiBox-root, div");
      if (container) {
        container.classList.toggle("ys-no-relay-disabled-field", disabled);
      }
      if (disabled) {
        setNativeValue(element, kind === "relay" ? "0" : "");
      }
    });
  };

  const transformJSONBody = (body) => {
    if (!readDisabledState()) return body;
    if (body instanceof FormData) {
      body.set("relayDisabled", "true");
      body.set("noRelay", "true");
      body.set("relayNumber", "0");
      body.set("relay", "0");
      body.set("address", "");
      return body;
    }
    if (typeof body !== "string") return body;
    const trimmed = body.trim();
    if (!trimmed || (!trimmed.startsWith("{") && !trimmed.startsWith("["))) return body;
    try {
      const payload = JSON.parse(body);
      if (payload && typeof payload === "object" && !Array.isArray(payload)) {
        payload.relayDisabled = true;
        payload.noRelay = true;
        payload.relayNumber = 0;
        payload.relay = 0;
        payload.address = "";
      }
      return JSON.stringify(payload);
    } catch (error) {
      return body;
    }
  };

  const patchNetwork = () => {
    if (state.initialized) return;
    state.initialized = true;

    const nativeFetch = window.fetch.bind(window);
    window.fetch = (input, init = {}) => {
      const url = typeof input === "string" ? input : input?.url;
      const method = init.method || (typeof input !== "string" ? input?.method : "GET");
      if (isSaveRequest(method, url) && readDisabledState() && init.body !== undefined) {
        return nativeFetch(input, { ...init, body: transformJSONBody(init.body) });
      }
      return nativeFetch(input, init);
    };

    const nativeOpen = XMLHttpRequest.prototype.open;
    const nativeSend = XMLHttpRequest.prototype.send;
    XMLHttpRequest.prototype.open = function open(method, url, ...args) {
      this.__ysNoRelayMethod = method;
      this.__ysNoRelayUrl = url;
      return nativeOpen.call(this, method, url, ...args);
    };
    XMLHttpRequest.prototype.send = function send(body) {
      if (isSaveRequest(this.__ysNoRelayMethod, this.__ysNoRelayUrl) && readDisabledState()) {
        return nativeSend.call(this, transformJSONBody(body));
      }
      return nativeSend.call(this, body);
    };
  };

  const findInsertTarget = () => {
    const firstInput = Array.from(document.querySelectorAll("input, textarea")).find((element) => !isInsidePanel(element));
    return firstInput?.closest(".MuiFormControl-root, .MuiGrid-root, .MuiBox-root, form, div") || document.querySelector("main") || document.body;
  };

  const renderPanel = () => {
    if (!isSettingTableForm()) {
      document.getElementById(PANEL_ID)?.remove();
      return;
    }

    if (state.routeKey !== getRouteKey()) {
      state.routeKey = getRouteKey();
      requestEditRelayState();
    }

    let panel = document.getElementById(PANEL_ID);
    if (!panel) {
      panel = document.createElement("section");
      panel.id = PANEL_ID;
      panel.innerHTML = `
        <label class="ys-no-relay-toggle">
          <input type="checkbox" />
          <span>
            <strong>ไม่ต่อ Relay / ไม่สั่งไฟ</strong>
            <small>ใช้กับโต๊ะอาหารหรือบิลเงินสดที่ไม่ต้องผูกหมายเลขรีเลย์และบอร์ด</small>
          </span>
        </label>
      `;
      panel.querySelector("input").addEventListener("change", (event) => {
        writeDisabledState(event.target.checked);
        syncRelayFields();
      });

      const target = findInsertTarget();
      target.parentElement?.insertBefore(panel, target);
    }

    panel.querySelector("input").checked = readDisabledState();
  };

  const installStyles = () => {
    if (document.getElementById("ys-no-relay-style")) return;
    const style = document.createElement("style");
    style.id = "ys-no-relay-style";
    style.textContent = `
      #${PANEL_ID} {
        margin: 12px 0 16px;
        padding: 14px 16px;
        border: 1px solid rgba(25, 118, 210, 0.35);
        border-radius: 12px;
        background: rgba(25, 118, 210, 0.08);
        color: #0d2540;
        box-sizing: border-box;
      }
      #${PANEL_ID} .ys-no-relay-toggle {
        display: flex;
        align-items: center;
        gap: 12px;
        cursor: pointer;
        margin: 0;
      }
      #${PANEL_ID} input[type="checkbox"] {
        width: 22px;
        height: 22px;
        flex: 0 0 auto;
        accent-color: #1976d2;
      }
      #${PANEL_ID} strong {
        display: block;
        font-size: 16px;
        line-height: 1.35;
      }
      #${PANEL_ID} small {
        display: block;
        margin-top: 3px;
        font-size: 13px;
        line-height: 1.45;
        color: rgba(13, 37, 64, 0.78);
      }
      .ys-no-relay-disabled-field {
        opacity: 0.58;
      }
    `;
    document.head.appendChild(style);
  };

  const tick = () => {
    installStyles();
    renderPanel();
    syncRelayFields();
  };

  patchNetwork();
  window.addEventListener("hashchange", tick);
  window.addEventListener("popstate", tick);
  const observer = new MutationObserver(() => window.requestAnimationFrame(tick));
  observer.observe(document.documentElement, { childList: true, subtree: true });
  window.setInterval(tick, 1200);
  tick();
})();
