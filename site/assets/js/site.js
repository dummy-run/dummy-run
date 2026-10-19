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

// Watch a run. Without JavaScript every step shows as a list. With it, one step shows at a time.
(function () {
  var bar = document.querySelector(".rp-bar");
  if (!bar) {
    return;
  }
  var steps = Array.prototype.slice.call(document.querySelectorAll(".rp-step"));
  var dots = Array.prototype.slice.call(document.querySelectorAll(".rp-dot"));
  var index = document.querySelector(".rp-index");
  var pos = bar.querySelector(".rp-pos");
  var prev = bar.querySelector("[data-rp=prev]");
  var next = bar.querySelector("[data-rp=next]");
  var current = 1;

  function show(n, move) {
    n = Math.max(1, Math.min(steps.length, n));
    current = n;
    steps.forEach(function (li, i) {
      li.hidden = i + 1 !== n;
    });
    dots.forEach(function (d, i) {
      if (i + 1 === n) {
        d.setAttribute("aria-current", "step");
      } else {
        d.removeAttribute("aria-current");
      }
    });
    pos.textContent = "Step " + n + " of " + steps.length;
    prev.disabled = n === 1;
    next.disabled = n === steps.length;
    if (move) {
      var h = steps[n - 1].querySelector(".rp-step-h");
      h.tabIndex = -1;
      h.focus({ preventScroll: true });
    }
  }

  function fromHash() {
    var m = /^#step-(\d+)$/.exec(window.location.hash);
    return m ? parseInt(m[1], 10) : 1;
  }

  prev.addEventListener("click", function () { show(current - 1, false); });
  next.addEventListener("click", function () { show(current + 1, false); });
  dots.forEach(function (d, i) {
    d.addEventListener("click", function () { show(i + 1, true); });
  });
  document.querySelectorAll(".rp-jump").forEach(function (a) {
    a.addEventListener("click", function (e) {
      e.preventDefault();
      show(parseInt(a.getAttribute("data-step"), 10), true);
      bar.scrollIntoView({ block: "start" });
    });
  });
  document.addEventListener("keydown", function (e) {
    var t = e.target;
    var typing = t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT");
    if (typing || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) {
      return;
    }
    var inScrubber = t && t.closest && t.closest(".rp-bar, .rp-index");
    if (!inScrubber) {
      return;
    }
    if (e.key === "ArrowRight") { show(current + 1, false); e.preventDefault(); }
    if (e.key === "ArrowLeft") { show(current - 1, false); e.preventDefault(); }
  });
  window.addEventListener("hashchange", function () { show(fromHash(), false); });

  bar.hidden = false;
  index.hidden = false;
  show(fromHash(), false);
})();
