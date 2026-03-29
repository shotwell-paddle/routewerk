/* ═══════════════════════════════════════════════════════════════
   Routewerk — Minimal JS (HTMX does the heavy lifting)
   ═══════════════════════════════════════════════════════════════ */

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

// ── Mobile sidebar toggle ─────────────────────────────────────
document.addEventListener('click', function(e) {
  if (e.target.closest('.sidebar-toggle')) {
    document.getElementById('sidebar').classList.toggle('open');
  }
});

// ── HTMX: close sidebar on mobile navigation ─────────────────
document.addEventListener('htmx:afterSwap', function() {
  var sidebar = document.getElementById('sidebar');
  if (sidebar) sidebar.classList.remove('open');
});
