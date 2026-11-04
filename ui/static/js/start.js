// Get started: a copy button on every command block. No inline script (CSP).
document.addEventListener("DOMContentLoaded", () => {
  if (!navigator.clipboard) return;
  for (const pre of document.querySelectorAll("pre.cmd")) {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.className = "copy";
    btn.textContent = "Copy";
    btn.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(pre.querySelector("code").textContent);
        btn.textContent = "Copied";
      } catch {
        btn.textContent = "Press Ctrl+C";
      }
      setTimeout(() => { btn.textContent = "Copy"; }, 1500);
    });
    pre.appendChild(btn);
  }
});
