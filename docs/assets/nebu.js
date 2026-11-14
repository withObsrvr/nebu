/* =============================================================================
 * nebu.js — shared interactions for the documentation site.
 * Each module no-ops if its target elements aren't on the page.
 * Load at end of <body>.
 * =========================================================================== */

(function () {
  'use strict';

  /* ---- Clipboard -------------------------------------------------------- */

  function writeText(text) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(text).catch(function () {
        fallbackCopy(text);
      });
    }
    fallbackCopy(text);
    return Promise.resolve();
  }

  function fallbackCopy(text) {
    var ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.position = 'absolute';
    ta.style.left = '-9999px';
    document.body.appendChild(ta);
    ta.select();
    try { document.execCommand('copy'); } catch (_) { /* nothing else to try */ }
    document.body.removeChild(ta);
  }

  function flash(btn) {
    var original = btn.getAttribute('data-label') || btn.textContent;
    btn.setAttribute('data-label', original);
    btn.classList.add('is-copied');
    btn.textContent = 'Copied';
    setTimeout(function () {
      btn.classList.remove('is-copied');
      btn.textContent = original;
    }, 1200);
  }

  // [data-copy] copies the visible <pre> of the nearest [data-code] box.
  // A tabbed box holds one <pre> per tab; only the shown one is copied.
  document.querySelectorAll('[data-copy]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var box = btn.closest('[data-code]');
      if (!box) return;
      var pres = box.querySelectorAll('pre');
      var pre = null;
      for (var i = 0; i < pres.length; i++) {
        if (!pres[i].closest('[hidden]')) { pre = pres[i]; break; }
      }
      if (!pre) return;
      writeText(pre.textContent.replace(/\s+$/, '')).then(function () { flash(btn); });
    });
  });

  // [data-copy-page] copies the page's main column as plain text.
  document.querySelectorAll('[data-copy-page]').forEach(function (btn) {
    btn.addEventListener('click', function () {
      var main = document.querySelector('main');
      if (!main) return;
      writeText(main.innerText).then(function () { flash(btn); });
    });
  });

  /* ---- Tabs (install method) ------------------------------------------- */
  // A [data-tabs] tablist switches every [data-panel] in the same section:
  // the code panel and the note under it.
  document.querySelectorAll('[data-tabs]').forEach(function (list) {
    var scope = list.closest('section') || document;
    var tabs = Array.prototype.slice.call(list.querySelectorAll('[role="tab"]'));

    function select(tab, focus) {
      var key = tab.getAttribute('data-tab');
      tabs.forEach(function (t) {
        var on = t === tab;
        t.setAttribute('aria-selected', String(on));
        t.tabIndex = on ? 0 : -1;
      });
      scope.querySelectorAll('[data-panel]').forEach(function (p) {
        p.hidden = p.getAttribute('data-panel') !== key;
      });
      if (focus) tab.focus();
    }

    tabs.forEach(function (tab, i) {
      tab.addEventListener('click', function () { select(tab, false); });
      tab.addEventListener('keydown', function (e) {
        var next = null;
        if (e.key === 'ArrowRight') next = tabs[(i + 1) % tabs.length];
        else if (e.key === 'ArrowLeft') next = tabs[(i - 1 + tabs.length) % tabs.length];
        else if (e.key === 'Home') next = tabs[0];
        else if (e.key === 'End') next = tabs[tabs.length - 1];
        if (!next) return;
        e.preventDefault();
        select(next, true);
      });
    });
  });

  /* ---- Search ----------------------------------------------------------- */
  // Indexes every page's headings on first use by fetching the pages linked
  // from the sidebar. Static, same-origin, no service behind it.
  (function search() {
    var box = document.querySelector('[data-search]');
    if (!box) return;
    var input = box.querySelector('input');
    if (!input) return;

    var list = document.createElement('ul');
    list.className = 'search__results';
    list.id = 'search-results';
    list.setAttribute('role', 'listbox');
    list.hidden = true;
    box.appendChild(list);

    input.setAttribute('role', 'combobox');
    input.setAttribute('aria-autocomplete', 'list');
    input.setAttribute('aria-controls', list.id);
    input.setAttribute('aria-expanded', 'false');
    input.setAttribute('autocomplete', 'off');

    var index = null;
    var loading = null;
    var active = -1;

    function pagesFromSidebar() {
      var seen = {};
      var urls = [];
      document.querySelectorAll('nav[aria-label="Documentation"] a[href]').forEach(function (a) {
        var href = a.getAttribute('href');
        if (/^https?:/.test(href) || href.charAt(0) === '#') return;
        href = href.split('#')[0];
        if (!seen[href]) { seen[href] = true; urls.push(href); }
      });
      return urls;
    }

    function headingsOf(doc, url) {
      var page = (doc.querySelector('main h1') || {}).textContent || url;
      var out = [{ title: page, page: '', url: url }];
      doc.querySelectorAll('main h2[id], main h3[id], main section[id] > h2').forEach(function (h) {
        var id = h.id || (h.parentElement && h.parentElement.id);
        if (!id) return;
        out.push({ title: h.textContent.trim(), page: page, url: url + '#' + id });
      });
      return out;
    }

    function load() {
      if (index) return Promise.resolve(index);
      if (loading) return loading;
      var parser = new DOMParser();
      loading = Promise.all(pagesFromSidebar().map(function (url) {
        return fetch(url)
          .then(function (r) { return r.ok ? r.text() : ''; })
          .then(function (html) { return html ? headingsOf(parser.parseFromString(html, 'text/html'), url) : []; })
          .catch(function () { return []; });
      })).then(function (lists) {
        index = [].concat.apply([], lists);
        // Opened from disk, fetch is refused: fall back to this page alone.
        if (!index.length) index = headingsOf(document, location.pathname.split('/').pop() || 'index.html');
        return index;
      });
      return loading;
    }

    function close() {
      list.hidden = true;
      input.setAttribute('aria-expanded', 'false');
      input.removeAttribute('aria-activedescendant');
      active = -1;
    }

    function highlight(i) {
      var items = list.querySelectorAll('[role="option"]');
      if (!items.length) return;
      active = (i + items.length) % items.length;
      items.forEach(function (el, j) { el.setAttribute('aria-selected', String(j === active)); });
      input.setAttribute('aria-activedescendant', items[active].id);
      items[active].scrollIntoView({ block: 'nearest' });
    }

    function render(q) {
      q = q.trim().toLowerCase();
      if (!q) { close(); return; }
      load().then(function (all) {
        if (input.value.trim().toLowerCase() !== q) return;
        var hits = all.filter(function (e) {
          return (e.title + ' ' + e.page).toLowerCase().indexOf(q) !== -1;
        }).slice(0, 12);
        list.textContent = '';
        if (!hits.length) {
          var empty = document.createElement('li');
          empty.className = 'search__empty';
          empty.textContent = 'No matches';
          list.appendChild(empty);
        }
        hits.forEach(function (e, i) {
          var li = document.createElement('li');
          li.className = 'search__result';
          li.id = 'search-opt-' + i;
          li.setAttribute('role', 'option');
          li.setAttribute('aria-selected', 'false');
          var a = document.createElement('a');
          a.href = e.url;
          a.tabIndex = -1;
          var t = document.createElement('span');
          t.textContent = e.title;
          a.appendChild(t);
          if (e.page) {
            var p = document.createElement('span');
            p.className = 'search__result-page';
            p.textContent = e.page;
            a.appendChild(p);
          }
          li.appendChild(a);
          list.appendChild(li);
        });
        active = -1;
        list.hidden = false;
        input.setAttribute('aria-expanded', 'true');
        if (hits.length) highlight(0);
      });
    }

    input.addEventListener('focus', function () { load(); });
    input.addEventListener('input', function () { render(input.value); });
    input.addEventListener('keydown', function (e) {
      if (e.key === 'ArrowDown') { e.preventDefault(); highlight(active + 1); }
      else if (e.key === 'ArrowUp') { e.preventDefault(); highlight(active - 1); }
      else if (e.key === 'Escape') { close(); input.blur(); }
      else if (e.key === 'Enter') {
        var opt = list.querySelectorAll('[role="option"] a')[Math.max(active, 0)];
        if (opt && !list.hidden) { e.preventDefault(); location.href = opt.href; close(); }
      }
    });
    document.addEventListener('click', function (e) {
      if (!box.contains(e.target)) close();
    });

    // ⌘K / Ctrl+K focuses the search box from anywhere on the page.
    document.addEventListener('keydown', function (e) {
      if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault();
        input.focus();
        input.select();
      }
    });

    // Show Ctrl rather than ⌘ off Apple platforms.
    var kbd = box.querySelector('.search__kbd');
    var platform = (navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || '';
    if (kbd && !/mac|iphone|ipad/i.test(platform)) kbd.textContent = 'Ctrl K';
  })();
})();
