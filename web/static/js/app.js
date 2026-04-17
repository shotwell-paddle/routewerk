/* ═══════════════════════════════════════════════════════════════
   Routewerk — Minimal JS (HTMX does the heavy lifting)
   ═══════════════════════════════════════════════════════════════ */

// ── Loading bar (CSS-driven, JS just toggles class) ──────────
document.addEventListener('htmx:beforeRequest', function() {
  var bar = document.getElementById('rw-loading-bar');
  if (!bar) return;
  bar.classList.remove('done', 'fade');
  // Force reflow so the transition restarts
  void bar.offsetWidth;
  bar.classList.add('active');
});

document.addEventListener('htmx:afterOnLoad', function() {
  var bar = document.getElementById('rw-loading-bar');
  if (!bar) return;
  bar.classList.remove('active');
  bar.classList.add('done');
  setTimeout(function() { bar.classList.add('fade'); }, 200);
  setTimeout(function() {
    bar.classList.remove('done', 'fade');
    bar.style.width = '';
  }, 600);
});

// ── Toast helper ─────────────────────────────────────────────
function showToast(msg, isError) {
  var toast = document.getElementById('rw-toast');
  if (!toast) return;
  toast.textContent = msg;
  toast.classList.toggle('error', !!isError);
  toast.classList.add('visible');
  clearTimeout(toast._timeout);
  toast._timeout = setTimeout(function() {
    toast.classList.remove('visible');
  }, 4000);
}

// ── HTMX error handling ──────────────────────────────────────

// Decide whether to swallow an error toast for an HTMX event.
// We suppress in two cases:
//   1. The server sent HX-Redirect (HTMX is already navigating elsewhere —
//      e.g. auth expiry redirecting to /login. Flashing a misleading
//      "Request failed. Please check your input." toast before the redirect
//      is pure UX noise).
//   2. The request was a background poll (hx-trigger="every …"). The user
//      didn't initiate it, so a popup is disorienting; worse, a persistent
//      failure spams a new toast every Nth second.
function shouldSuppressErrorToast(e) {
  var xhr = e.detail && e.detail.xhr;
  if (xhr && typeof xhr.getResponseHeader === 'function' && xhr.getResponseHeader('HX-Redirect')) {
    return true;
  }
  var elt = e.detail && e.detail.elt;
  if (elt && typeof elt.getAttribute === 'function') {
    var trigger = elt.getAttribute('hx-trigger') || '';
    if (/\bevery\s+\d/.test(trigger)) return true;
  }
  return false;
}

document.addEventListener('htmx:responseError', function(e) {
  if (shouldSuppressErrorToast(e)) return;
  var status = e.detail.xhr ? e.detail.xhr.status : 0;
  if (status === 429) {
    showToast('Too many requests — please wait a moment.', true);
  } else if (status >= 500) {
    showToast('Something went wrong. Please try again.', true);
  } else if (status >= 400) {
    showToast('Request failed. Please check your input.', true);
  }
});

document.addEventListener('htmx:sendError', function(e) {
  if (shouldSuppressErrorToast(e)) return;
  showToast('Connection lost. Check your network and try again.', true);
});

document.addEventListener('htmx:timeout', function(e) {
  if (shouldSuppressErrorToast(e)) return;
  showToast('Request timed out. Please try again.', true);
});

// ── Double-submit prevention ─────────────────────────────────
// Add htmx-request class to the submit button during requests
// so CSS can visually disable it and prevent re-clicks.
document.addEventListener('htmx:beforeRequest', function(e) {
  var form = e.detail.elt.closest('form');
  if (!form) return;
  var btn = form.querySelector('[type="submit"], .btn-primary');
  if (btn) btn.classList.add('htmx-request');
});
document.addEventListener('htmx:afterRequest', function(e) {
  var form = e.detail.elt.closest('form');
  if (!form) return;
  var btn = form.querySelector('[type="submit"], .btn-primary');
  if (btn) btn.classList.remove('htmx-request');
});

// ── Smooth page transitions ──────────────────────────────────
// Prevent the bounce/flicker when hx-boost navigates between pages.
// Scroll to top BEFORE the swap so the content change happens at the
// top of the viewport — no visible reflow or jump.
htmx.config.scrollIntoViewOnBoost = false;

document.addEventListener('htmx:beforeSwap', function(e) {
  var target = e.detail.target;
  if (!target || target.id !== 'main-content') return;
  window.scrollTo({ top: 0, behavior: 'instant' });
});

// ── Filter chip toggle ────────────────────────────────────────
document.addEventListener('click', function(e) {
  var chip = e.target.closest('.filter-chip');
  if (!chip) return;

  // Toggle within parent group
  var siblings = chip.parentElement.querySelectorAll('.filter-chip');
  siblings.forEach(function(s) { s.classList.remove('active'); });
  chip.classList.add('active');
});

