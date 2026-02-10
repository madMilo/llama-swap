(function () {
  function readResponseSnippet(resp) {
    return resp
      .text()
      .then(function (body) {
        if (!body) return "";
        return body.slice(0, 200);
      })
      .catch(function () {
        return "";
      });
  }

  function throwHttpError(method, url, resp) {
    return readResponseSnippet(resp).then(function (snippet) {
      var details = method + " " + url + " failed with status " + resp.status;
      if (snippet) {
        details += ": " + snippet;
      }

      var error = new Error(details);
      error.userMessage = "Operation failed. Please try again.";
      throw error;
    });
  }

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

  function htmzPost(element) {
    var url = element.getAttribute("data-htmz-post");
    if (!url) return;
    var targetSelector = element.getAttribute("data-htmz-target");
    var swap = element.getAttribute("data-htmz-swap");

    fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" }
    })
      .then(function (resp) {
        if (!resp.ok) return throwHttpError("POST", url, resp);
        if (swap === "none") return "";
        return resp.text();
      })
      .then(function (html) {
        if (swap !== "none" && targetSelector) {
          var target = document.querySelector(targetSelector);
          if (target) {
            target.outerHTML = html;
          }
        }
      })
      .catch(function (err) {
        console.error("htmz POST error:", err);
      });
  }

  document.addEventListener("click", function (event) {
    var getTrigger = event.target.closest("[data-htmz-get]");
    var postTrigger = event.target.closest("[data-htmz-post]");

    if (!getTrigger && !postTrigger) return;

    event.preventDefault();

    if (getTrigger) {
      htmzFetch(getTrigger);
    } else if (postTrigger) {
      htmzPost(postTrigger);
    }
  });

  document.querySelectorAll("[data-htmz-poll]").forEach(function (element) {
    var interval = parseInt(element.getAttribute("data-htmz-poll"), 10);
    if (!interval || interval < 1000) interval = 5000;
    setInterval(function () {
      htmzFetch(element);
    }, interval);
  });

  // Toast notification system
  function showToast(message, type) {
    var toast = document.createElement('div');
    toast.className = 'toast toast-' + type;
    toast.textContent = message;
    toast.style.cssText = 'position:fixed;bottom:20px;right:20px;padding:12px 20px;border-radius:6px;color:white;z-index:10000;animation:slideIn 0.3s;';

    if (type === 'error') {
      toast.style.background = '#dc2626';
    } else if (type === 'success') {
      toast.style.background = '#059669';
    }

    document.body.appendChild(toast);

    setTimeout(function() {
      toast.style.animation = 'slideOut 0.3s';
      setTimeout(function() {
        if (toast.parentNode) {
          document.body.removeChild(toast);
        }
      }, 300);
    }, 3000);
  }

  // Track in-flight requests
  var pendingRequests = {};

  // Expose public API
  window.htmz = {
    get: function(url, targetSelector) {
      var target = targetSelector ? document.querySelector(targetSelector) : null;
      if (!target) {
        console.error("htmz.get: target not found:", targetSelector);
        return;
      }

      fetch(url, { headers: { Accept: "text/html" } })
        .then(function (resp) {
          if (!resp.ok) throw new Error("htmz fetch failed");
          return resp.text();
        })
        .then(function (html) {
          target.innerHTML = html;
        })
        .catch(function (err) {
          console.error("htmz GET error:", err);
          showToast("Failed to refresh: " + err.message, "error");
        });
    },

    post: function(url, targetSelector, swap) {
      // Prevent duplicate requests
      if (pendingRequests[url]) {
        console.log("Request already in progress:", url);
        return;
      }

      pendingRequests[url] = true;

      fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" }
      })
        .then(function (resp) {
          if (!resp.ok) return throwHttpError("POST", url, resp);

          // Show success toast for model operations
          if (url.includes('/api/models/load/')) {
            showToast("Model loading started", "success");
          } else if (url.includes('/api/models/unload/')) {
            showToast("Model unloaded successfully", "success");
          }

          if (swap === "none") return "";
          return resp.text();
        })
        .then(function (html) {
          if (swap !== "none" && targetSelector) {
            var target = document.querySelector(targetSelector);
            if (target) {
              target.outerHTML = html;
            }
          }
        })
        .catch(function (err) {
          console.error("htmz POST error:", err && err.message ? err.message : err);
          showToast((err && err.userMessage) || "Operation failed. Please try again.", "error");
        })
        .finally(function() {
          // Clear request tracking after a short delay
          setTimeout(function() {
            delete pendingRequests[url];
          }, 500);
        });
    }
  };
})();
