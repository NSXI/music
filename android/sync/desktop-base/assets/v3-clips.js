'use strict';

(() => {
  const {t} = window.nawaV2;
  let preparing = false;
  let cancelled = false;
  let activeClip = '';
  let sample = null;
  let pollTimer = null;

  function coverImage(task) {
    return new Promise(resolve => {
      const url = cover(task.song, task.id);
      if (!url) return resolve(null);
      const image = new Image();
      const timeout = setTimeout(() => resolve(null), 3000);
      image.crossOrigin = 'anonymous';
      image.onload = () => {clearTimeout(timeout); resolve(image)};
      image.onerror = () => {clearTimeout(timeout); resolve(null)};
      image.src = url;
    });
  }

  function coverRect(context, image, x, y, width, height) {
    const factor = Math.max(width / image.width, height / image.height);
    const sourceW = width / factor;
    const sourceH = height / factor;
    context.drawImage(image, (image.width - sourceW) / 2, (image.height - sourceH) / 2, sourceW, sourceH, x, y, width, height);
  }

  function lineWrap(context, text, width) {
    const rows = [];
    let current = '';
    for (const word of String(text).trim().split(/\s+/u)) {
      if (!word) continue;
      if (current && context.measureText(current + ' ' + word).width > width) {
        rows.push(current);
        current = word;
      } else current = current ? current + ' ' + word : word;
    }
    if (current) rows.push(current);
    return rows;
  }

  function drawText(context, text, x, y, width, size, family, color, weight = 700, maxLines = 4) {
    if (!text) return y;
    const font = String(family || 'system-ui').replace(/["\\]/g, '');
    context.font = `${weight} ${size}px "${font}", system-ui`;
    context.direction = textDirection(text);
    context.fillStyle = color;
    context.textAlign = 'center';
    const lines = lineWrap(context, text, width).slice(0, maxLines);
    for (const line of lines) {
      context.fillText(line, x, y, width);
      y += size * 1.28;
    }
    return y;
  }

  function drawFrame(canvas, frame, item, vertical) {
    canvas.width = vertical ? 540 : 960;
    canvas.height = vertical ? 960 : 540;
    const {width: w, height: h} = canvas;
    const context = canvas.getContext('2d');
    const {task, image, design, accent, originalFont, translationFont} = item;
    context.fillStyle = design.background === 'theme' ? item.panel : '#08080b';
    context.fillRect(0, 0, w, h);
    if (image && ['cover','aura'].includes(design.background)) {
      context.save();
      context.filter = design.background === 'aura' ? 'blur(20px) saturate(110%)' : 'none';
      coverRect(context, image, 0, 0, w, h);
      context.restore();
      context.fillStyle = `rgba(0,0,0,${design.dim})`;
      context.fillRect(0, 0, w, h);
    }
    const sideCover = !vertical && design.layout === 'theater' && design.art && image;
    const textX = sideCover ? w * .67 : w * .5;
    const textWidth = sideCover ? w * .51 : w * .88;
    if (image && design.art) {
      const size = sideCover ? 245 : vertical ? 185 : 92;
      const x = sideCover ? 55 : (w - size) / 2;
      const y = sideCover ? (h - size) / 2 : 60;
      coverRect(context, image, x, y, size, size);
    }
    const originalSize = Math.max(22, Math.min(vertical ? 59 : 54, 39 * design.scale));
    const textY = vertical ? h * .51 : sideCover ? h * .36 : h * .42;
    context.save();
    if (design.glow) {context.shadowColor = accent; context.shadowBlur = 16}
    const nextY = drawText(context, frame.text || task.song.title, textX, textY, textWidth, originalSize, originalFont, '#ffffff');
    context.restore();
    let y = nextY + 16;
    if (frame.translation) y = drawText(context, frame.translation, textX, y, textWidth, originalSize * design.translation, translationFont, '#e4dce9', 500, 3) + 14;
    if (frame.word && y < h - 100) drawText(context, frame.word, textX, y + 3, textWidth, originalSize * .62, originalFont, accent, 700, 1);
    context.fillStyle = accent;
    context.fillRect(w * .1, h - 56, w * .8, 2);
    drawText(context, `${task.song.title} · ${task.song.artist}`, w / 2, h - 25, w * .8, 13, originalFont, '#dad7e1', 500, 1);
  }

  function frameAt(item, time) {
    const position = time + item.offset;
    const index = item.lines.findLastIndex(line => line.start <= position);
    if (index < 0) return {text: '', translation: '', word: ''};
    const line = item.lines[index];
    if (position >= line.end) return {text: '', translation: '', word: ''};
    const word = line.words?.find(word => word.start <= position && position < word.end);
    return {text: line.text, translation: item.translations?.[index]?.text || '', word: word?.text || ''};
  }

  function dimensions() {
    return $('#v3-clip-format')?.value !== 'wide';
  }

  function redraw() {
    const canvas = $('#v3-clip-preview');
    if (!canvas || !sample) return;
    const start = Number($('#v3-clip-start')?.value) || 0;
    drawFrame(canvas, frameAt(sample, start), sample, dimensions());
  }

  async function open() {
    if (!current || current.preview) throw Error(t('اختر أغنية محفوظة', 'Choose a saved track'));
    if (preparing) return;
    if (activeClip) return showClipStatus(true);
    const task = current;
    const doc = await call('GetLyricDocument', task.id);
    if (!doc.timed) throw Error(t('أضف توقيت الكلمات أولًا', 'Add lyric timing first'));
    const parsed = parseLyrics(doc.text, true);
    const lines = doc.lines?.length ? doc.lines : parsed.map((line, i) => ({
      text: line.text, start: line.time, end: parsed[i + 1]?.time ?? task.song.duration, words: []
    }));
    if (!lines.length) throw Error(t('ما فيه كلمات موقّتة للتصدير', 'There are no timed lyrics to export'));
    const [image, translation] = await Promise.all([
      coverImage(task),
      ui.translationEnabled ? call('GetTranslation', task.id, ui.translationTarget, ui.translationSource, false).catch(() => null) : null
    ]);
    if (current?.id !== task.id) return;
    const appearance = getComputedStyle(document.documentElement);
    sample = {
      task, image, lines, translations: translation?.exists ? translation.lines : [],
      design: window.nawaV3Stage.settings(),
      accent: appearance.getPropertyValue('--v3-album-accent').trim() || '#dbbadf',
      panel: appearance.getPropertyValue('--panel-solid').trim() || '#171521',
      offset: Number(ui.lyricOffset || 0),
      originalFont: ui.lyricFont || '',
      translationFont: ui.translationFont || ''
    };
    const start = Math.max(0, Math.min(audio.currentTime, task.song.duration - 1));
    modal(`<h2>${t('تصدير فيديو الكلمات', 'Export lyric video')}</h2>
      <canvas id="v3-clip-preview" aria-label="${t('معاينة الفيديو', 'Video preview')}"></canvas>
      <div class="v3-dialog-actions">
        <label>${t('اتجاه الفيديو', 'Orientation')} <select id="v3-clip-format"><option value="vertical">${t('عمودي', 'Portrait')}</option><option value="wide">${t('أفقي', 'Landscape')}</option></select></label>
        <label>${t('من ثانية', 'Start second')} <input type="number" id="v3-clip-start" step=".1" min="0" value="${start.toFixed(1)}"></label>
        <label>${t('إلى ثانية', 'End second')} <input type="number" id="v3-clip-end" step=".1" min="1" value="${Math.min(task.song.duration, start + 15).toFixed(1)}"></label>
        <button class="primary" data-v3-clip="start">${t('أنشئ فيديو MP4', 'Create MP4 video')}</button>
      </div>`);
    redraw();
  }

  function frameBoundaries(item, start, end) {
    const points = [start, end];
    for (const line of item.lines) {
      const events = [line.start, line.end, ...(line.words || []).flatMap(word => [word.start, word.end])];
      for (const event of events) {
        const position = event - item.offset;
        if (position > start && position < end) points.push(position);
      }
    }
    points.sort((a, b) => a - b);
    const spaced = points.filter((value, index) => index === 0 || index === points.length - 1 || value - points[index - 1] >= .033);
    if (spaced.length <= 301) return spaced;
    return spaced.filter((_, index) => index === spaced.length - 1 || index % Math.ceil(spaced.length / 300) === 0);
  }

  async function create() {
    if (!sample || preparing || activeClip) return;
    const item = sample;
    const start = Number($('#v3-clip-start').value);
    const end = Number($('#v3-clip-end').value);
    const vertical = dimensions();
    cancelled = false;
    preparing = true;
    try {
      activeClip = await call('BeginLyricClip', item.task.id, start, end, vertical);
      const canvas = document.createElement('canvas');
      modal(`<h2>${t('يجري تجهيز فيديو الكلمات', 'Preparing lyric video')}</h2><p id="v3-clip-status" role="status"></p><button class="quiet" data-v3-clip="cancel">${t('إلغاء', 'Cancel')}</button>`);
      const points = frameBoundaries(item, start, end);
      for (let i = 0; i < points.length - 1 && !cancelled; i++) {
        drawFrame(canvas, frameAt(item, points[i] + .001), item, vertical);
        await call('AddLyricClipFrame', activeClip, points[i + 1] - points[i], canvas.toDataURL('image/png').split(',')[1]);
        if ($('#v3-clip-status')) $('#v3-clip-status').textContent = `${i + 1} / ${points.length - 1}`;
      }
      if (!cancelled) {
        await call('FinishLyricClip', activeClip);
        pollTimer = setInterval(() => showClipStatus().catch(error => toast(error.message)), 1800);
      }
    } catch (error) {
      if (activeClip) await call('CancelLyricClip').catch(() => {});
      activeClip = '';
      throw error;
    } finally {
      preparing = false;
    }
  }

  async function showClipStatus(force = false) {
    if (!activeClip || preparing) return;
    const status = await call('GetLyricClipStatus');
    if (status.running) {
      if ($('#v3-clip-status')) $('#v3-clip-status').textContent = t('جارٍ ترميز الفيديو…', 'Encoding video…');
      return;
    }
    clearInterval(pollTimer);
    pollTimer = null;
    if (status.ready && (force || $('#v3-clip-status'))) {
      modal(`<h2>${t('فيديو الكلمات جاهز', 'Lyric video is ready')}</h2><button class="primary" data-v3-clip="save">${t('احفظ MP4', 'Save MP4')}</button>`);
    } else if (status.ready) {
      toast(t('فيديو الكلمات جاهز للحفظ', 'Lyric video is ready to save'));
    } else {
      activeClip = '';
      if (status.error && !cancelled) toast(status.error);
    }
  }

  document.addEventListener('input', event => {
    if (['v3-clip-format','v3-clip-start'].includes(event.target.id)) redraw();
  });
  document.addEventListener('change', event => {
    if (event.target.id === 'v3-clip-format') redraw();
  });
  document.addEventListener('click', async event => {
    const button = event.target.closest('[data-v3-clip]');
    if (!button || button.disabled) return;
    button.disabled = true;
    try {
      switch (button.dataset.v3Clip) {
        case 'open': await open(); break;
        case 'start': await create(); break;
        case 'save': {
          const saved = await call('SaveLyricClip', activeClip);
          if (saved) {activeClip = ''; $('#modal').close(); toast(t('تم حفظ الفيديو', 'Video saved'))}
          break;
        }
        case 'cancel':
          cancelled = true;
          clearInterval(pollTimer);
          pollTimer = null;
          await call('CancelLyricClip');
          activeClip = '';
          $('#modal').close();
          break;
      }
    } catch (error) {
      if (!cancelled) toast(error.message || String(error));
    } finally {
      if (button.isConnected) button.disabled = false;
    }
  });
})();
