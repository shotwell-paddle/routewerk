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

// extractPlainErrorMessage pulls the short text/plain body out of an xhr
// response so the toast can show the server's actual rejection reason instead
// of the generic "Request failed" fallback. We only surface the body when:
//   - the server explicitly said text/plain (avoids rendering HTML error pages
//     as raw markup in a toast), and
//   - the body is short enough to fit (≤ 140 chars — anything longer either
//     overflows the toast or is probably a stack trace that we don't want to
//     leak to users).
function extractPlainErrorMessage(xhr) {
  if (!xhr || typeof xhr.getResponseHeader !== 'function') return '';
  var ct = xhr.getResponseHeader('Content-Type') || '';
  if (ct.indexOf('text/plain') === -1) return '';
  var body = (xhr.responseText || '').trim();
  if (!body || body.length > 140) return '';
  return body;
}

document.addEventListener('htmx:responseError', function(e) {
  if (shouldSuppressErrorToast(e)) return;
  var xhr = e.detail.xhr;
  var status = xhr ? xhr.status : 0;
  if (status === 429) {
    showToast('Too many requests — please wait a moment.', true);
  } else if (status >= 500) {
    showToast('Something went wrong. Please try again.', true);
  } else if (status >= 400) {
    var serverMsg = extractPlainErrorMessage(xhr);
    showToast(serverMsg || 'Request failed. Please check your input.', true);
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
  initCardBatchPicker();
  initCardBatchPreviewFallback();
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

// ── Card batch picker ─────────────────────────────────────────
// Client-side glue for the route-card batch creation form:
//   - live search that filters candidate rows (+ hides empty wall groups)
//   - bulk actions: "Select all visible", "Clear", per-wall "select all"
//   - running selection summary: count, derived sheet count, Save enable
// Everything is data-attribute driven so the template stays declarative
// and we don't have to import a framework just to tick some boxes.
function initCardBatchPicker() {
  var root = document.getElementById('card-batch-picker');
  if (!root) return;

  var search       = root.querySelector('[data-card-batch-search]');
  var rows         = root.querySelectorAll('[data-card-batch-row]');
  var groups       = root.querySelectorAll('[data-card-batch-group]');
  var emptyState   = root.querySelector('[data-card-batch-empty]');
  var countEl      = root.querySelector('[data-card-batch-count]');
  var sheetsEl     = root.querySelector('[data-card-batch-sheets]');
  var sheetsPlural = root.querySelector('[data-card-batch-sheets-plural]');
  var submit       = root.querySelector('[data-card-batch-submit]');

  var CARDS_PER_SHEET = 8;

  function refreshSelection() {
    var checked = root.querySelectorAll('[data-card-batch-checkbox]:checked').length;
    var sheets = Math.ceil(checked / CARDS_PER_SHEET);
    if (countEl)  countEl.textContent  = String(checked);
    if (sheetsEl) sheetsEl.textContent = String(sheets);
    if (sheetsPlural) sheetsPlural.textContent = sheets === 1 ? '' : 's';
    if (submit) submit.disabled = checked === 0;

    // Per-wall row counts: the "(12)" badge in each header reflects the
    // number of rows currently visible in that wall group so setters know
    // how many they'd grab with "Select all in wall".
    groups.forEach(function(group) {
      var visible = group.querySelectorAll('[data-card-batch-row]:not([hidden])').length;
      var countBadge = group.querySelector('[data-card-batch-group-count]');
      if (countBadge) countBadge.textContent = String(visible);
      // Hide the whole group if every row is filtered out.
      group.hidden = visible === 0;
    });
  }

  function applySearch() {
    var q = search ? search.value.trim().toLowerCase() : '';
    var anyVisible = false;
    rows.forEach(function(row) {
      var hay = (row.getAttribute('data-search-haystack') || '').toLowerCase();
      var match = q === '' || hay.indexOf(q) !== -1;
      row.hidden = !match;
      if (match) anyVisible = true;
    });
    if (emptyState) emptyState.hidden = anyVisible;
    refreshSelection();
  }

  if (search) {
    search.addEventListener('input', applySearch);
  }

  root.addEventListener('change', function(e) {
    if (e.target && e.target.matches('[data-card-batch-checkbox]')) {
      refreshSelection();
    }
  });

  root.addEventListener('click', function(e) {
    var selectAll = e.target.closest('[data-card-batch-select-all]');
    if (selectAll) {
      e.preventDefault();
      rows.forEach(function(row) {
        if (row.hidden) return;
        var cb = row.querySelector('[data-card-batch-checkbox]');
        if (cb) cb.checked = true;
      });
      refreshSelection();
      return;
    }

    var clearBtn = e.target.closest('[data-card-batch-clear]');
    if (clearBtn) {
      e.preventDefault();
      root.querySelectorAll('[data-card-batch-checkbox]').forEach(function(cb) {
        cb.checked = false;
      });
      refreshSelection();
      return;
    }

    var wallBtn = e.target.closest('[data-card-batch-select-wall]');
    if (wallBtn) {
      e.preventDefault();
      var group = wallBtn.closest('[data-card-batch-group]');
      if (!group) return;
      var wallRows = group.querySelectorAll('[data-card-batch-row]:not([hidden]) [data-card-batch-checkbox]');
      // Toggle: if all visible are already checked, uncheck them; otherwise
      // check them all. Makes the button a true bulk toggle rather than a
      // one-way selector.
      var allChecked = Array.prototype.every.call(wallRows, function(cb) { return cb.checked; });
      wallRows.forEach(function(cb) { cb.checked = !allChecked; });
      refreshSelection();
    }
  });

  applySearch();
}

// ── Card batch preview fallback ───────────────────────────────
// The /preview.png endpoint renders synchronously and can legitimately fail
// (route deleted mid-session, storage blip). A raw broken-image icon looks
// alarming, so we swap in a friendly "preview unavailable" placeholder and
// hide the <img> entirely when it errors. CSP blocks inline onerror, so we
// bind the handler from here.
function initCardBatchPreviewFallback() {
  document.querySelectorAll('[data-card-batch-preview-img]').forEach(function(img) {
    if (img.dataset.fallbackBound === '1') return;
    img.dataset.fallbackBound = '1';
    img.addEventListener('error', function() {
      var fig = img.closest('[data-card-batch-preview-figure]');
      if (!fig) return;
      img.hidden = true;
      var fallback = fig.querySelector('.card-batch-preview-fallback');
      if (fallback) fallback.hidden = false;
    }, { once: true });
  });
}

// ── Settings: auto-dismiss success toast ──────────────────────
document.addEventListener('DOMContentLoaded', function() {
  initCircuitDragDrop();
  initSettingsFormSync();
  initCircuitAddColor();
  initHoldColorAdd();
  initCardBatchPicker();
  initCardBatchPreviewFallback();

  var toast = document.getElementById('settings-toast');
  if (toast) {
    setTimeout(function() {
      toast.style.transition = 'opacity 0.3s';
      toast.style.opacity = '0';
      setTimeout(function() { toast.remove(); }, 300);
    }, 3000);
  }
});

// ── Photo upload UX ──────────────────────────────────────────
// Give the user clear feedback during a photo upload:
//   1. Client-side reject (too big / wrong type) before the request fires,
//      so they don't watch a spinner for 3s only to get a server 400.
//   2. Client-side HEIC → JPEG conversion via heic2any. Our server is
//      pure Go (CGO_ENABLED=0), so it can't decode HEIC. iPhones still
//      ship HEIC by default, though, so we convert in the browser before
//      the upload ever happens. heic2any is ~2 MB of WASM that we
//      lazy-load from CDN only when the user actually picks a HEIC file.
//   3. Inline "Uploading photo…" / "Converting HEIC…" status right next
//      to the form — toasts in the corner are easy to miss on mobile.
//   4. Live percent progress via htmx's xhr.upload.progress bridge —
//      matters for a 3–4 MB photo over a flaky gym wifi connection.
//   5. Clear error message inline on server reject, anchored to the form.

var UPLOAD_MAX_BYTES = 5 * 1024 * 1024; // must match server's maxInputBytes
var UPLOAD_ALLOWED_TYPES = /^image\/(jpeg|png|webp|heic|heif)$/i;
// Some browsers (and especially iOS Safari on older iOS) report HEIC as
// an empty string or application/octet-stream. Fall back to extension so
// we don't reject on the client a file the user legitimately picked.
var UPLOAD_ALLOWED_EXT = /\.(jpe?g|png|webp|heic|heif)$/i;
var UPLOAD_HEIC_EXT = /\.(heic|heif)$/i;
var UPLOAD_HEIC_MIME = /^image\/(heic|heif)$/i;

// CDN URL for heic2any. Pinned to a specific version so a CDN takeover
// or breaking release can't silently change what runs in our users'
// browsers. If this CDN goes down we lose HEIC-on-iPhone; the error
// message tells the user how to work around it (toggle camera format).
var HEIC2ANY_URL = 'https://cdnjs.cloudflare.com/ajax/libs/heic2any/0.0.4/heic2any.min.js';

// A form opts into the upload UX by including a `.upload-status` element.
// We key off that rather than a specific class name so the route-edit form
// (which embeds a photo input among many other fields) gets the treatment
// without having to wear a "photo-upload-form" marker class.
function hasUploadStatus(el) {
  if (!el || typeof el.querySelector !== 'function') return false;
  return !!el.querySelector('.upload-status');
}

// True when the form is about to send a real photo payload. For forms
// that embed an optional photo input (like route-form.html), a submit
// without a selected file is just editing text — we shouldn't pop a
// "Uploading photo…" indicator in that case.
function hasPendingPhotoFile(form) {
  if (!form) return false;
  var inputs = form.querySelectorAll('input[type="file"][name="photo"]');
  for (var i = 0; i < inputs.length; i++) {
    if (inputs[i].files && inputs[i].files.length > 0) return true;
  }
  return false;
}

function setUploadStatus(form, state, msg) {
  if (!form) return;
  var el = form.querySelector('.upload-status');
  if (!el) return;
  el.classList.remove('uploading', 'error');
  if (!state) {
    el.textContent = '';
    return;
  }
  el.classList.add(state);
  if (state === 'uploading') {
    // innerHTML is fine here — msg is a short string we control
    el.innerHTML = '<span class="upload-spinner" aria-hidden="true"></span><span></span>';
    el.lastChild.textContent = msg;
  } else {
    el.textContent = msg;
  }
}

function isHEICFile(file) {
  if (!file) return false;
  return UPLOAD_HEIC_MIME.test(file.type || '') || UPLOAD_HEIC_EXT.test(file.name || '');
}

// Lazy-load heic2any the first time we need it. Subsequent calls reuse
// the same promise — one network fetch, one WASM compile.
var heic2anyPromise = null;
function loadHEIC2Any() {
  if (heic2anyPromise) return heic2anyPromise;
  heic2anyPromise = new Promise(function(resolve, reject) {
    if (window.heic2any) { resolve(window.heic2any); return; }
    var s = document.createElement('script');
    s.src = HEIC2ANY_URL;
    s.async = true;
    s.onload = function() {
      if (window.heic2any) resolve(window.heic2any);
      else reject(new Error('heic2any loaded but not available on window'));
    };
    s.onerror = function() {
      // Reset so a retry can try again — e.g. user got online and picks
      // another HEIC after the first network-dropped load attempt.
      heic2anyPromise = null;
      reject(new Error('Could not load HEIC converter — check your network'));
    };
    document.head.appendChild(s);
  });
  return heic2anyPromise;
}

// Swap a HEIC File in the input for its converted JPEG equivalent.
// Resolves once the input's .files reflects the new JPEG, ready to upload.
function convertHEICInInput(input) {
  var file = input.files[0];
  var form = input.closest('form');
  setUploadStatus(form, 'uploading', 'Converting HEIC to JPEG…');
  return loadHEIC2Any().then(function(heic2any) {
    return heic2any({ blob: file, toType: 'image/jpeg', quality: 0.9 });
  }).then(function(result) {
    // heic2any returns Blob OR Blob[] (for multi-image HEIC) — take the first.
    var blob = Array.isArray(result) ? result[0] : result;
    var name = (file.name || 'photo').replace(/\.(heic|heif)$/i, '.jpg');
    var jpeg = new File([blob], name, { type: 'image/jpeg', lastModified: Date.now() });
    if (jpeg.size > UPLOAD_MAX_BYTES) {
      throw new Error('Converted photo is still too large (' +
        Math.round(jpeg.size / (1024 * 1024)) + ' MB). Try a smaller photo.');
    }
    // Modern browsers let us write to input.files via DataTransfer. This
    // is supported in all evergreen browsers + iOS Safari 14.5+.
    var dt = new DataTransfer();
    dt.items.add(jpeg);
    input.files = dt.files;
    // Reflect the new filename in the UI (matches the non-HEIC path's
    // inline onchange handler, which already fired with the original name).
    var filenameEl = form.querySelector('.photo-filename');
    if (filenameEl) filenameEl.textContent = name;
    setUploadStatus(form, null);
    return jpeg;
  });
}

function checkPhotoFile(input) {
  var file = input.files && input.files[0];
  var form = input.closest('form');
  if (!file || !form) return true;
  if (file.size > UPLOAD_MAX_BYTES) {
    setUploadStatus(form, 'error', 'File is too large (max 5 MB). Choose a smaller photo.');
    input.value = '';
    return false;
  }
  var typeOK = UPLOAD_ALLOWED_TYPES.test(file.type || '') ||
               UPLOAD_ALLOWED_EXT.test(file.name || '');
  if (!typeOK) {
    setUploadStatus(form, 'error', 'Unsupported format — use JPEG, PNG, WebP, or HEIC.');
    input.value = '';
    return false;
  }
  setUploadStatus(form, null);
  return true;
}

// Track forms mid-HEIC-conversion. While a conversion is in-flight:
//   - the user may click Upload (beforeRequest handler cancels and waits)
//   - the dropzone's auto-submit may fire (we cancel and re-trigger later)
var convertingForms = new WeakMap();

// Replace the inline auto-submit on the session-photos dropzone. We
// still support the attribute `data-auto-submit` to signal intent, but
// the submit now goes through this function so it waits on conversion.
function afterPhotoReady(form, autoSubmit) {
  if (autoSubmit) {
    // requestSubmit triggers HTMX's normal submit flow and our
    // htmx:beforeRequest handler will pick up the uploading UX.
    if (typeof form.requestSubmit === 'function') form.requestSubmit();
    else form.submit();
  }
}

document.addEventListener('change', function(e) {
  var input = e.target;
  if (!input || input.type !== 'file' || input.name !== 'photo') return;
  var form = input.closest('form');
  if (!hasUploadStatus(form)) return;

  var autoSubmit = input.hasAttribute('data-auto-submit');

  if (!checkPhotoFile(input)) {
    // Input has been cleared; don't propagate to any inline auto-submit.
    e.stopPropagation();
    e.preventDefault();
    return;
  }

  var file = input.files[0];
  if (!isHEICFile(file)) {
    // Non-HEIC path — ready to upload immediately.
    afterPhotoReady(form, autoSubmit);
    return;
  }

  // HEIC path — we need to convert first. Mark the form as converting
  // so the submit handler knows to wait.
  var pending = convertHEICInInput(input).then(function() {
    convertingForms.delete(form);
    afterPhotoReady(form, autoSubmit);
  }).catch(function(err) {
    convertingForms.delete(form);
    input.value = '';
    setUploadStatus(form, 'error', (err && err.message) ||
      'Could not convert HEIC — try a JPEG or toggle iPhone Settings → Camera → Formats → Most Compatible.');
  });
  convertingForms.set(form, pending);
});

// Track whether the in-flight request is a photo upload. We only want to
// surface the upload UX for submits that actually carry a file; a form
// with an optional photo input shouldn't show "Uploading photo…" on a
// text-only edit.
var uploadingForms = new WeakSet();

document.addEventListener('htmx:beforeRequest', function(e) {
  var form = e.detail.elt && e.detail.elt.closest ? e.detail.elt.closest('form') : null;
  if (!hasUploadStatus(form) || !hasPendingPhotoFile(form)) return;
  uploadingForms.add(form);
  setUploadStatus(form, 'uploading', 'Uploading photo…');
});

document.addEventListener('htmx:xhr:progress', function(e) {
  var form = e.detail.elt && e.detail.elt.closest ? e.detail.elt.closest('form') : null;
  if (!form || !uploadingForms.has(form)) return;
  var loaded = e.detail.loaded || 0;
  var total = e.detail.total || 0;
  if (!total) return;
  var pct = Math.round((loaded / total) * 100);
  if (pct >= 100) {
    // Upload bytes done — server is decoding/resizing/uploading to S3.
    setUploadStatus(form, 'uploading', 'Processing…');
  } else {
    setUploadStatus(form, 'uploading', 'Uploading photo… ' + pct + '%');
  }
});

document.addEventListener('htmx:responseError', function(e) {
  var form = e.detail.elt && e.detail.elt.closest ? e.detail.elt.closest('form') : null;
  if (!form || !uploadingForms.has(form)) return;
  uploadingForms.delete(form);
  var xhr = e.detail.xhr;
  var msg = extractPlainErrorMessage(xhr);
  if (!msg) {
    var status = xhr ? xhr.status : 0;
    if (status === 413) msg = 'File is too large.';
    else if (status === 429) msg = 'Too many uploads — please wait a moment.';
    else if (status >= 500) msg = 'Server error — try again in a moment.';
    else msg = 'Upload failed. Please try again.';
  }
  setUploadStatus(form, 'error', msg);
});

document.addEventListener('htmx:sendError', function(e) {
  var form = e.detail.elt && e.detail.elt.closest ? e.detail.elt.closest('form') : null;
  if (!form || !uploadingForms.has(form)) return;
  uploadingForms.delete(form);
  setUploadStatus(form, 'error', 'Connection lost. Check your network.');
});

document.addEventListener('htmx:timeout', function(e) {
  var form = e.detail.elt && e.detail.elt.closest ? e.detail.elt.closest('form') : null;
  if (!form || !uploadingForms.has(form)) return;
  uploadingForms.delete(form);
  setUploadStatus(form, 'error', 'Upload timed out. Please try again.');
});

document.addEventListener('htmx:afterRequest', function(e) {
  // Happy path: successful swap replaces the form, so the WeakSet entry
  // becomes garbage. But for responses where the form survives (e.g. a
  // 2xx that didn't swap the form's subtree), drop the tracking bit.
  var form = e.detail.elt && e.detail.elt.closest ? e.detail.elt.closest('form') : null;
  if (form) uploadingForms.delete(form);
});
