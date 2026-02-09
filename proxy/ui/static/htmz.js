(function () {
  function htmzFetch(element) {
    var url = element.getAttribute("data-htmz-get");
    if (!url) return;
    var targetSelector = element.getAttribute("data-htmz-target");
    var target = targetSelector ? document.querySelector(targetSelector) : element;
    if (!target) return;
    fetch(url, { headers: { Accept: "text/html" } })
      .then(function (resp) {
        if (!resp.ok) throw new Error("htmz fetch failed");
        return resp.text();
      })
      .then(function (html) {
        target.innerHTML = html;
      })
      .catch(function () {});
  }

  document.addEventListener("click", function (event) {
    var trigger = event.target.closest("[data-htmz-get]");
    if (!trigger) return;
    event.preventDefault();
    htmzFetch(trigger);
  });

  document.querySelectorAll("[data-htmz-poll]").forEach(function (element) {
    var interval = parseInt(element.getAttribute("data-htmz-poll"), 10);
    if (!interval || interval < 1000) interval = 5000;
    setInterval(function () {
      htmzFetch(element);
    }, interval);
  });
})();
