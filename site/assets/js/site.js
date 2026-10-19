// The theme toggle. Without JavaScript the page follows the system setting and the button stays hidden.
(function () {
  var root = document.documentElement;
  var button = document.querySelector(".theme-toggle");
  if (!button) {
    return;
  }
  var text = button.querySelector(".theme-toggle-text");
  var media = window.matchMedia("(prefers-color-scheme: dark)");

  function current() {
    var set = root.getAttribute("data-theme");
    if (set === "light" || set === "dark") {
      return set;
    }
    return media.matches ? "dark" : "light";
  }

  function label() {
    var next = current() === "dark" ? "light" : "dark";
    text.textContent = "Switch to " + next + " theme";
  }

  button.addEventListener("click", function () {
    var next = current() === "dark" ? "light" : "dark";
    root.setAttribute("data-theme", next);
    try {
      window.localStorage.setItem("theme", next);
    } catch (e) {
      // Storage may be blocked. The choice then lasts for this page only.
    }
    label();
  });

  media.addEventListener("change", label);
  label();
  button.hidden = false;
})();

// The persona library. Without JavaScript every persona shows and the filter and copy buttons stay hidden.
(function () {
  var filter = document.querySelector(".lib-filter");
  if (!filter) {
    return;
  }
  var cards = document.querySelectorAll(".lib-card");
  var chips = filter.querySelectorAll(".lib-chip");
  var status = document.querySelector(".lib-status");

  function show(need) {
    var shown = 0;
    cards.forEach(function (card) {
      var on = need === "all" || card.getAttribute("data-need") === need;
      card.hidden = !on;
      if (on) {
        shown++;
      }
    });
    chips.forEach(function (chip) {
      chip.setAttribute("aria-pressed", chip.getAttribute("data-need") === need ? "true" : "false");
    });
    status.textContent = need === "all" ? "" : "Showing " + shown + " of " + cards.length + " personas.";
  }

  chips.forEach(function (chip) {
    chip.addEventListener("click", function () {
      show(chip.getAttribute("data-need"));
    });
  });
  filter.hidden = false;
  status.hidden = false;

  document.querySelectorAll(".lib-copy").forEach(function (button) {
    var label = button.textContent;
    var timer;
    button.addEventListener("click", function () {
      var code = document.getElementById(button.getAttribute("data-target"));
      if (!code || !navigator.clipboard) {
        return;
      }
      navigator.clipboard.writeText(code.textContent).then(function () {
        button.textContent = "Copied";
        clearTimeout(timer);
        timer = setTimeout(function () {
          button.textContent = label;
        }, 2000);
      });
    });
    button.hidden = false;
  });
})();

// A copy button on every code block. Without JavaScript the code stays as plain text.
(function () {
  if (!navigator.clipboard || !window.isSecureContext) {
    return;
  }
  var status = document.createElement("div");
  status.className = "sr-only";
  status.setAttribute("role", "status");
  document.body.appendChild(status);

  document.querySelectorAll("pre").forEach(function (pre) {
    if (pre.classList.contains("diagram-source") || pre.closest(".record")) {
      return;
    }
    var bar = pre.closest(".terminal") && pre.closest(".terminal").querySelector(".t-bar");
    var button = document.createElement("button");
    button.type = "button";
    button.className = "copy-button";
    button.textContent = "Copy";
    button.setAttribute("aria-label", "Copy code");
    button.addEventListener("click", function () {
      var sel = pre.getAttribute("data-copy");
      var text = pre.textContent;
      if (sel) {
        text = Array.prototype.map.call(pre.querySelectorAll(sel), function (n) {
          return n.textContent;
        }).join("\n");
      }
      navigator.clipboard.writeText(text.replace(/\n$/, "")).then(function () {
        button.textContent = "Copied";
        status.textContent = "Copied to the clipboard";
        window.setTimeout(function () {
          button.textContent = "Copy";
          status.textContent = "";
        }, 2000);
      }, function () {
        status.textContent = "Copy failed. Select the code and copy it.";
      });
    });
    if (bar) {
      bar.insertBefore(button, bar.querySelector(".t-pause"));
      return;
    }
    var host = pre.closest(".highlight") || pre;
    var wrap = document.createElement("div");
    wrap.className = "code-wrap";
    host.parentNode.insertBefore(wrap, host);
    wrap.appendChild(host);
    wrap.appendChild(button);
  });
})();
