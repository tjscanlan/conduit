// Renders ```mermaid code blocks with a pinned, integrity-checked Mermaid build.
// Pages without diagrams never load it.
(function () {
  "use strict";
  var MERMAID = "https://cdn.jsdelivr.net/npm/mermaid@11.17.2/dist/mermaid.min.js";
  var SRI = "sha384-EOXBFmc3gx5mb+vn0vPvvGqACToJD24hhacX5Yx+8NUUQrHIle/Qi5Bg9o3zKwW2";

  document.addEventListener("DOMContentLoaded", function () {
    var blocks = document.querySelectorAll("code.language-mermaid");
    if (!blocks.length) return;
    blocks.forEach(function (code) {
      var div = document.createElement("div");
      div.className = "mermaid";
      div.textContent = code.textContent;
      var wrapper = code.closest("div.highlighter-rouge") || code.parentElement;
      wrapper.replaceWith(div);
    });
    var s = document.createElement("script");
    s.src = MERMAID;
    s.integrity = SRI;
    s.crossOrigin = "anonymous";
    s.onload = function () {
      window.mermaid.initialize({
        startOnLoad: false,
        securityLevel: "strict",
        theme: "base",
        themeVariables: {
          fontFamily: "ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, sans-serif",
          primaryColor: "#ffffff", primaryBorderColor: "#ec5300", primaryTextColor: "#1c1917",
          lineColor: "#57534e", secondaryColor: "#fffbeb", tertiaryColor: "#fafaf9",
          actorBkg: "#ffffff", actorBorder: "#ec5300", noteBkgColor: "#fffbeb", noteBorderColor: "#ec9b00"
        }
      });
      window.mermaid.run({ querySelector: ".mermaid" });
    };
    document.head.appendChild(s);
  });
})();
