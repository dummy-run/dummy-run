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
