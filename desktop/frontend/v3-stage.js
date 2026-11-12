'use strict';

(() => {
  const {t} = window.nawaV2;
  const layouts = [
    ['theater', 'المسرح', 'Theater'],
    ['focus', 'تركيز', 'Focus'],
    ['dual', 'لغتان', 'Two languages'],
    ['karaoke', 'كارايوكي', 'Karaoke']
  ];
  const backgrounds = [
    ['oled', 'أسود OLED', 'OLED black'],
    ['cover', 'غلاف الألبوم', 'Album cover'],
    ['aura', 'هالة الغلاف', 'Cover aura'],
    ['theme', 'الثيم الحالي', 'Current theme']
  ];
  const defaults = {layout: 'auto', background: 'oled', scale: 1, translation: .56, dim: .68, art: true, glow: true, density: 'surround', font: '', translationFont: '', motion: '', reveal: ''};
  const reduced = matchMedia('(prefers-reduced-motion: reduce)');
  let preview = null;
  let stageNode = null;
  let activeDesign = null;
  let accentRequest = 0;
  let accentTrack = '';
  let manualScroll = false;
  let range = null;
  let rangeStart = null;
  let loopPending = false;
  let loopSeekAt = 0;
  let active = -1;
  let countdown = '';
  let lastTrack = '';
  let dockTouch = null;

  function settings() {
    const value = ui.v3Stage || {};
    const album = albumKey();
    return {...defaults, ...value.global, ...value.albums?.[album], ...value.tracks?.[current?.id], ...preview};
  }

  function albumKey() {
    const song = current?.song;
    return song?.album ? JSON.stringify([song.artist, song.album]) : '';
  }

  function currentLayout(config) {
    if (config.layout !== 'auto') return config.layout;
    return innerWidth >= 700 ? 'theater' : 'focus';
  }

  function controls() {
    return `<div class="v3-stage-actions">
      <button type="button" class="v3-stage-icon" data-v3-stage="exit" aria-label="${t('الخروج من ملء الشاشة', 'Exit fullscreen')}">↙</button>
      <button type="button" class="v3-stage-icon" data-v3-stage="design" aria-label="${t('تصميم شاشة الكلمات', 'Lyrics screen design')}">${icon('settings')}</button>
    </div>
    <div class="v3-stage-bottom">
      <span class="v3-gap" aria-hidden="true"></span>
      <div class="v3-stage-tools">
        <button type="button" class="quiet" data-v3-stage="follow" hidden>${t('ارجع للسطر الحالي', 'Back to current line')}</button>
        <button type="button" class="quiet" data-v3-stage="range" aria-pressed="false">${t('تكرار مقطع', 'Repeat section')}</button>
        <button type="button" class="quiet" data-v3-stage="mark">${t('حفظ موضع', 'Save cue')}</button>
        <button type="button" class="quiet" data-v3-stage="cues">${t('المواضع', 'Cues')}</button>
      </div>
    </div>`;
  }

  function mount() {
    const scene = $('.fs-scene');
    if (!scene || !window.madarFullscreen.active) {
      stageNode = null;
      activeDesign = null;
      document.body.dataset.lyricReveal = ui.lyricReveal;
      return;
    }
    if (lastTrack !== current?.id) {
      lastTrack = current?.id || '';
      manualScroll = false;
      rangeStart = null;
      active = -1;
    }
    if (stageNode !== scene) {
      stageNode = scene;
      scene.classList.add('v3-stage');
      scene.insertAdjacentHTML('beforeend', controls());
      if (current) scene.insertAdjacentHTML('beforeend', `<aside class="v3-stage-cover">${art(current.song, current.id)}<strong dir="auto">${esc(current.song.title)}</strong><small dir="auto">${esc(current.song.artist)}</small></aside>`);
      const dock = $('#fs-dock-content');
      dock?.insertAdjacentHTML('beforeend', `<div class="v3-stage-dock-tools">
        <button class="quiet" type="button" data-v2="queue">${t('طابور التشغيل', 'Play queue')}</button>
        <button class="quiet" type="button" data-v3-stage="design">${t('تصميم العرض', 'Design')}</button>
        <button class="quiet" type="button" data-v3-stage="pin">${t('تثبيت المشغّل', 'Pin player')}</button>
      </div>`);
      const stage = $('#lyric-stage');
      stage?.addEventListener('wheel', suspendFollow, {passive: true});
      stage?.addEventListener('touchmove', suspendFollow, {passive: true});
      const handle = $('#fs-dock-toggle');
      handle?.addEventListener('pointerdown', event => {
        if (event.pointerType === 'mouse') return;
        dockTouch = event.clientX;
        handle.setPointerCapture?.(event.pointerId);
      });
      handle?.addEventListener('pointerup', event => {
        if (dockTouch !== null && event.clientX - dockTouch > 26) {
          window.madarFullscreen.setDockPinned(true);
        }
        dockTouch = null;
      });
      coverAccent(scene);
      window.madarFullscreen.setDockPinned(window.madarFullscreen.dockPinned());
    }
    apply();
    reflectRange();
    tick(audio.currentTime, lyricActive);
  }

  function apply() {
    if (!stageNode) return;
    const config = activeDesign = settings();
    const previousLayout = stageNode.dataset.layout;
    stageNode.dataset.layout = currentLayout(config);
    stageNode.dataset.backdrop = config.background;
    stageNode.dataset.density = config.density;
    stageNode.dataset.art = String(config.art);
    stageNode.dataset.glow = String(config.glow);
    stageNode.style.setProperty('--v3-scale', config.scale);
    stageNode.style.setProperty('--v3-translation', config.translation);
    stageNode.style.setProperty('--v3-dim', config.dim);
    stageNode.querySelector('[data-v3-stage="follow"]').hidden = !manualScroll;
    const stage = $('#lyric-stage');
    if (stage) {
      stage.dataset.manualFollow = String(manualScroll);
      stage.dataset.motion = config.motion || ui.lyricMotion;
    }
    document.body.dataset.lyricReveal = config.reveal || ui.lyricReveal;
    applyFonts(stageNode, config);
    if (previousLayout !== stageNode.dataset.layout) window.madarVerci?.sync();
  }

  function applyFonts(node, config) {
    if (!node) return;
    for (const [property, font] of [['--lyric-font', config.font], ['--translation-font', config.translationFont]]) {
      if (font) node.style.setProperty(property, `"${font.replace(/["\\\r\n]/g, '')}", sans-serif`);
      else node.style.removeProperty(property);
    }
  }

  function coverAccent(scene) {
    const track = current?.id;
    const url = current && cover(current.song, track);
    const request = ++accentRequest;
    if (!url) return;
    const source = new Image();
    source.crossOrigin = 'anonymous';
    source.onload = () => {
      if (request !== accentRequest || current?.id !== track || !scene.isConnected) return;
      try {
        const canvas = document.createElement('canvas');
        canvas.width = canvas.height = 12;
        const context = canvas.getContext('2d', {willReadFrequently: true});
        context.drawImage(source, 0, 0, 12, 12);
        const data = context.getImageData(0, 0, 12, 12).data;
        const weights = [0, 0, 0];
        let count = 0;
        for (let i = 0; i < data.length; i += 4) {
          const rgb = [data[i], data[i + 1], data[i + 2]];
          const light = rgb.reduce((sum, value) => sum + value, 0) / 3;
          const weight = data[i + 3] > 180 && light > 25 && light < 240 ? 10 + Math.max(...rgb) - Math.min(...rgb) : 0;
          for (let j = 0; j < 3; j++) weights[j] += rgb[j] * weight;
          count += weight;
        }
        if (count) {
          const color = `rgb(${weights.map(value => Math.round(138 + 117 * value / count / 255)).join(' ')})`;
          scene.style.setProperty('--v3-cover-accent', color);
          if (accentTrack !== track) document.documentElement.style.setProperty('--v3-album-accent', color);
          accentTrack = track;
        }
      } catch { /* Cross-origin artwork may block pixel access. */ }
    };
    source.src = url;
  }

  function suspendFollow() {
    if (!stageNode || !lyricTimed || $('#lyric-stage')?.classList.contains('verci-renderer')) return;
    if (!manualScroll) {
      manualScroll = true;
      apply();
    }
  }

  function follow() {
    manualScroll = false;
    apply();
    const row = $('#lyric-stage .lyric-line.current');
    if (row) row.scrollIntoView({block: 'center', behavior: ui.motion && !reduced.matches ? 'smooth' : 'instant'});
  }

  function validSeek() {
    return current && duration() > 0 && !(current.preview && current.decoded);
  }

  function seek(position) {
    if (!validSeek() || !Number.isFinite(position)) return;
    audio.currentTime = Math.min(duration(), Math.max(0, position));
  }

  function selectLine(event) {
    if (rangeStart === null || !stageNode) return;
    const row = event.target.closest('[data-lyric-index][data-lyric-seek]');
    if (!row || !validSeek()) return;
    event.stopImmediatePropagation();
    event.preventDefault();
    const index = Number(row.dataset.lyricIndex);
    if (rangeStart === -1) {
      rangeStart = index;
      reflectRange();
      toast(t('اختر آخر سطر في المقطع', 'Choose the last line'));
      return;
    }
    const first = Math.min(index, rangeStart);
    const last = Math.max(index, rangeStart);
    const offset = Number(ui.lyricOffset || 0);
    const start = Math.max(0, Number(lyricData[first]?.time) - offset);
    const end = Math.min(duration(), lyricData[last + 1] ? lyricData[last + 1].time - offset : duration());
    rangeStart = null;
    if (end - start < .25) {
      toast(t('اختر مقطعًا أطول', 'Choose a longer section'));
      reflectRange();
      return;
    }
    range = {id: current.id, start, end};
    reflectRange();
    seek(start);
  }

  function reflectRange() {
    const button = stageNode?.querySelector('[data-v3-stage="range"]');
    if (!button) return;
    const repeating = range?.id === current?.id;
    button.setAttribute('aria-pressed', String(repeating || rangeStart !== null));
    button.textContent = repeating ? t('إيقاف تكرار المقطع', 'Stop section repeat') :
      rangeStart !== null ? t('إلغاء تحديد المقطع', 'Cancel selection') : t('تكرار مقطع', 'Repeat section');
    for (const row of $$('#lyric-stage [data-lyric-index]')) {
      row.classList.toggle('v3-range-start', Number(row.dataset.lyricIndex) === rangeStart);
    }
  }

  function tick(position, index) {
    if (range && range.id !== current?.id) range = null;
    if (range && !audio.paused && !audio.seeking && position >= range.end && !loopPending) {
      loopPending = true;
      loopSeekAt = performance.now();
      seek(range.start);
    }
    if (loopPending && (!range || position < range.end - .1 || performance.now() - loopSeekAt > 1000)) loopPending = false;
    if (!stageNode || !stageNode.isConnected || !lyricTimed) return;
    if (index !== active) {
      active = index;
      stageNode.dataset.line = String(index);
    }
    const next = lyricData[index + 1];
    const aligned = lyricWordLines[index];
    const positionInLyrics = position + Number(ui.lyricOffset || 0);
    const seconds = next ? next.time - positionInLyrics : 0;
    const inGap = index < 0 || (Number.isFinite(aligned?.end) && aligned.end < positionInLyrics);
    const upcoming = stageNode.dataset.layout === 'karaoke' && inGap && seconds > 0 && seconds <= 5 ? String(Math.ceil(seconds)) : '';
    const counter = $('.v3-gap', stageNode);
    if (counter && (upcoming !== countdown || counter.textContent !== upcoming)) counter.textContent = upcoming;
    countdown = upcoming;
  }

  function options(values, value) {
    return values.map(([id, ar, en]) => `<option value="${id}" ${value === id ? 'selected' : ''}>${t(ar, en)}</option>`).join('');
  }

  function previewMarkup(config) {
    const row = $('#lyric-stage .lyric-line.current') || $('#lyric-stage .lyric-line');
    const index = Number(row?.dataset.lyricIndex);
    const source = Number.isFinite(index) ? lyricData[index]?.text : '';
    const translated = Number.isFinite(index) ? translationData?.lines?.[index]?.text : '';
    return `<div class="v3-design-preview" data-preview-background="${config.background}" data-preview-layout="${currentLayout(config)}" data-preview-glow="${config.glow}" style="--preview-size:${config.scale};--preview-translation:${config.translation};--preview-dim:${config.dim}">
      ${config.art && current ? art(current.song, current.id) : ''}
      <div class="v3-design-sample">
        <strong dir="auto">${esc(source || t('أول ضوء على الطريق', 'First light on the road'))}</strong>
        <span dir="auto">${esc(translated || t('First light on the road', 'أول ضوء على الطريق'))}</span>
      </div>
    </div>`;
  }

  async function design() {
    if (!installedFonts.length) {
      try { installedFonts = await call('GetFonts'); }
      catch { installedFonts = ['Segoe UI', 'Tahoma', 'Arial']; }
    }
    preview = settings();
    const fontOptions = value => `<option value="">${t('يتبع إعداد الكلمات', 'Follow lyrics settings')}</option>` +
      [...new Set([value, ...(window.madarGameThemes?.fonts || []), ...installedFonts].filter(Boolean))].map(font =>
        `<option value="${esc(font)}" ${value === font ? 'selected' : ''}>${esc(font)}</option>`).join('');
    modal(`<h2>${t('تصميم شاشة الكلمات', 'Lyrics screen design')}</h2>
      <div id="v3-design-preview">${previewMarkup(preview)}</div>
      <div class="v2-filter-grid v3-design-grid">
        <label>${t('العرض', 'Layout')}<select data-v3-design="layout">${options([['auto', 'حسب الجهاز', 'By device'], ...layouts], preview.layout)}</select></label>
        <label>${t('الخلفية', 'Background')}<select data-v3-design="background">${options(backgrounds, preview.background)}</select></label>
        <label>${t('الأسطر حول السطر الحالي', 'Context lines')}<select data-v3-design="density">${options([['surround','السابق والقادم','Previous and next'],['single','الحالي فقط','Current only']], preview.density)}</select></label>
        <label>${t('خط الكلمات الأصلية', 'Original lyrics font')}<select data-v3-design="font">${fontOptions(preview.font)}</select></label>
        <label>${t('خط الترجمة', 'Translation font')}<select data-v3-design="translationFont">${fontOptions(preview.translationFont)}</select></label>
        <label>${t('الحركة', 'Motion')}<select data-v3-design="motion"><option value="">${t('يتبع إعداد الكلمات', 'Follow lyrics settings')}</option>${lyricMotions.map(([id, label]) => `<option value="${esc(id)}" ${preview.motion === id ? 'selected' : ''}>${esc(label)}</option>`).join('')}</select></label>
        <label>${t('تدرّج الظهور', 'Reveal')}<select data-v3-design="reveal">${options([['','يتبع إعداد الكلمات','Follow lyrics settings'],['word','كلمة بكلمة','Word by word'],['character','حرف بحرف','Character by character'],['line','سطر كامل','Whole line']], preview.reveal)}</select></label>
        <label>${t('حجم الكلمات', 'Lyrics size')}<input type="range" min=".65" max="1.5" step=".05" data-v3-design="scale" value="${preview.scale}"></label>
        <label>${t('حجم الترجمة', 'Translation size')}<input type="range" min=".35" max="1" step=".05" data-v3-design="translation" value="${preview.translation}"></label>
        <label>${t('تعتيم الخلفية', 'Backdrop dim')}<input type="range" min=".35" max=".95" step=".05" data-v3-design="dim" value="${preview.dim}"></label>
        <label class="check-line"><input type="checkbox" data-v3-design="art" ${preview.art ? 'checked' : ''}>${t('إظهار الغلاف', 'Show cover')}</label>
        <label class="check-line"><input type="checkbox" data-v3-design="glow" ${preview.glow ? 'checked' : ''}>${t('إضاءة السطر النشط', 'Active line light')}</label>
      </div>
      <div class="v3-dialog-actions">
        <label>${t('طبّق على', 'Apply to')} <select id="v3-design-scope">${options([['global','كل الأغاني','All tracks'],...(albumKey() ? [['album','هذا الألبوم','This album']] : []),...(current && !current.preview ? [['track','هذه الأغنية','This track']] : [])], 'global')}</select></label>
        <button type="button" class="primary" data-v3-stage="save-design">${t('اعتماد التصميم', 'Apply design')}</button>
        <button type="button" class="quiet" data-v3-stage="reset-design">${t('استعادة الافتراضي', 'Reset to default')}</button>
        <button type="button" class="quiet" data-action="lyrics-settings">${t('الخطوط والحركة والترجمة', 'Fonts, motion and translation')}</button>
      </div>`);
    applyFonts($('.v3-design-preview'), preview);
  }

  function changeDesign(event) {
    const key = event.target.dataset.v3Design;
    if (!preview || !key || !Object.hasOwn(defaults, key)) return;
    const control = event.target;
    preview[key] = control.type === 'checkbox' ? control.checked : control.type === 'range' ? Number(control.value) : control.value;
    const sample = $('#v3-design-preview');
    if (sample) {
      sample.innerHTML = previewMarkup(preview);
      applyFonts($('.v3-design-preview'), preview);
    }
    apply();
    if (key === 'font' || key === 'translationFont') window.madarVerci?.invalidateFonts();
  }

  function saveDesign() {
    if (!preview) return;
    const scope = $('#v3-design-scope')?.value;
    ui.v3Stage ||= {};
    if (scope === 'track' && current && !current.preview) {
      ui.v3Stage.tracks ||= {};
      ui.v3Stage.tracks[current.id] = {...preview};
    } else if (scope === 'album' && albumKey()) {
      ui.v3Stage.albums ||= {};
      ui.v3Stage.albums[albumKey()] = {...preview};
    } else ui.v3Stage.global = {...preview};
    preview = null;
    persist();
    $('#modal').close();
    apply();
  }

  function cues() {
    return ui.v3Cues?.[current?.id] || [];
  }

  function listCues() {
    const items = cues();
    modal(`<h2>${t('المواضع المحفوظة', 'Saved cues')}</h2>
      ${items.length ? items.map((cue, index) => `<div class="v3-cue-row">
        <button type="button" data-v3-stage="jump" data-index="${index}" dir="auto">${esc(cue.name)} <bdi>${time(cue.at)}</bdi></button>
        <button type="button" class="quiet" data-v3-stage="delete-cue" data-index="${index}">${t('حذف', 'Delete')}</button>
      </div>`).join('') : `<p>${t('احفظ موضع الكورس أو أي لحظة في الأغنية.', 'Save the chorus or any moment in the song.')}</p>`}
      <button type="button" class="primary" data-v3-stage="mark">${t('حفظ الموضع الحالي', 'Save current position')}</button>`);
  }

  function mark() {
    if (!validSeek() || current.preview) return toast(t('شغّل أغنية محفوظة أولًا', 'Play a saved track first'));
    const at = audio.currentTime;
    modal(`<h2>${t('حفظ موضع', 'Save cue')} <bdi>${time(at)}</bdi></h2>
      <label for="v3-cue-name">${t('اسم الموضع', 'Cue name')}</label>
      <input id="v3-cue-name" maxlength="80" dir="auto" value="${t('الكورس', 'Chorus')}">
      <button type="button" class="primary" data-v3-stage="save-cue" data-id="${esc(current.id)}" data-at="${at}">${t('حفظ', 'Save')}</button>`);
  }

  function stageAction(event) {
    const button = event.target.closest('[data-v3-stage]');
    if (!button) return;
    switch (button.dataset.v3Stage) {
      case 'exit': window.madarFullscreen.leave(); break;
      case 'design': design(); break;
      case 'save-design': saveDesign(); break;
      case 'reset-design':
        preview = {...defaults};
        for (const control of $$('[data-v3-design]')) {
          if (control.type === 'checkbox') control.checked = preview[control.dataset.v3Design];
          else control.value = preview[control.dataset.v3Design];
        }
        $('#v3-design-preview').innerHTML = previewMarkup(preview);
        applyFonts($('.v3-design-preview'), preview);
        apply();
        break;
      case 'follow': follow(); break;
      case 'pin':
        window.madarFullscreen.setDockPinned(stageNode?.dataset.dockPinned !== 'true');
        break;
      case 'range':
        if (range?.id === current?.id || rangeStart !== null) {range = null; rangeStart = null}
        else if (lyricTimed && validSeek()) {
          rangeStart = -1;
          toast(t('اختر أول سطر في المقطع', 'Choose the first line'));
        } else toast(t('تحتاج إلى كلمات بتوقيت محفوظ', 'Timed lyrics are required'));
        reflectRange();
        break;
      case 'mark': mark(); break;
      case 'cues': listCues(); break;
      case 'jump': {
        const cue = cues()[Number(button.dataset.index)];
        if (cue) {seek(cue.at); $('#modal').close()}
        break;
      }
      case 'delete-cue':
        ui.v3Cues[current.id] = cues().filter((_, index) => index !== Number(button.dataset.index));
        persist();
        listCues();
        break;
      case 'save-cue': {
        const name = $('#v3-cue-name')?.value.trim();
        if (!name || current?.id !== button.dataset.id) return;
        ui.v3Cues ||= {};
        ui.v3Cues[current.id] = [...cues().slice(-23), {name, at: Number(button.dataset.at)}].sort((a, b) => a.at - b.at);
        persist();
        listCues();
        break;
      }
    }
  }

  document.addEventListener('click', selectLine, true);
  document.addEventListener('click', stageAction);
  document.addEventListener('input', changeDesign);
  document.addEventListener('change', changeDesign);
  $('#modal').addEventListener('close', () => {preview = null; apply()});
  audio.addEventListener('timeupdate', () => tick(audio.currentTime, lyricActive));
  window.addEventListener('resize', apply);
  window.nawaV3Stage = {mount, tick, follow, listCues, mark, seek, design, settings,
    activeSettings: () => window.madarFullscreen.active ? activeDesign : null,
    allowCanvas: () => !window.madarFullscreen.active || stageNode?.dataset.layout !== 'dual'};
})();
