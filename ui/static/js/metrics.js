// metrics.js — simple bar chart rendered on a <canvas> element
"use strict";

(function () {
  const canvas = document.getElementById("duration-chart");
  if (!canvas) return;

  const ctx = canvas.getContext("2d");
  const W = canvas.width;
  const H = canvas.height;
  const PAD = { top: 10, right: 10, bottom: 30, left: 50 };

  // Fetch recent span durations from the traces endpoint (HTMX-free fallback).
  // In a real deployment, query VictoriaMetrics /api/v1/query_range instead.
  fetch("/traces", { headers: { "Accept": "application/json" } })
    .then(function (r) { return r.ok ? r.json() : Promise.reject(r.status); })
    .then(drawBars)
    .catch(function () { drawPlaceholder(); });

  function drawBars(data) {
    const rows = Array.isArray(data) ? data : [];
    if (rows.length === 0) { drawPlaceholder(); return; }

    const values = rows.map(function (r) { return r.duration_ms || 0; });
    const maxVal = Math.max(...values, 1);

    ctx.clearRect(0, 0, W, H);
    drawAxes(maxVal);

    const plotW = W - PAD.left - PAD.right;
    const plotH = H - PAD.top - PAD.bottom;
    const barW = Math.max(2, plotW / values.length - 2);

    values.forEach(function (v, i) {
      const x = PAD.left + i * (plotW / values.length);
      const bh = (v / maxVal) * plotH;
      const y = PAD.top + plotH - bh;
      ctx.fillStyle = "#58a6ff";
      ctx.fillRect(x, y, barW, bh);
    });
  }

  function drawAxes(maxVal) {
    ctx.strokeStyle = "#30363d";
    ctx.fillStyle = "#8b949e";
    ctx.font = "11px monospace";
    ctx.lineWidth = 1;

    // Y axis
    ctx.beginPath();
    ctx.moveTo(PAD.left, PAD.top);
    ctx.lineTo(PAD.left, H - PAD.bottom);
    ctx.stroke();

    // X axis
    ctx.beginPath();
    ctx.moveTo(PAD.left, H - PAD.bottom);
    ctx.lineTo(W - PAD.right, H - PAD.bottom);
    ctx.stroke();

    // Y label
    ctx.fillText(maxVal + "ms", 2, PAD.top + 10);
    ctx.fillText("0", PAD.left - 14, H - PAD.bottom);
  }

  function drawPlaceholder() {
    ctx.fillStyle = "#8b949e";
    ctx.font = "12px monospace";
    ctx.textAlign = "center";
    ctx.fillText("No data — emit spans to see durations", W / 2, H / 2);
  }
})();
