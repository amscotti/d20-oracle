// The adventurer's identity stays live while HTMX replaces only the reading.
document.addEventListener('alpine:init', () => {
  Alpine.data('oracle', () => ({
    name: '',
    profession: '',
    get seen() {
      const n = this.name.trim();
      const p = this.profession.trim();
      if (n && p) return `${n} the ${p}`;
      return n || p || '';
    },
  }));
});

(() => {
  const results = document.getElementById('results');
  const status = document.getElementById('oracle-status');
  const form = document.getElementById('adventurer-form');
  const connectionError = document.getElementById('connection-error');
  const isReadingRequest = event => event.detail?.ctx?.target === results;

  function revealReading() {
    results.setAttribute('aria-busy', 'false');
    const heading = results.querySelector('#reading-title');
    if (!heading) return;

    heading.focus({ preventScroll: true });
    const bounds = heading.getBoundingClientRect();
    if (bounds.top < 24 || bounds.bottom > window.innerHeight - 24) {
      heading.closest('.reading').scrollIntoView({
        behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth',
        block: 'start',
      });
    }
  }

  document.addEventListener('htmx:before:request', event => {
    if (!isReadingRequest(event)) return;
    results.setAttribute('aria-busy', 'true');
    status.textContent = 'Rolling destiny and quirk. The oracle is choosing your class.';
  });

  document.addEventListener('htmx:after:swap', event => {
    if (!isReadingRequest(event)) return;
    const error = results.querySelector('.error-card .lore');
    status.textContent = error ? error.textContent.trim() : '';
    requestAnimationFrame(revealReading);
  });

  document.addEventListener('htmx:finally:request', event => {
    if (!isReadingRequest(event)) return;
    results.setAttribute('aria-busy', 'false');
    if (status.textContent.startsWith('Rolling')) status.textContent = '';
  });

  document.addEventListener('htmx:error', event => {
    if (!isReadingRequest(event)) return;
    results.replaceChildren(connectionError.content.cloneNode(true));
    status.textContent = 'The oracle lost the thread. Check your connection and try again.';
    requestAnimationFrame(revealReading);
  });

  document.addEventListener('click', event => {
    if (!event.target?.closest?.('[data-retry]')) return;
    form.requestSubmit();
  });
})();
