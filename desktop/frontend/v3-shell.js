'use strict';

(() => {
  const {t} = window.nawaV2;
  const originalHome = home;
  const originalLibrary = library;
  const originalNav = nav;
  let deck = false;
  let queuePinned = false;
  let touchStart = null;
  let searchTimer = 0;

  const primary = ['home', 'search', 'library', 'lyrics'];
  const libraryRoutes = ['library', 'albums', 'collection', 'library-health', 'offline'];

  nav = function () {
    originalNav();
    const menu = $('#navigation');
    if (!menu) return;
    menu.classList.add('v3-navigation');
    const selected = libraryRoutes.includes(view) ? 'library' : view === 'studio' ? 'lyrics' : view;
    for (const button of menu.querySelectorAll('[data-nav]')) {
      const isPrimary = primary.includes(button.dataset.nav);
      button.classList.toggle('v3-nav-extra', !isPrimary);
      if (!isPrimary) continue;
      const active = button.dataset.nav === selected;
      button.classList.toggle('active', active);
      if (active) button.setAttribute('aria-current', 'page');
      else button.removeAttribute('aria-current');
      if (button.dataset.nav === 'search') button.querySelector('span').textContent = t('البحث', 'Search');
    }
    menu.insertAdjacentHTML('beforeend', `<button type="button" data-v3="more" class="${primary.includes(selected) ? '' : 'active'}">
      ${icon('settings')}<span>${t('المزيد', 'More')}</span>
    </button>`);
  };

  function libraryTabs() {
    const tabs = [
      ['library', 'المسارات والجينرا', 'Tracks and genres'],
      ['albums', 'الألبومات', 'Albums'],
      ['collection', 'المجموعات', 'Collections'],
      ['library-health', 'تنظيم المكتبة', 'Library health'],
      ['offline', 'أوفلاين', 'Offline']
    ];
    return `<nav class="v3-library-tabs" aria-label="${t('أقسام المكتبة', 'Library sections')}">
      ${tabs.map(([id, ar, en]) => `<button class="quiet ${view === id ? 'active' : ''}" data-nav="${id}" ${view === id ? 'aria-current="page"' : ''}>${t(ar, en)}</button>`).join('')}
    </nav>`;
  }

  home = function () {
    const prior = originalHome();
    if (!window.nawaV2.workspace) return prior;
    const session = window.nawaV2.workspace.playback;
    const last = session && (!ui.homeSections || ui.homeSections.includes('continue')) && taskByID(session.taskId);
    const artButton = last ? art(last.song, last.id) : '<span class="v3-empty-cover">◉</span>';
    const sections = new DOMParser().parseFromString(prior, 'text/html');
    const shelves = [...sections.querySelectorAll('.v2-home-section')].map(node => node.outerHTML).join('');
    return `<div class="v3-home">
      <header class="v3-home-header">
        <h1>${t('الرئيسية', 'Home')}</h1>
        <div class="v3-home-actions">
          <button type="button" class="quiet" data-v3="search">${icon('search')}${t('بحث سريع', 'Quick search')}</button>
          <button type="button" class="quiet" data-v2="home-layout">${t('رتّب الرئيسية', 'Arrange home')}</button>
          <button type="button" class="primary" data-action="import">${icon('plus')}${t('أضف موسيقى', 'Add music')}</button>
        </div>
      </header>
      ${last ? `<section class="v3-resume">
        ${artButton}
        <div><small>${t('كمّل من مكانك', 'Continue listening')}</small><h2 dir="auto">${esc(last.song.title)}</h2><p dir="auto">${esc(last.song.artist)}</p></div>
        <button type="button" class="primary" data-v2="resume">${icon('play')}${t('متابعة', 'Resume')} <bdi>${time(session.position)}</bdi></button>
      </section>` : ''}
      ${recentlyPlayed()}
      ${shelves}
      ${!last && !shelves ? sections.querySelector('.empty')?.outerHTML || '' : ''}
    </div>`;
  };

  function recentlyPlayed() {
    const history = window.nawaV2.workspace?.listening || {};
    const items = Object.entries(history).sort((a, b) => b[1].lastPlayed - a[1].lastPlayed)
      .map(([id]) => taskByID(id)).filter(task => task?.status === 'done').slice(0, 8);
    if (!items.length) return '';
    return `<section class="v2-home-section">
      <div class="section-head"><h2>${t('آخر ما شغّلت', 'Recently played')}</h2><button class="quiet" data-nav="collection">${t('عرض المجموعات', 'View collections')}</button></div>
      <div class="v2-home-grid v3-home-grid">${items.map(albumCard).join('')}</div>
    </section>`;
  }

  library = function () {
    return libraryTabs() + originalLibrary();
  };

  function openDeck() {
    deck = true;
    navigate('lyrics');
  }

  function queuePanel() {
    const order = playOrder.length ? playOrder : current ? [current.id] : [];
    const start = Math.max(0, order.indexOf(current?.id));
    const tracks = order.slice(start, start + 15).map(taskByID).filter(Boolean);
    return `<aside class="v3-queue-panel"><h2>${t('طابور التشغيل', 'Play queue')}</h2>
      <div class="v3-queue-tracks">${tracks.map(track => `<button type="button" class="v3-queue-track ${track.id === current?.id ? 'active' : ''}" data-v3="deck-play" data-id="${esc(track.id)}">
        ${art(track.song, track.id)}
        <span><strong dir="auto">${esc(track.song.title)}</strong><small dir="auto">${esc(track.song.artist)}</small></span>
      </button>`).join('') || `<p>${t('الطابور فارغ', 'Queue is empty')}</p>`}</div>
      <button class="quiet" data-v2="queue">${t('إدارة الطابور', 'Manage queue')}</button>
    </aside>`;
  }

  function augmentLyrics() {
    if (view !== 'lyrics' || window.madarFullscreen?.active || !current) return;
    const layout = $('.lyrics-layout');
    if (!layout) return;
    if (deck) {
      layout.classList.add('v3-playback-deck');
      layout.classList.toggle('v3-queue-pinned', queuePinned);
      if (queuePinned) layout.insertAdjacentHTML('beforeend', queuePanel());
      $('.page-head h1').textContent = t('التشغيل الحالي', 'Now playing');
    }
    $('.lyrics-controls')?.insertAdjacentHTML('beforeend', `
      <button type="button" class="quiet" data-v3="toggle-queue" aria-pressed="${queuePinned}">${t('الطابور', 'Queue')}</button>
      <button type="button" class="quiet" data-v3-stage="cues">${t('المواضع', 'Cues')}</button>`);
  }

  function menu() {
    const links = [
      ['player','استوديو الصوت','Sound studio','music'],
      ['studio','استوديو الكلمات','Lyrics studio','lyrics'],
      ['downloads','التنزيلات','Downloads','download'],
      ['devices','الأجهزة','Devices','link'],
      ['offline','قوائم الأوفلاين','Offline collections','download'],
      ['appearance','المظهر والحركة','Appearance','theme'],
      ['settings','الإعدادات','Settings','settings']
    ];
    modal(`<h2>${t('المزيد', 'More')}</h2><div class="v3-more-grid">
      ${links.map(([id, ar, en, i]) => `<button type="button" data-v3="navigate" data-id="${id}">${icon(i)}${t(ar,en)}</button>`).join('')}
    </div>`);
  }

  function quickSearch() {
    modal(`<h2>${t('بحث سريع', 'Quick search')}</h2>
      <form id="v3-search-form" class="v3-search-form">
        <input id="v3-search-query" placeholder="${t('أغنية، ألبوم، فنان، أو رابط', 'Track, album, artist, or link')}" dir="auto" type="search" autocomplete="off" aria-label="${t('بحث سريع', 'Quick search')}">
        <button type="submit" class="primary">${t('بحث في المصادر', 'Search sources')}</button>
      </form><div id="v3-search-results"></div>`);
    $('#v3-search-query')?.focus();
  }

  function searchLocal() {
    const result = $('#v3-search-results');
    const query = $('#v3-search-query')?.value.trim().toLocaleLowerCase() || '';
    if (!result || !query) {
      if (result) result.textContent = '';
      return;
    }
    const matched = (value) => String(value || '').toLocaleLowerCase().includes(query);
    const tracks = [];
    const artists = new Map();
    const albums = new Map();
    for (const track of done()) {
      const {song} = track;
      if (tracks.length < 8 && matched(song.title)) tracks.push(track);
      if (artists.size < 4 && matched(song.artist)) artists.set(song.artist, track);
      if (albums.size < 4 && matched(song.album)) albums.set(JSON.stringify([song.artist, song.album]), track);
    }
    function section(name, rows) {
      return rows.length ? `<section class="v3-search-results"><h3>${name}</h3>${rows.join('')}</section>` : '';
    }
    const button = (track, type, title, subtitle = '') => `<button type="button" data-v3="quick-result" data-kind="${type}" data-id="${esc(track.id)}" dir="auto">
      <span>${esc(title)}</span><small>${esc(subtitle)}</small>
    </button>`;
    result.innerHTML = section(t('أغانٍ', 'Tracks'), tracks.map(track => button(track, 'track', track.song.title, track.song.artist))) +
      section(t('ألبومات', 'Albums'), [...albums.values()].map(track => button(track, 'album', track.song.album, track.song.artist))) +
      section(t('فنانون', 'Artists'), [...artists.values()].map(track => button(track, 'artist', track.song.artist))) ||
      `<p class="muted">${t('ما لقينا نتيجة في مكتبتك.', 'No matches in your library.')}</p>`;
  }

  function quickResult(button) {
    const track = taskByID(button.dataset.id);
    if (!track) return;
    $('#modal').close();
    if (button.dataset.kind === 'track') return playTask(track.id);
    navigate('library');
    ui.libraryMode = 'tracks';
    term = button.dataset.kind === 'album' ? track.song.album : track.song.artist;
    render();
  }

  function afterRender() {
    document.body.classList.add('nawa-v3');
    if (view !== 'lyrics') deck = false;
    if (['albums','collection','library-health','offline'].includes(view) && !$('.v3-library-tabs')) {
      $('#main').insertAdjacentHTML('afterbegin', libraryTabs());
    }
    const titlebar = $('.titlebar');
    if (titlebar && !$('.v3-title-search', titlebar)) {
      titlebar.insertAdjacentHTML('beforeend', `<button class="quiet v3-title-search" data-v3="search" aria-label="${t('بحث سريع', 'Quick search')}">${icon('search')}<span>${t('بحث', 'Search')}</span><kbd>Ctrl+K</kbd></button>`);
    }
    const search = $('.v3-title-search');
    if (search) {
      search.setAttribute('aria-label', t('بحث سريع', 'Quick search'));
      search.querySelector('span').textContent = t('بحث', 'Search');
    }
    const artButton = $('#now-art');
    if (artButton) artButton.onclick = openDeck;
    augmentLyrics();
    window.nawaV3Stage?.mount();
    window.nawaV3Tools?.mount();
    window.nawaV3Devices?.mount();
  }

  document.addEventListener('click', async event => {
    const button = event.target.closest('[data-v3]');
    if (!button) return;
    try {
      switch (button.dataset.v3) {
        case 'more': menu(); break;
        case 'search': quickSearch(); break;
        case 'navigate': $('#modal').close(); navigate(button.dataset.id); break;
        case 'toggle-queue': deck = true; queuePinned = !queuePinned; render(); break;
        case 'deck-play': await playTask(button.dataset.id, playOrder); break;
        case 'quick-result': await quickResult(button); break;
      }
    } catch (error) {
      toast(error.message || String(error));
    }
  });
  document.addEventListener('input', event => {
    if (event.target.id !== 'v3-search-query') return;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(searchLocal, 90);
  });
  document.addEventListener('submit', event => {
    if (event.target.id !== 'v3-search-form') return;
    event.preventDefault();
    const query = $('#v3-search-query').value.trim();
    if (!query) return;
    $('#modal').close();
    runSearch(query).catch(error => toast(error.message));
  });
  document.addEventListener('keydown', event => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault();
      quickSearch();
    }
  });
  $('#transport').addEventListener('touchstart', event => {
    if (!event.target.closest('input')) touchStart = event.touches[0]?.clientY;
  }, {passive: true});
  $('#transport').addEventListener('touchend', event => {
    if (touchStart != null && touchStart - event.changedTouches[0]?.clientY > 46) openDeck();
    touchStart = null;
  }, {passive: true});

  window.nawaV3 = {afterRender, libraryTabs, openDeck};
  nav();
  render();
})();
