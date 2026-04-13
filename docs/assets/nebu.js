/* =============================================================================
 * nebu.js — shared interactions.
 * Each module is an IIFE that no-ops if its target elements aren't on the page.
 * Load at end of <body>.
 * =========================================================================== */

(function () {
  'use strict';

  /* ---- Mobile nav drawer ------------------------------------------------ */
  (function mobileNav() {
    var btn  = document.getElementById('menu-btn');
    var menu = document.getElementById('mobile-menu');
    if (!btn || !menu) return;

    function closeMenu() {
      menu.classList.add('drawer-hidden');
      btn.setAttribute('aria-expanded', 'false');
      menu.setAttribute('aria-hidden', 'true');
      btn.textContent = '[ MENU ]';
    }

    btn.addEventListener('click', function () {
      var hidden = menu.classList.toggle('drawer-hidden');
      btn.setAttribute('aria-expanded', String(!hidden));
      menu.setAttribute('aria-hidden', String(hidden));
      btn.textContent = hidden ? '[ MENU ]' : '[ CLOSE ]';
    });

    // Close drawer whenever a link inside is activated
    menu.querySelectorAll('a').forEach(function (a) {
      a.addEventListener('click', closeMenu);
    });
  })();

  /* ---- Install-block tab switcher -------------------------------------- */
  (function installTabs() {
    var tabs   = document.querySelectorAll('.install-tab');
    var panels = document.querySelectorAll('[data-install-panel]');
    if (!tabs.length) return;

    tabs.forEach(function (tab) {
      tab.addEventListener('click', function () {
        var key = tab.getAttribute('data-install-tab');

        tabs.forEach(function (t) {
          t.classList.remove(
            'is-active',
            'text-primary-container',
            'border-primary-container',
            'font-bold'
          );
          t.classList.add('text-secondary', 'border-transparent');
        });
        tab.classList.add(
          'is-active',
          'text-primary-container',
          'border-primary-container',
          'font-bold'
        );
        tab.classList.remove('text-secondary', 'border-transparent');

        panels.forEach(function (p) {
          if (p.getAttribute('data-install-panel') === key) {
            p.classList.remove('hidden');
          } else {
            p.classList.add('hidden');
          }
        });
      });
    });
  })();

  /* ---- Sticky sidebar TOC: highlight the section currently in view ---- */
  (function tocActiveHighlight() {
    var links    = document.querySelectorAll('.toc-link');
    var sections = document.querySelectorAll('main section[id]');
    if (!links.length || !sections.length) return;
    if (typeof IntersectionObserver === 'undefined') return;

    var byId = {};
    links.forEach(function (l) {
      byId[l.getAttribute('data-toc')] = l;
    });

    var visible = new Set();
    var observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        if (e.isIntersecting) visible.add(e.target.id);
        else                  visible.delete(e.target.id);
      });

      // Pick the first section[id] in DOM order that's currently visible.
      // Stable for both up and down scrolling.
      var activeId = null;
      for (var i = 0; i < sections.length; i++) {
        if (visible.has(sections[i].id)) { activeId = sections[i].id; break; }
      }

      links.forEach(function (l) { l.classList.remove('is-active'); });
      if (activeId && byId[activeId]) byId[activeId].classList.add('is-active');
    }, { rootMargin: '-15% 0px -70% 0px', threshold: 0 });

    sections.forEach(function (s) { observer.observe(s); });
  })();

  /* ---- Copy-to-clipboard ------------------------------------------------ */
  // Picks up any element with [data-copy-target] — .copy-btn, .copy-btn--inline,
  // or any bare button that points at a code block id.
  (function copyToClipboard() {
    var btns = document.querySelectorAll('[data-copy-target]');
    if (!btns.length) return;

    function fallbackCopy(text) {
      var ta = document.createElement('textarea');
      ta.value = text;
      ta.setAttribute('readonly', '');
      ta.style.position = 'absolute';
      ta.style.left = '-9999px';
      document.body.appendChild(ta);
      ta.select();
      try { document.execCommand('copy'); } catch (_) { /* noop */ }
      document.body.removeChild(ta);
    }

    btns.forEach(function (btn) {
      btn.addEventListener('click', function () {
        var id = btn.getAttribute('data-copy-target');
        var el = document.getElementById(id);
        if (!el) return;

        var text = el.innerText.trim();
        var originalText = btn.textContent;

        function flashCopied() {
          btn.classList.add('is-copied');
          btn.textContent = '[ COPIED ]';
          setTimeout(function () {
            btn.classList.remove('is-copied');
            btn.textContent = originalText;
          }, 1200);
        }

        if (navigator.clipboard && navigator.clipboard.writeText) {
          navigator.clipboard.writeText(text).then(flashCopied, function () {
            fallbackCopy(text);
            flashCopied();
          });
        } else {
          fallbackCopy(text);
          flashCopied();
        }
      });
    });
  })();
})();
