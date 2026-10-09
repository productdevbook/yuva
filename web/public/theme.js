;(function () {
  var dark = false
  try {
    var t = localStorage.getItem("theme")
    dark = t === "dark" || (t !== "light" && window.matchMedia("(prefers-color-scheme: dark)").matches)
  } catch (e) {
    dark = window.matchMedia("(prefers-color-scheme: dark)").matches
  }
  document.documentElement.classList.toggle("dark", dark)
})()
