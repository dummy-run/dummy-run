// Runs in <head> before paint. Applies a saved theme choice so the page does not flash.
(function () {
  document.documentElement.classList.add("js");
  var theme = null;
  try {
    theme = window.localStorage.getItem("theme");
  } catch (e) {
    theme = null;
  }
  if (theme === "light" || theme === "dark") {
    document.documentElement.setAttribute("data-theme", theme);
  }
})();
