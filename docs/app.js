/* ============================================================
   shortorder landing — interactions
   ============================================================ */
(function () {
  'use strict';

  /* ---------- THEME ---------- */
  var root = document.documentElement;
  var stored = null;
  try { stored = localStorage.getItem('so-theme'); } catch (e) {}
  if (stored) root.setAttribute('data-theme', stored);

  function toggleTheme() {
    var cur = root.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
    var next = cur === 'light' ? 'dark' : 'light';
    root.setAttribute('data-theme', next);
    try { localStorage.setItem('so-theme', next); } catch (e) {}
  }
  document.addEventListener('click', function (e) {
    if (e.target.closest('.theme-toggle')) toggleTheme();
  });

  /* ---------- TWEAKS APPLIER (called by the React island) ---------- */
  var ACCENTS = {
    green:  'oklch(0.74 0.14 156)',
    amber:  'oklch(0.78 0.14 68)',
    blue:   'oklch(0.66 0.15 250)',
    purple: 'oklch(0.67 0.15 300)'
  };
  window.applyTweaks = function (t) {
    if (!t) return;
    if (t.accent) root.style.setProperty('--accent', ACCENTS[t.accent] || t.accent);
    if (t.hero) setHero(t.hero);
  };

  function setHero(which) {
    var stages = document.querySelectorAll('.hero-stage');
    var found = false;
    stages.forEach(function (s) {
      var on = s.getAttribute('data-hero') === which;
      s.classList.toggle('active', on);
      if (on) found = true;
    });
    if (!found && stages[0]) stages[0].classList.add('active');
    // (re)kick the print animation if hero A becomes visible
    kickPrintAnim();
  }
  window.__setHero = setHero;

  /* ---------- LATEST RELEASE ---------- */
  function syncLatestRelease() {
    var pills = document.querySelectorAll('[data-latest-release]');
    if (!pills.length || !window.fetch) return;

    fetch('https://api.github.com/repos/aphexddb/shortorder/releases/latest', {
      headers: { Accept: 'application/vnd.github+json' }
    })
      .then(function (res) { return res.ok ? res.json() : null; })
      .then(function (release) {
        var tag = release && release.tag_name;
        if (!tag) return;
        pills.forEach(function (pill) {
          pill.textContent = tag;
          pill.setAttribute('title', 'Latest GitHub release: ' + tag);
          pill.setAttribute('aria-label', 'Latest shortorder release: ' + tag);
        });
      })
      .catch(function () {});
  }

  /* ---------- CODE TABS ---------- */
  document.addEventListener('click', function (e) {
    var tab = e.target.closest('.code-tab');
    if (!tab) return;
    var block = tab.closest('.codeblock');
    var key = tab.getAttribute('data-tab');
    block.querySelectorAll('.code-tab').forEach(function (t) { t.classList.toggle('active', t === tab); });
    block.querySelectorAll('.code-pane').forEach(function (p) {
      p.classList.toggle('active', p.getAttribute('data-pane') === key);
    });
  });

  /* ---------- INSTALL TABS ---------- */
  document.addEventListener('click', function (e) {
    var b = e.target.closest('.os-btn');
    if (!b) return;
    var key = b.getAttribute('data-os');
    var scope = b.closest('.install-grid');
    scope.querySelectorAll('.os-btn').forEach(function (x) { x.classList.toggle('active', x === b); });
    scope.querySelectorAll('.install-pane').forEach(function (p) {
      p.classList.toggle('active', p.getAttribute('data-os') === key);
    });
  });

  /* ---------- COPY BUTTONS ---------- */
  function copyText(txt, btn) {
    var done = function () {
      if (!btn) return;
      var label = btn.querySelector('.copy-label');
      btn.classList.add('copied');
      var prev = label ? label.textContent : null;
      if (label) label.textContent = 'copied';
      setTimeout(function () {
        btn.classList.remove('copied');
        if (label && prev !== null) label.textContent = prev;
      }, 1400);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(txt).then(done, fallback);
    } else { fallback(); }
    function fallback() {
      var ta = document.createElement('textarea');
      ta.value = txt; ta.style.position = 'fixed'; ta.style.opacity = '0';
      document.body.appendChild(ta); ta.select();
      try { document.execCommand('copy'); } catch (e) {}
      document.body.removeChild(ta); done();
    }
  }
  document.addEventListener('click', function (e) {
    var btn = e.target.closest('[data-copy]');
    if (!btn) return;
    var sel = btn.getAttribute('data-copy');
    var txt = '';
    if (sel === 'self-prev') {
      var body = btn.closest('.code-body');
      var code = body ? body.querySelector('.code-pane.active .code-pre, .install-pane.active .code-pre') : null;
      txt = code ? code.textContent : '';
    } else if (sel.charAt(0) === '#') {
      var el = document.querySelector(sel);
      txt = el ? (el.getAttribute('data-raw') || el.textContent) : '';
    } else {
      txt = btn.getAttribute('data-text') || sel;
    }
    copyText(txt.trim(), btn);
  });

  /* ---------- SCROLL REVEAL ---------- */
  var io = new IntersectionObserver(function (entries) {
    entries.forEach(function (en) {
      if (en.isIntersecting) { en.target.classList.add('in'); io.unobserve(en.target); }
    });
  }, { threshold: 0.12, rootMargin: '0px 0px -8% 0px' });
  function observeReveals() {
    document.querySelectorAll('.reveal:not(.in)').forEach(function (el) { io.observe(el); });
  }

  /* ---------- PRINT ANIMATION (hero A feed) ---------- */
  var printTimer = null;
  function kickPrintAnim() {
    var feed = document.querySelector('.hero-stage.active .feed-paper');
    if (!feed) return;
    if (printTimer) { clearTimeout(printTimer); printTimer = null; }
    runPrint(feed);
  }
  function runPrint(feed) {
    // measure full height then animate from 0 -> full once
    feed.style.transition = 'none';
    feed.style.maxHeight = 'none';
    var full = feed.scrollHeight;
    feed.style.overflow = 'hidden';
    feed.style.maxHeight = '0px';
    // force reflow
    void feed.offsetHeight;
    feed.style.transition = 'max-height 2.6s cubic-bezier(.6,.02,.25,1)';
    requestAnimationFrame(function () {
      requestAnimationFrame(function () { feed.style.maxHeight = full + 'px'; });
    });
  }

  /* ---------- BOOT ---------- */
  function boot() {
    syncLatestRelease();
    observeReveals();
    // default hero if island hasn't set one yet
    if (!document.querySelector('.hero-stage.active')) setHero('split');
    kickPrintAnim();
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else { boot(); }

  // expose for island re-observe after DOM tweaks
  window.__observeReveals = observeReveals;
})();