// ── Star rating hover ─────────────────────────────────────────
document.addEventListener('mouseover', function(e) {
  var star = e.target.closest('.star-icon');
  if (!star) return;
  var label = star.closest('label');
  if (!label) return;

  var container = label.parentElement;
  var labels = Array.from(container.querySelectorAll('label'));
  var idx = labels.indexOf(label);

  labels.forEach(function(l, i) {
    var svg = l.querySelector('.star-icon');
    if (i <= idx) {
      svg.setAttribute('fill', 'var(--rw-yellow)');
      svg.setAttribute('stroke', 'var(--rw-yellow)');
    } else {
      svg.setAttribute('fill', 'none');
      svg.setAttribute('stroke', 'var(--rw-mid-gray)');
    }
  });
});

document.addEventListener('mouseout', function(e) {
  var star = e.target.closest('.star-icon');
  if (!star) return;

  var container = star.closest('label').parentElement;
  var checkedInput = container.querySelector('.star-input:checked');
  var labels = Array.from(container.querySelectorAll('label'));

  labels.forEach(function(l, i) {
    var svg = l.querySelector('.star-icon');
    var selected = checkedInput && i <= labels.indexOf(checkedInput.closest('label'));
    svg.setAttribute('fill', selected ? 'var(--rw-yellow)' : 'none');
    svg.setAttribute('stroke', selected ? 'var(--rw-yellow)' : 'var(--rw-mid-gray)');
  });
});

document.addEventListener('change', function(e) {
  if (!e.target.classList.contains('star-input')) return;

  var container = e.target.closest('label').parentElement;
  var labels = Array.from(container.querySelectorAll('label'));
  var selectedLabel = e.target.closest('label');
  var idx = labels.indexOf(selectedLabel);

  labels.forEach(function(l, i) {
    var svg = l.querySelector('.star-icon');
    if (i <= idx) {
      svg.setAttribute('fill', 'var(--rw-yellow)');
      svg.setAttribute('stroke', 'var(--rw-yellow)');
    } else {
      svg.setAttribute('fill', 'none');
      svg.setAttribute('stroke', 'var(--rw-mid-gray)');
    }
  });
});

// ── Route type → grade constraint ─────────────────────────────
// ── Mobile sidebar toggle ─────────────────────────────────────
function openSidebar() {
  document.getElementById('sidebar').classList.add('open');
  document.getElementById('sidebar-backdrop').classList.add('visible');
  document.body.classList.add('sidebar-open');
}
function closeSidebar() {
  document.getElementById('sidebar').classList.remove('open');
  document.getElementById('sidebar-backdrop').classList.remove('visible');
  document.body.classList.remove('sidebar-open');
}
document.addEventListener('click', function(e) {
  if (e.target.closest('.sidebar-toggle')) {
    var sidebar = document.getElementById('sidebar');
    if (sidebar.classList.contains('open')) {
      closeSidebar();
    } else {
      openSidebar();
    }
  }
  if (e.target.id === 'sidebar-backdrop') {
    closeSidebar();
  }
});

// ── Sidebar: update active nav based on current URL ──────────
function updateActiveNav() {
  var path = window.location.pathname;
  var items = document.querySelectorAll('.sidebar-nav .nav-item');
  var bestMatch = null;
  var bestLen = 0;

  items.forEach(function(item) {
    var href = item.getAttribute('href');
    if (!href) return;
    if (path === href || path.indexOf(href + '/') === 0) {
      if (href.length > bestLen) {
        bestMatch = item;
        bestLen = href.length;
      }
    }
  });

  items.forEach(function(item) { item.classList.remove('active'); });
  if (bestMatch) bestMatch.classList.add('active');
}

// ── HTMX: close sidebar on mobile + re-init settings ────────
document.addEventListener('htmx:afterSwap', function() {
  closeSidebar();

  updateActiveNav();

  // Re-init settings after HTMX swap
  initCircuitDragDrop();
  initSettingsFormSync();
  initCircuitAddColor();
  initHoldColorAdd();
});

