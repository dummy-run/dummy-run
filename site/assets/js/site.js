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

// The waitlist form. Without JavaScript the form posts to the API and the API redirects to a result page.
(function () {
  var form = document.querySelector("[data-waitlist] form");
  if (!form || !window.fetch) {
    return;
  }
  var status = form.querySelector(".waitlist-status");
  var messages = {
    invalid_email: "That email address does not look right. Check it and try again.",
    consent_required: "Tick the box to agree, then try again.",
    turnstile: "The human check did not pass. Try again.",
    rate_limited: "Too many tries. Wait a little and try again.",
    unavailable: "The list is not taking sign-ups right now. Try again later."
  };

  function show(kind, text) {
    status.className = "waitlist-status is-" + kind;
    status.textContent = text;
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
      return response.json().catch(function () { return { ok: false, error: "unavailable" }; });
    }).then(function (result) {
      if (result.ok) {
        form.reset();
        show("ok", "Thank you. Check your inbox for an email and select the link in it to confirm.");
      } else {
        show("error", messages[result.error] || messages.unavailable);
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
})();
