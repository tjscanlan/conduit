// traces.js — expand/collapse LLM payload for a span row
"use strict";

document.addEventListener("click", function (e) {
  const row = e.target.closest(".span-row[data-trace-id]");
  if (!row) return;

  // If the click was on a link, let it navigate normally.
  if (e.target.tagName === "A") return;

  const traceID = row.dataset.traceId;
  if (!traceID) return;

  window.location.href = "/traces/" + traceID;
});

// Highlight the row that matches the current URL trace ID (detail page).
(function highlightCurrent() {
  const m = window.location.pathname.match(/\/traces\/(.+)/);
  if (!m) return;
  const traceID = m[1];
  document.querySelectorAll(`.span-row[data-trace-id="${CSS.escape(traceID)}"]`).forEach(function (r) {
    r.style.background = "var(--surface)";
  });
})();