// ── Settings: circuit color drag-and-drop reorder ─────────────
function initCircuitDragDrop() {
  var list = document.getElementById('circuit-list');
  if (!list) return;

  var dragItem = null;

  list.addEventListener('dragstart', function(e) {
    var item = e.target.closest('.circuit-item');
    if (!item) return;
    dragItem = item;
    item.classList.add('dragging');
    e.dataTransfer.effectAllowed = 'move';
  });

  list.addEventListener('dragend', function(e) {
    var item = e.target.closest('.circuit-item');
    if (item) item.classList.remove('dragging');
    // Clear all drop indicators
    list.querySelectorAll('.circuit-item').forEach(function(ci) {
      ci.classList.remove('drop-above', 'drop-below');
    });
    dragItem = null;
    syncCircuitColorsJSON();
  });

  list.addEventListener('dragover', function(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    var target = e.target.closest('.circuit-item');
    if (!target || target === dragItem) return;

    // Clear previous indicators
    list.querySelectorAll('.circuit-item').forEach(function(ci) {
      ci.classList.remove('drop-above', 'drop-below');
    });

    var rect = target.getBoundingClientRect();
    var midY = rect.top + rect.height / 2;
    if (e.clientY < midY) {
      target.classList.add('drop-above');
    } else {
      target.classList.add('drop-below');
    }
  });

  list.addEventListener('drop', function(e) {
    e.preventDefault();
    var target = e.target.closest('.circuit-item');
    if (!target || !dragItem || target === dragItem) return;

    var rect = target.getBoundingClientRect();
    var midY = rect.top + rect.height / 2;

    if (e.clientY < midY) {
      list.insertBefore(dragItem, target);
    } else {
      list.insertBefore(dragItem, target.nextSibling);
    }

    // Clear indicators
    list.querySelectorAll('.circuit-item').forEach(function(ci) {
      ci.classList.remove('drop-above', 'drop-below');
    });

    syncCircuitColorsJSON();
  });
}

// Sync the circuit colors order to the hidden JSON field
function syncCircuitColorsJSON() {
  var input = document.getElementById('circuit-colors-json');
  var list = document.getElementById('circuit-list');
  if (!input || !list) return;

  var colors = [];
  list.querySelectorAll('.circuit-item').forEach(function(item, i) {
    colors.push({
      name: item.getAttribute('data-name'),
      hex: item.getAttribute('data-hex'),
      sort_order: i
    });
  });

  input.value = JSON.stringify(colors);
}

// Sync before form submit
function initSettingsFormSync() {
  var form = document.querySelector('.settings-form');
  if (!form) return;

  form.addEventListener('submit', function() {
    syncCircuitColorsJSON();
  });
}

// ── Settings: add circuit color via fetch ─────────────────────
function initCircuitAddColor() {
  var btn = document.getElementById('add-color-btn');
  if (!btn) return;

  btn.addEventListener('click', function() {
    var nameInput = document.getElementById('add-color-name');
    var hexInput = document.getElementById('add-color-hex');
    var name = nameInput.value.trim();
    var hex = hexInput.value;

    if (!name) {
      nameInput.focus();
      return;
    }

    var csrf = document.querySelector('.page-body').getAttribute('data-csrf');
    var body = new URLSearchParams();
    body.append('_csrf_token', csrf);
    body.append('color_name', name);
    body.append('color_hex', hex);

    fetch('/settings/circuits/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: body.toString()
    }).then(function(resp) {
      if (resp.ok) {
        // Follow the HX-Redirect or just reload settings
        var redirect = resp.headers.get('HX-Redirect');
        if (redirect) {
          htmx.ajax('GET', redirect, { target: '#main-content', swap: 'innerHTML' });
          history.pushState({}, '', redirect);
        } else {
          htmx.ajax('GET', '/settings?saved=1', { target: '#main-content', swap: 'innerHTML' });
          history.pushState({}, '', '/settings?saved=1');
        }
      } else {
        resp.text().then(function(t) { alert(t || 'Failed to add color'); });
      }
    });
  });
}

// ── Settings: hold color add button ──────────────────────────
function initHoldColorAdd() {
  var btn = document.getElementById('add-hold-color-btn');
  if (!btn) return;

  btn.addEventListener('click', function() {
    var nameInput = document.getElementById('add-hold-color-name');
    var hexInput = document.getElementById('add-hold-color-hex');
    var name = nameInput.value.trim();
    var hex = hexInput.value;

    if (!name) {
      nameInput.focus();
      return;
    }

    var csrf = document.querySelector('.page-body').getAttribute('data-csrf');
    var body = new URLSearchParams();
    body.append('_csrf_token', csrf);
    body.append('color_name', name);
    body.append('color_hex', hex);

    fetch('/settings/hold-colors/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: body.toString()
    }).then(function(resp) {
      if (resp.ok) {
        var redirect = resp.headers.get('HX-Redirect');
        if (redirect) {
          htmx.ajax('GET', redirect, { target: '#main-content', swap: 'innerHTML' });
          history.pushState({}, '', redirect);
        } else {
          htmx.ajax('GET', '/settings?saved=1', { target: '#main-content', swap: 'innerHTML' });
          history.pushState({}, '', '/settings?saved=1');
        }
      } else {
        resp.text().then(function(t) { alert(t || 'Failed to add color'); });
      }
    });
  });
}

// ── Settings: auto-dismiss success toast ──────────────────────
document.addEventListener('DOMContentLoaded', function() {
  initCircuitDragDrop();
  initSettingsFormSync();
  initCircuitAddColor();
  initHoldColorAdd();

  var toast = document.getElementById('settings-toast');
  if (toast) {
    setTimeout(function() {
      toast.style.transition = 'opacity 0.3s';
      toast.style.opacity = '0';
      setTimeout(function() { toast.remove(); }, 300);
    }, 3000);
  }
});
