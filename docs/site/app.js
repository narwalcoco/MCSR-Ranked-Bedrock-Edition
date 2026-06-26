/* ============================================================
 * MCSR Ranked Bedrock — Guide site interactivity
 * ============================================================ */
(function () {
  'use strict';

  const qs  = (s, r = document) => r.querySelector(s);
  const qsa = (s, r = document) => Array.from(r.querySelectorAll(s));

  const STORAGE_KEY = 'mcsr-guide-theme';

  /* ---------- THEME ---------- */
  function setTheme(name) {
    document.documentElement.dataset.theme = name;
    try { localStorage.setItem(STORAGE_KEY, name); } catch (_) {}
    qsa('.theme-btn').forEach(b => {
      b.classList.toggle('is-active', b.dataset.themeSet === name);
      b.setAttribute('aria-pressed', String(b.dataset.themeSet === name));
    });
  }
  function initTheme() {
    let preferred;
    try { preferred = localStorage.getItem(STORAGE_KEY); } catch (_) {}
    if (!preferred) {
      const hour = new Date().getHours();
      preferred = (hour >= 7 && hour < 19) ? 'lab' : 'synthwave';
    }
    setTheme(preferred);
  }
  qsa('.theme-btn').forEach(btn => {
    btn.addEventListener('click', () => setTheme(btn.dataset.themeSet));
  });

  /* ---------- SCROLL PROGRESS ---------- */
  const progressBar = qs('#scrollProgress');
  function updateProgress() {
    const h = document.documentElement;
    const max = h.scrollHeight - h.clientHeight;
    const pct = max > 0 ? (h.scrollTop / max) * 100 : 0;
    if (progressBar) progressBar.style.width = pct + '%';
  }

  /* ---------- ACTIVE SECTION (nav underlining) ---------- */
  const sectionIds = ['hero', 'tools', 'bigpicture', 'architecture',
                       'filemap', 'walkthrough', 'phases', 'tryit', 'glossary'];
  const sections = sectionIds
    .map(id => qs('#' + id))
    .filter(Boolean);
  const navlinks = qsa('.topnav__links .navlink');
  const linkByHash = new Map(navlinks.map(a => [a.getAttribute('href').replace('#', ''), a]));

  function updateActiveLink() {
    const probe = window.scrollY + 120;
    let currentId = sectionIds[0];
    for (const sec of sections) {
      if (sec.offsetTop <= probe) currentId = sec.id;
    }
    navlinks.forEach(a => a.classList.remove('is-active'));
    const active = linkByHash.get(currentId);
    if (active) active.classList.add('is-active');
  }

  window.addEventListener('scroll', () => {
    updateProgress();
    updateActiveLink();
  }, { passive: true });

  /* ---------- REVEAL ON SCROLL ---------- */
  const revealTargets = [
    ...qsa('.hero__chip, .hero__title, .hero__lede, .hero__ctas, .hero__stats'),
    ...qsa('.section__head'),
    ...qsa('.toolcard'),
    ...qsa('.bigcard'),
    ...qsa('.legend-item'),
    ...qsa('.walk'),
    ...qsa('.phase'),
    ...qsa('.cmdcard'),
    ...qsa('.gloss'),
    ...qsa('.archdiagram, .filetree'),
  ];
  revealTargets.forEach(el => el.classList.add('reveal'));

  if ('IntersectionObserver' in window) {
    const io = new IntersectionObserver((entries) => {
      entries.forEach(e => {
        if (e.isIntersecting) {
          e.target.classList.add('is-visible');
          io.unobserve(e.target);
        }
      });
    }, { threshold: 0.12, rootMargin: '0px 0px -40px 0px' });
    revealTargets.forEach(el => io.observe(el));
  } else {
    revealTargets.forEach(el => el.classList.add('is-visible'));
  }

  /* ---------- SMOOTH SCROLL FOR ANCHOR LINKS ---------- */
  qsa('a[href^="#"]').forEach(a => {
    a.addEventListener('click', (ev) => {
      const id = a.getAttribute('href').slice(1);
      const target = qs('#' + id);
      if (!target) return;
      ev.preventDefault();
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
      history.replaceState(null, '', '#' + id);
    });
  });

  /* ---------- COPY BUTTONS ---------- */
  qsa('.copy-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const text = btn.dataset.copy || '';
      let ok = false;
      try {
        if (navigator.clipboard && window.isSecureContext) {
          await navigator.clipboard.writeText(text);
          ok = true;
        } else {
          const ta = document.createElement('textarea');
          ta.value = text;
          ta.style.position = 'fixed';
          ta.style.left = '-9999px';
          document.body.appendChild(ta);
          ta.select();
          ok = document.execCommand('copy');
          document.body.removeChild(ta);
        }
      } catch (_) { ok = false; }
      const original = btn.textContent;
      btn.textContent = ok ? 'Copied!' : 'Error';
      btn.classList.toggle('is-copied', ok);
      setTimeout(() => {
        btn.textContent = original;
        btn.classList.remove('is-copied');
      }, 1500);
    });
  });

  /* ---------- FILE TREE TOGGLES + CLICK-TO-WALK ---------- */
  // Toggle folder
  qsa('.ft-toggle').forEach(btn => {
    btn.addEventListener('click', (ev) => {
      ev.stopPropagation();
      const li = btn.closest('.ft-folder');
      if (!li) return;
      const expanded = li.getAttribute('aria-expanded') === 'true';
      li.setAttribute('aria-expanded', String(!expanded));
    });
  });

  // Map file paths to walkthrough section ids
  const FILE_TO_SECTION = [
    { match: /migrations\//,  target: 'sql-migrations' },
    { match: /cmd\/seedgen/,   target: 'cmd-seedgen' },
    { match: /pkg\/shared\/seeds/, target: 'pkg-shared-seeds' },
    { match: /pkg\/queen\/main\.go/, target: 'pkg-queen-main' },
    { match: /pkg\/queen\/internal\/config/, target: 'pkg-queen-config' },
    { match: /pkg\/queen\/internal\/db/,     target: 'pkg-queen-db' },
    { match: /pkg\/queen\/internal\/store/,  target: 'pkg-queen-store' },
    { match: /pkg\/queen\/internal\/matchmaker/, target: 'pkg-queen-matchmaker' },
    { match: /pkg\/queen\/internal\/api/,    target: 'pkg-queen-api' },
    { match: /pkg\/worker/,    target: 'pkg-worker-main' },
  ];

  qsa('.ft-detail').forEach(btn => {
    btn.addEventListener('click', (ev) => {
      ev.preventDefault();
      const file = btn.dataset.file || '';
      const map = FILE_TO_SECTION.find(m => m.match.test(file));
      if (!map) return;
      const target = qs('#' + map.target);
      if (!target) return;
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
      target.classList.add('is-flash');
      setTimeout(() => target.classList.remove('is-flash'), 1400);
    });
  });

  /* ---------- FILE MODAL SUMMARIES (for files without walktarget) ---------- */
  const FILE_SUMMARIES = {
    'pkg/shared/logging/logging.go': {
      title: 'pkg/shared/logging/logging.go',
      body: 'Houses the single shared Setup() function for Go’s slog structured logger. Each service calls it once at startup to give every log line the same shape (timestamp, level, service name, optional fields).'
    },
    'pkg/shared/version/version.go': {
      title: 'pkg/shared/version/version.go',
      body: 'Stamps every binary with its version, git commit, and build time at link time. Pure metadata — but vital for field debugging ("which build is actually deployed right now?").'
    },
    'migrations/0001_initial_schema.sql': {
      title: 'migrations/0001_initial_schema.sql',
      body: 'Creates the bookkeeping table (schema_version) the migration runner uses to know which migrations have already been applied.'
    },
    'pkg/queen/internal/store/store.go': {
      title: 'pkg/queen/internal/store/store.go',
      body: 'A wiring file: it constructs each repository (Players, Queue, Matches) and exposes them as a single struct so callers don’t have to assemble the pieces themselves.'
    }
  };

  function showFileModal(file) {
    let modal = qs('#file-detail');
    const summary = FILE_SUMMARIES[file];
    if (!summary) return false;
    if (!modal) {
      modal = document.createElement('aside');
      modal.id = 'file-detail';
      modal.className = 'file-modal';
      modal.setAttribute('role', 'dialog');
      modal.setAttribute('aria-modal', 'true');
      document.body.appendChild(modal);
    }
    modal.innerHTML = `
      <h4>${summary.title}</h4>
      <p>${summary.body}</p>
      <p style="color:var(--text-3); font-size:0.86rem;">Find details in the <a href="#walkthrough">Walkthrough</a> above for closely-related files.</p>
      <button class="close-modal">Close</button>
    `;
    modal.hidden = false;
    modal.querySelector('.close-modal').addEventListener('click', () => { modal.hidden = true; });
    modal.addEventListener('click', (ev) => { if (ev.target === modal) modal.hidden = true; }, { once: true });
    return true;
  }

  /* Intercept ft-detail clicks for files that DO NOT have a walktarget,
     and show a popup instead. */
  qsa('.ft-detail').forEach(btn => {
    btn.addEventListener('click', (ev) => {
      const file = btn.dataset.file || '';
      const map = FILE_TO_SECTION.find(m => m.match.test(file));
      if (!map) {
        ev.stopImmediatePropagation();
        ev.preventDefault();
        showFileModal(file);
      }
    }, true);
  });

  /* ---------- INITIAL STATE ---------- */
  initTheme();
  updateProgress();
  updateActiveLink();

})();
