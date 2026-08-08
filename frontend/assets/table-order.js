(() => {
  const API_BASE = "/api";
  const SELECTORS = {
    posCards: ".gridForm > .card",
    settingRows: "tbody tr",
  };

  let tables = [];
  let tableByName = new Map();
  let tableById = new Map();
  let applying = false;
  let saveTimer = null;
  let touchDrag = null;

  const isRoute = (part) => window.location.hash.includes(part);
  const isActiveRoute = () => isRoute("/point-of-sales") || isRoute("/setting-tables");

  const getToken = () => localStorage.getItem("token") || "";

  const getHeaders = () => ({
    "Content-Type": "application/json",
    Authorization: `Bearer ${getToken()}`,
  });

  const showToast = (message, type = "info") => {
    let toast = document.querySelector(".ysm-table-order-toast");
    if (!toast) {
      toast = document.createElement("div");
      toast.className = "ysm-table-order-toast";
      document.body.appendChild(toast);
    }
    toast.textContent = message;
    toast.dataset.type = type;
    toast.classList.add("show");
    window.clearTimeout(toast._timer);
    toast._timer = window.setTimeout(() => toast.classList.remove("show"), 2200);
  };

  const normalizeName = (value) => String(value || "").replace(/\s+/g, "").trim();

  const readTableName = (element) => {
    const candidates = Array.from(element.querySelectorAll("a, th, td, p, h1, h2, h3, h4, h5, h6, span, div"));
    for (const candidate of candidates) {
      const text = (candidate.textContent || "").trim();
      if (!text || text.length > 40) continue;
      const key = normalizeName(text);
      if (tableByName.has(key)) return tableByName.get(key);
    }
    return null;
  };

  const elementTableID = (element, index) => {
    const existing = Number(element.dataset.ysmTableId || 0);
    if (existing) return existing;

    const table = readTableName(element) || tables[index];
    if (!table) return 0;

    const id = Number(table.id || table.ID || 0);
    if (!id) return 0;

    element.dataset.ysmTableId = String(id);
    return id;
  };

  const orderedPayload = (elements) =>
    elements
      .map((element, index) => ({ id: elementTableID(element, index), sort_order: index + 1 }))
      .filter((item) => item.id > 0);

  const saveOrder = (elements) => {
    window.clearTimeout(saveTimer);
    saveTimer = window.setTimeout(async () => {
      const items = orderedPayload(elements);
      if (!items.length) return;

      try {
        const response = await fetch(`${API_BASE}/setting-tables/reorder`, {
          method: "PUT",
          headers: getHeaders(),
          body: JSON.stringify({ items }),
        });

        if (!response.ok) {
          const data = await response.json().catch(() => ({}));
          throw new Error(data.error || `HTTP ${response.status}`);
        }

        items.forEach((item) => {
          const table = tableById.get(item.id);
          if (table) table.sort_order = item.sort_order;
        });
        tables.sort((a, b) => (a.sort_order || a.SortOrder || a.ID || a.id) - (b.sort_order || b.SortOrder || b.ID || b.id));
        showToast("บันทึกลำดับโต๊ะแล้ว", "success");
      } catch (error) {
        console.error("Failed to save table order:", error);
        showToast(`บันทึกลำดับไม่สำเร็จ: ${error.message}`, "error");
      }
    }, 180);
  };

  const reorderElement = (dragged, target, container, placeAfter) => {
    if (!dragged || !target || dragged === target || !container) return false;
    if (placeAfter) {
      container.insertBefore(dragged, target.nextSibling);
    } else {
      container.insertBefore(dragged, target);
    }
    return true;
  };

  const isInteractive = (target) =>
    Boolean(target.closest("button, a, input, textarea, select, [role='button'], .MuiDialog-root, .MuiModal-root"));

  const bindNativeDrag = (element, elements, container) => {
    element.draggable = true;

    element.addEventListener("dragstart", (event) => {
      if (isInteractive(event.target)) {
        event.preventDefault();
        return;
      }
      event.dataTransfer.effectAllowed = "move";
      event.dataTransfer.setData("text/plain", element.dataset.ysmTableId || "");
      element.classList.add("ysm-dragging");
    });

    element.addEventListener("dragend", () => {
      element.classList.remove("ysm-dragging");
      document.querySelectorAll(".ysm-drop-target").forEach((item) => item.classList.remove("ysm-drop-target"));
    });

    element.addEventListener("dragover", (event) => {
      event.preventDefault();
      event.dataTransfer.dropEffect = "move";
      element.classList.add("ysm-drop-target");
    });

    element.addEventListener("dragleave", () => element.classList.remove("ysm-drop-target"));

    element.addEventListener("drop", (event) => {
      event.preventDefault();
      element.classList.remove("ysm-drop-target");
      const dragged = container.querySelector(".ysm-dragging");
      if (!dragged) return;
      const rect = element.getBoundingClientRect();
      const placeAfter = event.clientY > rect.top + rect.height / 2 || event.clientX > rect.left + rect.width / 2;
      if (reorderElement(dragged, element, container, placeAfter)) {
        saveOrder(Array.from(container.children).filter((child) => child.dataset.ysmOrderBound === "1"));
      }
    });
  };

  const bindPointerDrag = (element, container) => {
    element.addEventListener("pointerdown", (event) => {
      if (event.button !== 0 || isInteractive(event.target)) return;

      const startX = event.clientX;
      const startY = event.clientY;
      const timer = window.setTimeout(() => {
        touchDrag = { element, container, target: null, startX, startY };
        element.classList.add("ysm-dragging");
        element.setPointerCapture?.(event.pointerId);
        showToast("ลากไปวางตำแหน่งใหม่", "info");
      }, event.pointerType === "mouse" ? 180 : 420);

      const cancel = () => {
        window.clearTimeout(timer);
        element.removeEventListener("pointermove", moveBeforeStart);
        element.removeEventListener("pointerup", cancel);
        element.removeEventListener("pointercancel", cancel);
      };

      const moveBeforeStart = (moveEvent) => {
        if (Math.abs(moveEvent.clientX - startX) > 8 || Math.abs(moveEvent.clientY - startY) > 8) {
          cancel();
        }
      };

      element.addEventListener("pointermove", moveBeforeStart);
      element.addEventListener("pointerup", cancel, { once: true });
      element.addEventListener("pointercancel", cancel, { once: true });
    });

    element.addEventListener("pointermove", (event) => {
      if (!touchDrag || touchDrag.element !== element) return;
      const target = document.elementFromPoint(event.clientX, event.clientY)?.closest("[data-ysm-order-bound='1']");
      document.querySelectorAll(".ysm-drop-target").forEach((item) => item.classList.remove("ysm-drop-target"));
      if (target && target !== element) {
        target.classList.add("ysm-drop-target");
        touchDrag.target = target;
      }
    });

    element.addEventListener("pointerup", (event) => {
      if (!touchDrag || touchDrag.element !== element) return;
      const target = touchDrag.target;
      document.querySelectorAll(".ysm-drop-target").forEach((item) => item.classList.remove("ysm-drop-target"));
      element.classList.remove("ysm-dragging");
      touchDrag = null;

      if (!target) return;
      const rect = target.getBoundingClientRect();
      const placeAfter = event.clientY > rect.top + rect.height / 2 || event.clientX > rect.left + rect.width / 2;
      if (reorderElement(element, target, container, placeAfter)) {
        saveOrder(Array.from(container.children).filter((child) => child.dataset.ysmOrderBound === "1"));
      }
    });
  };

  const bindElements = (elements, container, routeClass) => {
    elements.forEach((element, index) => {
      if (element.dataset.ysmOrderBound === "1") return;

      const id = elementTableID(element, index);
      if (!id) return;

      element.dataset.ysmOrderBound = "1";
      element.classList.add("ysm-orderable", routeClass);
      element.title = "กดค้างแล้วลากเพื่อสลับลำดับโต๊ะ";
      bindNativeDrag(element, elements, container);
      bindPointerDrag(element, container);
    });
  };

  const applyOrder = () => {
    if (applying || !isActiveRoute() || !tables.length) return;
    applying = true;
    try {
      if (isRoute("/point-of-sales")) {
        const container = document.querySelector(".gridForm");
        const elements = Array.from(document.querySelectorAll(SELECTORS.posCards));
        if (container && elements.length) bindElements(elements, container, "ysm-pos-card");
      }

      if (isRoute("/setting-tables")) {
        const body = document.querySelector("tbody");
        const rows = Array.from(document.querySelectorAll(SELECTORS.settingRows));
        if (body && rows.length) bindElements(rows, body, "ysm-setting-row");
      }
    } finally {
      applying = false;
    }
  };

  const loadTables = async () => {
    if (!isActiveRoute()) return;
    try {
      const response = await fetch(`${API_BASE}/setting-tables/anyData`, { headers: getHeaders() });
      if (!response.ok) return;
      const data = await response.json();
      tables = Array.isArray(data) ? data : [];
      tables.sort((a, b) => (a.sort_order || a.SortOrder || a.ID || a.id) - (b.sort_order || b.SortOrder || b.ID || b.id));
      tableByName = new Map(tables.map((table) => [normalizeName(table.Name || table.name), table]));
      tableById = new Map(tables.map((table) => [Number(table.id || table.ID), table]));
      window.setTimeout(applyOrder, 80);
    } catch (error) {
      console.error("Failed to load table order:", error);
    }
  };

  const installStyles = () => {
    if (document.getElementById("ysm-table-order-style")) return;
    const style = document.createElement("style");
    style.id = "ysm-table-order-style";
    style.textContent = `
      .ysm-orderable {
        cursor: grab;
        touch-action: manipulation;
      }
      .ysm-orderable:active {
        cursor: grabbing;
      }
      .ysm-dragging {
        opacity: 0.72;
        outline: 3px solid rgba(21, 152, 251, 0.9);
        outline-offset: 4px;
        transform: scale(0.99);
      }
      .ysm-drop-target {
        outline: 3px dashed rgba(76, 175, 80, 0.9);
        outline-offset: 6px;
      }
      .ysm-table-order-toast {
        position: fixed;
        left: 50%;
        bottom: 24px;
        z-index: 99999;
        transform: translate(-50%, 20px);
        opacity: 0;
        pointer-events: none;
        padding: 10px 16px;
        border-radius: 999px;
        color: #fff;
        background: rgba(15, 23, 42, 0.94);
        box-shadow: 0 12px 28px rgba(0, 0, 0, 0.24);
        font-family: Prompt, sans-serif;
        font-size: 14px;
        font-weight: 600;
        transition: opacity 160ms ease, transform 160ms ease;
      }
      .ysm-table-order-toast.show {
        opacity: 1;
        transform: translate(-50%, 0);
      }
      .ysm-table-order-toast[data-type="success"] {
        background: rgba(27, 120, 55, 0.96);
      }
      .ysm-table-order-toast[data-type="error"] {
        background: rgba(185, 28, 28, 0.96);
      }
    `;
    document.head.appendChild(style);
  };

  const boot = () => {
    installStyles();
    loadTables();
    const observer = new MutationObserver(() => {
      if (!isActiveRoute()) return;
      window.clearTimeout(observer._timer);
      observer._timer = window.setTimeout(applyOrder, 120);
    });
    observer.observe(document.body, { childList: true, subtree: true });
    window.addEventListener("hashchange", () => window.setTimeout(loadTables, 160));
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }
})();
