// chart2kro site: theme toggle, copy buttons, docs search. No dependencies.
(function () {
  "use strict";

  var root = document.documentElement;

  // Theme: follows the OS until the user picks one; code highlighting follows too.
  function applyTheme(t) {
    if (t) root.dataset.theme = t; else delete root.dataset.theme;
    document.querySelectorAll("link[data-theme-css]").forEach(function (l) {
      var own = l.dataset.themeCss;
      l.media = t ? (own === t ? "all" : "not all") : "(prefers-color-scheme: " + own + ")";
    });
  }
  function currentTheme() {
    return root.dataset.theme || (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
  }
  applyTheme(root.dataset.theme);
  document.querySelectorAll("[data-theme-toggle]").forEach(function (b) {
    b.addEventListener("click", function () {
      var next = currentTheme() === "dark" ? "light" : "dark";
      applyTheme(next);
      try { localStorage.setItem("theme", next); } catch (e) {}
    });
  });

  // Docs menu: open on wide screens, collapsed on phones.
  var docnav = document.querySelector("[data-docnav]");
  if (docnav && matchMedia("(max-width: 820px)").matches) docnav.open = false;

  // Copy buttons: the install line, and every code block in the docs.
  function copy(text, btn) {
    navigator.clipboard.writeText(text).then(function () {
      var old = btn.textContent;
      btn.textContent = "Copied";
      setTimeout(function () { btn.textContent = old; }, 1500);
    });
  }
  document.querySelectorAll("[data-copy]").forEach(function (b) {
    b.addEventListener("click", function () {
      copy(b.closest("[data-copy-wrap]").querySelector("code").innerText, b);
    });
  });
  if (navigator.clipboard) {
    document.querySelectorAll(".prose pre").forEach(function (pre) {
      var b = document.createElement("button");
      b.type = "button";
      b.className = "copy copy--block";
      b.textContent = "Copy";
      b.setAttribute("aria-label", "Copy code");
      b.addEventListener("click", function () { copy(pre.innerText.replace(/\nCopy$/, ""), b); });
      pre.style.position = "relative";
      pre.appendChild(b);
    });
  }

  // Search over the index Zola generates; fetched on first open.
  var dialog = document.querySelector("[data-search]");
  if (!dialog) return;
  var input = dialog.querySelector("[data-search-input]");
  var list = dialog.querySelector("[data-search-results]");
  var script = document.querySelector("script[data-index]");
  var index = null;

  function load() {
    if (index) return Promise.resolve(index);
    return fetch(script.dataset.index).then(function (r) { return r.json(); }).then(function (d) {
      index = d;
      return index;
    });
  }
  function esc(s) {
    return s.replace(/[&<>"]/g, function (c) { return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]; });
  }
  function snippet(body, q) {
    var i = body.toLowerCase().indexOf(q);
    if (i < 0) return esc(body.slice(0, 120));
    var s = Math.max(0, i - 50);
    var part = body.slice(s, i + q.length + 70);
    var j = i - s;
    return (s > 0 ? "…" : "") + esc(part.slice(0, j)) + "<mark>" + esc(part.slice(j, j + q.length)) + "</mark>" + esc(part.slice(j + q.length)) + "…";
  }
  function search() {
    var q = input.value.trim().toLowerCase();
    if (q.length < 2) { list.innerHTML = ""; return; }
    var words = q.split(/\s+/);
    load().then(function (idx) {
      var hits = idx.map(function (e) {
        var t = (e.title || "").toLowerCase(), b = (e.body || "").toLowerCase(), d = (e.description || "").toLowerCase();
        var score = 0;
        for (var k = 0; k < words.length; k++) {
          var w = words[k];
          if (t.indexOf(w) !== -1) score += 10;
          if (d.indexOf(w) !== -1) score += 4;
          var n = b.split(w).length - 1;
          if (n === 0 && t.indexOf(w) === -1 && d.indexOf(w) === -1) return null;
          score += Math.min(n, 5);
        }
        return { e: e, score: score };
      }).filter(Boolean).sort(function (a, b) { return b.score - a.score; }).slice(0, 8);
      list.innerHTML = hits.length ? hits.map(function (h) {
        return '<li><a href="' + h.e.url + '">' + esc(h.e.title || h.e.url) + "<small>" + snippet(h.e.body || h.e.description || "", words[0]) + "</small></a></li>";
      }).join("") : '<li class="fine" style="padding:.75rem">No results</li>';
    });
  }
  function open() {
    if (!dialog.open) dialog.showModal();
    input.focus();
    load();
  }
  document.querySelectorAll("[data-search-open]").forEach(function (b) { b.addEventListener("click", open); });
  input.addEventListener("input", search);
  document.addEventListener("keydown", function (ev) {
    var typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName);
    if ((ev.key === "/" && !typing) || ((ev.metaKey || ev.ctrlKey) && ev.key.toLowerCase() === "k")) {
      ev.preventDefault();
      open();
    }
  });
  dialog.addEventListener("click", function (ev) { if (ev.target === dialog) dialog.close(); });
})();
