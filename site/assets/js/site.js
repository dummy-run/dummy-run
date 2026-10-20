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

// The terminal on the Get started page. The whole transcript is already in the page as text. This types the
// commands and shows the output in small chunks, with a pause button. With reduced motion it does nothing.
(function () {
  var term = document.querySelector("[data-terminal]");
  var reduce = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)");
  if (!term || (reduce && reduce.matches)) {
    return;
  }
  var button = term.querySelector(".t-pause");
  var lines = Array.prototype.slice.call(term.querySelectorAll(".t-line"));
  var texts = lines.map(function (l) {
    var t = l.querySelector(".t-text");
    return t ? t.textContent : null;
  });
  var timer = null;
  var paused = false;
  var next = null;
  var started = false;
  var index = 0;

  function later(fn, ms) {
    next = fn;
    timer = window.setTimeout(function () {
      timer = null;
      next = null;
      fn();
    }, ms);
  }

  function reset() {
    index = 0;
    lines.forEach(function (l, i) {
      l.classList.add("t-wait");
      var t = l.querySelector(".t-text");
      if (t) {
        t.textContent = texts[i];
      }
    });
  }

  function finish() {
    lines.forEach(function (l) {
      l.classList.remove("t-wait", "t-caret");
    });
    button.textContent = "Replay";
    button.setAttribute("data-state", "done");
  }

  function type(i, n) {
    var t = lines[i].querySelector(".t-text");
    var full = texts[i];
    var step = Math.max(1, Math.ceil(full.length / 80));
    n = Math.min(full.length, n + step);
    t.textContent = "";
    t.appendChild(document.createTextNode(full.slice(0, n)));
    var rest = document.createElement("span");
    rest.className = "t-wait";
    rest.textContent = full.slice(n);
    t.appendChild(rest);
    if (n < full.length) {
      later(function () { type(i, n); }, 30);
      return;
    }
    t.textContent = full;
    lines[i].classList.remove("t-caret");
    index = i + 1;
    later(run, 700);
  }

  function run() {
    if (index >= lines.length) {
      finish();
      return;
    }
    var i = index;
    if (texts[i] !== null) {
      lines[i].classList.remove("t-wait");
      lines[i].classList.add("t-caret");
      type(i, 0);
      return;
    }
    var shown = 0;
    while (index < lines.length && texts[index] === null && shown < 2) {
      lines[index].classList.remove("t-wait");
      index++;
      shown++;
    }
    later(run, index < lines.length && texts[index] === null ? 420 : 300);
  }

  function start() {
    if (started) {
      return;
    }
    started = true;
    later(run, 600);
  }

  reset();
  button.hidden = false;
  button.addEventListener("click", function () {
    if (button.getAttribute("data-state") === "done") {
      button.setAttribute("data-state", "");
      button.textContent = "Pause";
      paused = false;
      reset();
      later(run, 400);
      return;
    }
    if (paused) {
      paused = false;
      button.textContent = "Pause";
      if (next) {
        later(next, 100);
      }
    } else {
      paused = true;
      button.textContent = "Resume";
      if (timer !== null) {
        window.clearTimeout(timer);
        timer = null;
      }
    }
  });

  if ("IntersectionObserver" in window) {
    var io = new IntersectionObserver(function (entries) {
      if (entries.some(function (e) { return e.isIntersecting; })) {
        io.disconnect();
        start();
      }
    });
    io.observe(term);
  } else {
    start();
  }
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
        button.textContent = "Copy failed";
        status.textContent = "Copy failed. Select the code and copy it.";
        window.setTimeout(function () {
          button.textContent = "Copy";
          status.textContent = "";
        }, 3000);
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

// The flow animation. Without JavaScript, or when the visitor asks for reduced motion, it stays a still list.
(function () {
  var media = window.matchMedia("(prefers-reduced-motion: reduce)");
  var flows = document.querySelectorAll(".flow");
  Array.prototype.forEach.call(flows, function (flow) {
    var button = flow.querySelector(".flow-toggle");
    if (!button) {
      return;
    }
    function sync() {
      var still = media.matches;
      flow.classList.toggle("is-live", !still);
      button.hidden = still;
    }
    button.addEventListener("click", function () {
      var paused = button.getAttribute("aria-pressed") !== "true";
      button.setAttribute("aria-pressed", paused ? "true" : "false");
      flow.classList.toggle("is-paused", paused);
    });
    media.addEventListener("change", sync);
    sync();
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

  function show(n, move, sync) {
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
    if (sync && window.location.hash !== "#step-" + n) {
      window.history.pushState(null, "", "#step-" + n);
    }
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

  prev.addEventListener("click", function () { show(current - 1, false, true); });
  next.addEventListener("click", function () { show(current + 1, false, true); });
  dots.forEach(function (d, i) {
    d.addEventListener("click", function () { show(i + 1, true, true); });
  });
  document.querySelectorAll(".rp-jump").forEach(function (a) {
    a.addEventListener("click", function (e) {
      e.preventDefault();
      show(parseInt(a.getAttribute("data-step"), 10), true, true);
      bar.scrollIntoView({ block: "start" });
    });
  });
  document.addEventListener("keydown", function (e) {
    var t = e.target;
    var typing = t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT");
    if (typing || e.altKey || e.ctrlKey || e.metaKey || e.shiftKey) {
      return;
    }
    var inScrubber = t && t.closest && t.closest(".rp-bar, .rp-index, .rp-step");
    if (!inScrubber) {
      return;
    }
    if (e.key === "ArrowRight") { show(current + 1, false, true); e.preventDefault(); }
    if (e.key === "ArrowLeft") { show(current - 1, false, true); e.preventDefault(); }
  });
  window.addEventListener("hashchange", function () { show(fromHash(), false); });
  window.addEventListener("popstate", function () { show(fromHash(), false); });

  bar.hidden = false;
  index.hidden = false;
  show(fromHash(), false);
})();

// The audit request form. Without JavaScript the form posts to the API and the API redirects to a result page.
(function () {
  var form = document.querySelector("form[data-audit]");
  if (!form || !window.fetch) {
    return;
  }
  var status = form.querySelector(".form-status");
  var messages = {
    invalid_email: "That email address does not look right. Check it and try again.",
    invalid_request: "Something in the form is missing or too long. Check it and try again.",
    consent_required: "Tick the box so we can contact you, then try again.",
    turnstile: "The human check did not pass. Try again.",
    rate_limited: "Too many tries. Wait a little and try again.",
    unavailable: "The form is not taking requests right now. Email hello@dummyrun.dev instead."
  };
  var prefixes = { error: "Problem: ", ok: "Done: " };

  function show(kind, text) {
    status.className = "form-status is-" + kind;
    status.textContent = prefixes[kind] + text;
  }

  form.addEventListener("submit", function (event) {
    event.preventDefault();
    var body = {};
    new FormData(form).forEach(function (value, key) {
      body[key] = value;
    });
    var button = form.querySelector("button[type=submit]");
    button.disabled = true;
    var controller = new AbortController();
    var timer = setTimeout(function () { controller.abort(); }, 15000);
    fetch(form.action, {
      method: "POST",
      signal: controller.signal,
      headers: { "Content-Type": "application/json", "Accept": "application/json" },
      body: JSON.stringify(body)
    }).then(function (response) {
      return response.json().then(function (parsed) {
        return { ok: response.ok && !!parsed && parsed.ok === true, error: parsed && parsed.error };
      }).catch(function () { return { ok: false, error: response.status === 429 ? "rate_limited" : "unavailable" }; });
    }).then(function (result) {
      if (result.ok) {
        form.reset();
        show("ok", "Thank you. We will reply to the email address you gave.");
      } else {
        show("error", Object.prototype.hasOwnProperty.call(messages, result.error) ? messages[result.error] : messages.unavailable);
      }
    }).catch(function () {
      show("error", messages.unavailable);
    }).then(function () {
      clearTimeout(timer);
      button.disabled = false;
      if (window.turnstile) {
        window.turnstile.reset();
      }
    });
  });
})();

// On the audit problem page, mark the cause named in the address.
(function () {
  var list = document.querySelector(".problem-codes");
  if (!list || !window.URLSearchParams) {
    return;
  }
  var code = new URLSearchParams(window.location.search).get("code");
  var items = list.querySelectorAll("li");
  for (var i = 0; i < items.length; i++) {
    if (items[i].getAttribute("data-code") === code) {
      items[i].className = "is-match";
      items[i].setAttribute("aria-current", "true");
    }
  }
})();

// The waitlist form. Without JavaScript the form posts to the API and the API redirects to a result page.
(function () {
  if (!window.fetch) {
    return;
  }
  var forms = document.querySelectorAll("[data-waitlist] form");
  for (var i = 0; i < forms.length; i++) {
    setupWaitlist(forms[i]);
  }
})();

function setupWaitlist(form) {
  var status = form.querySelector(".waitlist-status");
  var messages = {
    invalid_email: "That email address does not look right. Check it and try again.",
    consent_required: "Tick the box to agree, then try again.",
    turnstile: "The human check did not pass. Try again.",
    rate_limited: "Too many tries. Wait a little and try again.",
    unavailable: "The list is not taking sign-ups right now. Try again later."
  };

  var prefixes = { error: "Problem: ", ok: "Done: " };

  function show(kind, text) {
    status.className = "waitlist-status is-" + kind;
    status.textContent = prefixes[kind] + text;
    status.focus();
  }

  form.addEventListener("submit", function (event) {
    event.preventDefault();
    var data = new FormData(form);
    var body = {};
    data.forEach(function (value, key) {
      body[key] = value;
    });
    var button = form.querySelector("button[type=submit]");
    button.disabled = true;
    fetch(form.action, {
      method: "POST",
      headers: { "Content-Type": "application/json", "Accept": "application/json" },
      body: JSON.stringify(body)
    }).then(function (response) {
      return response.json().then(function (parsed) {
        return { ok: response.ok && !!parsed && parsed.ok === true, error: parsed && parsed.error };
      }).catch(function () { return { ok: false, error: "unavailable" }; });
    }).then(function (result) {
      if (result.ok) {
        form.reset();
        show("ok", "Thank you. Check your inbox for an email and select the link in it to confirm.");
      } else {
        show("error", Object.prototype.hasOwnProperty.call(messages, result.error) ? messages[result.error] : messages.unavailable);
      }
    }).catch(function () {
      show("error", messages.unavailable);
    }).then(function () {
      button.disabled = false;
      if (window.turnstile) {
        window.turnstile.reset();
      }
    });
  });
}
