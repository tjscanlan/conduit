// traces.js: trace list navigation and span deep links. Kept to an external
// file because the CSP forbids inline script.
"use strict";

// Trace list: clicking a row opens that trace.
document.addEventListener("click", function (e) {
  const row = e.target.closest(".span-row[data-trace-id]");
  if (!row || e.target.closest("a")) return;
  const traceID = row.dataset.traceId;
  if (traceID) window.location.href = "/traces/" + encodeURIComponent(traceID);
});

// Trace detail: /traces/<id>#span-<spanID> opens and highlights that span.
function openLinkedSpan() {
  if (!window.location.hash.startsWith("#span-")) return;
  const node = document.getElementById(decodeURIComponent(window.location.hash.slice(1)));
  if (!node || !node.classList.contains("span-node")) return;
  document.querySelectorAll(".span-node.linked").forEach(function (n) { n.classList.remove("linked"); });
  node.classList.add("linked");
  const details = node.querySelector("details");
  if (details) details.open = true;
  node.scrollIntoView({ block: "center" });
}
document.addEventListener("DOMContentLoaded", openLinkedSpan);
window.addEventListener("hashchange", openLinkedSpan);
