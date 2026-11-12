'use strict';

(() => {
  const {t, pages} = window.nawaV2;
  let libraryReport = null;
  let healthFilter = 'groups';
  let healthLimit = 60;
  let healthQuery = '';
  let packs = [];
  let offline = {running: false};
  let offlineLoaded = false;
  let offlinePolling = false;
  let pendingPack = null;
  let mini = false;
  let sourceInfo = null;
  let studioReview = null;
  let lyricsNotes = null;

  function bytes(value) {
    if (value >= 1024 ** 3) return `${(value / 1024 ** 3).toFixed(2)} GB`;
    return `${(value / 1024 ** 2).toFixed(1)} MB`;
  }

  function healthContent() {
    if (!libraryReport) return `<p class="muted">${t('افحص المكتبة لعرض النسخ والأغلفة والكلمات.', 'Scan the library to review versions, covers and lyrics.')}</p>`;
    const query = healthQuery.toLocaleLowerCase();
    const matches = (track) => !query || `${track.song.title} ${track.song.artist} ${track.song.album}`.toLocaleLowerCase().includes(query);
    if (healthFilter === 'groups' || healthFilter === 'quality') {
      const groups = libraryReport.groups.filter(group => group.tracks.some(matches) &&
        (healthFilter !== 'quality' || group.reason === 'recording' && group.tracks.some(track => track.quality.lossless) && group.tracks.some(track => !track.quality.lossless)));
      return `<p>${groups.length} ${t('مجموعة', 'groups')}</p><div class="v3-panel-list">${groups.slice(0, healthLimit).map(group => `
        <details>
          <summary><bdi>${esc(group.tracks[0].song.title)}</bdi> <small>${group.tracks.length} ${t('نسخ', 'versions')} · ${group.reason === 'file' ? t('ملف متطابق', 'Identical file') : t('تسجيل محتمل مطابق', 'Possible matching recording')}</small></summary>
          ${group.tracks.map(track => `<div class="v3-cue-row"><button type="button" data-play="${esc(track.id)}" dir="auto">${esc(track.song.title)} · ${esc(track.song.artist)}</button><span>${badge(track.quality)}</span></div>`).join('')}
          <button class="quiet" data-v3-tool="fingerprint" data-tracks="${esc(group.tracks.slice(0,20).map(track => track.id).join(','))}">${t('قارن بصمة الصوت', 'Compare audio fingerprints')}</button>
        </details>`).join('')}</div>${groups.length > healthLimit ? showMore() : ''}`;
    }
    const tracks = libraryReport.tracks.filter(track => matches(track) && (
      healthFilter === 'cover' ? !track.cover : healthFilter === 'timing' ? track.timing === 'missing' || track.timing === 'plain' : track.missing
    ));
    return `<p>${tracks.length} ${t('مسار', 'tracks')}</p><div class="v3-panel-list">${tracks.slice(0, healthLimit).map(track => `
      <div class="v3-cue-row"><button type="button" data-play="${esc(track.id)}" dir="auto">${esc(track.song.title)} · ${esc(track.song.artist)}</button><span>${badge(track.quality)}</span></div>`).join('')}</div>${tracks.length > healthLimit ? showMore() : ''}`;
  }

  function showMore() {
    return `<button type="button" class="quiet" data-v3-tool="health-more">${t('عرض المزيد', 'Show more')}</button>`;
  }

  pages['library-health'] = () => heading(t('تنظيم المكتبة', 'Library health'), '', `
    <button class="primary" data-v3-tool="scan">${t('افحص المكتبة', 'Scan library')}</button>
    <button class="quiet" data-v3-tool="relink">${t('أعد ربط الملفات المنقولة', 'Relink moved files')}</button>`) +
    `<div class="filters">
      <select id="v3-health-filter" aria-label="${t('نوع الفحص', 'Review type')}">
        ${[['groups','النسخ المتكررة','Versions and duplicates'],['quality','لها نسخة أعلى جودة','Has a higher quality version'],['cover','ناقصها غلاف محلي','Missing local cover'],['timing','تحتاج توقيت كلمات','Needs lyric timing'],['missing','ملفات تحتاج ربط','Files needing relink']].map(([id, ar, en]) => `<option value="${id}" ${healthFilter === id ? 'selected' : ''}>${t(ar,en)}</option>`).join('')}
      </select>
      <input id="v3-health-query" type="search" value="${esc(healthQuery)}" placeholder="${t('فلتر بالاسم', 'Filter by name')}">
    </div><div id="v3-health-results">${healthContent()}</div>`;

  function updateHealth() {
    const result = $('#v3-health-results');
    if (result) result.innerHTML = healthContent();
  }

  function packOptions() {
    const lists = ui.playlists.map(list => [`playlist:${list.id}`, list.name]);
    if (current && !current.preview) {
      lists.unshift(['track', t('الأغنية الحالية', 'Current track')]);
      if (current.song.album) lists.unshift(['album', t('الألبوم الحالي', 'Current album')]);
    }
    return lists;
  }

  function packTracks(value) {
    if (value === 'track' && current) return [current.id];
    if (value === 'album' && current) {
      const song = current.song;
      return done().filter(task => task.song.album === song.album && task.song.artist === song.artist).map(task => task.id);
    }
    return ui.playlists.find(list => `playlist:${list.id}` === value)?.tracks || [];
  }

  function offlineProgress() {
    if (offline.running) {
      return `<progress max="${offline.total || 1}" value="${offline.completed || 0}"></progress>
        <span>${offline.completed || 0} / ${offline.total}</span>
        <button class="quiet" data-v3-tool="cancel-pack">${t('إلغاء التجهيز', 'Cancel preparation')}</button>`;
    }
    return offline.error ? `<p class="v2-error">${esc(offline.error)}</p>` : '';
  }

  function offlinePacks() {
    return packs.map(pack => {
      const saved = pack.items.filter(item => item.file);
      const failed = pack.items.filter(item => !item.file);
      return `<details>
        <summary><bdi>${esc(pack.name)}</bdi> <small>${saved.length}/${pack.items.length} · ${bytes(pack.bytes)}</small></summary>
        <div class="v3-dialog-actions">
          <button class="primary" data-v3-tool="play-pack" data-id="${esc(pack.id)}">${t('تشغيل الحزمة', 'Play pack')}</button>
          ${failed.length ? `<button class="quiet" data-v3-tool="retry-pack" data-id="${esc(pack.id)}">${t('أعد تجهيز الناقص', 'Prepare missing tracks again')}</button>` : ''}
          <button class="quiet" data-v3-tool="confirm-delete-pack" data-id="${esc(pack.id)}">${t('إزالة الحزمة', 'Remove pack')}</button>
        </div>
        <ul>${pack.items.map(item => `<li><bdi>${esc(item.title || item.id)}</bdi> <small>${item.error ? esc(item.error) : [item.quality.codec, item.cover ? t('الغلاف', 'cover') : '', item.lyrics ? t('الكلمات', 'lyrics') : '', item.translation ? t('الترجمة', 'translation') : ''].filter(Boolean).map(esc).join(' · ')}</small></li>`).join('')}</ul>
      </details>`;
    }).join('') || `<p class="muted">${t('جهّز قائمة أو ألبومًا عشان تشغّله أوفلاين.', 'Prepare a playlist or album for offline listening.')}</p>`;
  }

  pages.offline = () => heading(t('جاهز للأوفلاين', 'Ready offline'), t('تخزين الصوت ومرفقاته المحفوظة في حزمة على هذا الجهاز.', 'Store audio and its saved extras in a pack on this device.'), '') +
    `<div class="v3-dialog-actions">
      <label>${t('القائمة أو الألبوم', 'Playlist or album')} <select id="v3-offline-source">${packOptions().map(([id, name]) => `<option value="${esc(id)}">${esc(name)}</option>`).join('')}</select></label>
      <button class="primary" data-v3-tool="estimate-pack">${t('جهّز للأوفلاين', 'Prepare offline')}</button>
    </div><div id="v3-offline-progress" class="v3-dialog-actions" role="status">${offlineProgress()}</div><div id="v3-offline-list" class="v3-panel-list">${offlinePacks()}</div>`;

  async function refreshOffline() {
    if (offlinePolling || (!offline.running && view !== 'offline') || document.hidden) return;
    offlinePolling = true;
    try {
      const previous = offline.running;
      offline = await call('GetOfflineStatus');
      if (!offlineLoaded || previous && !offline.running) {
        packs = await call('ListOfflinePacks');
        offlineLoaded = true;
        if ($('#v3-offline-list')) $('#v3-offline-list').innerHTML = offlinePacks();
      }
      if ($('#v3-offline-progress')) $('#v3-offline-progress').innerHTML = offlineProgress();
    } finally {
      offlinePolling = false;
    }
  }

  async function estimatePack(ids, name) {
    const estimate = await call('EstimateOfflinePack', ids);
    pendingPack = {ids, name};
    modal(`<h2>${t('تجهيز حزمة أوفلاين', 'Prepare an offline pack')}</h2>
      <p>${t('حجم الصوت', 'Audio size')} <bdi>${bytes(estimate.audioBytes)}</bdi> · ${t('المستخدم على هذا الجهاز', 'Used on this device')} <bdi>${bytes(estimate.usedBytes)}</bdi></p>
      <p class="muted">${t('تُضاف الأغلفة والكلمات والترجمات الموجودة في الذاكرة المحلية ضمن المساحة المحددة.', 'Saved covers, lyrics and translations are included within the storage limit.')}</p>
      <div class="v3-panel-list">${estimate.items.slice(0,120).map(item => `<div class="v3-cue-row"><bdi>${esc(item.title || item.id)}</bdi><small>${item.error ? esc(item.error) : `${esc(item.quality.codec)} · ${bytes(item.bytes)}`}</small></div>`).join('')}</div>
      <div class="v3-dialog-actions">
        <label>${t('اسم الحزمة', 'Pack name')} <input id="v3-pack-name" dir="auto" maxlength="90" value="${esc(name)}"></label>
        <label>${t('حد الحزم على الجهاز (GB)', 'Device pack limit (GB)')} <input id="v3-pack-quota" type="number" min=".1" max="1024" step=".1" value="${ui.v3OfflineQuota ? (ui.v3OfflineQuota / 1024 ** 3).toFixed(1) : 5}"></label>
        <button class="primary" data-v3-tool="create-pack" ${estimate.audioBytes ? '' : 'disabled'}>${t('ابدأ الحفظ', 'Save offline')}</button>
      </div>`);
  }

  async function reviewTiming() {
    if (!current || current.preview) return;
    const item = await call('GetRangeAlignmentPreview', current.id);
    studioReview = item;
    const lines = item.lines.map((line, index) => {
      const evidence = line.words.filter(word => word.probability > 0);
      const confidence = evidence.length ? evidence.reduce((sum, word) => sum + word.probability, 0) / evidence.length : null;
      return {line, index, confidence};
    }).sort((a,b) => (a.confidence ?? -1) - (b.confidence ?? -1));
    modal(`<h2>${t('راجع التوقيت حسب الثقة', 'Review timing by confidence')}</h2>
      <p>${t('السطور الأضعف أولًا. اضغط على كلمة لسماعها.', 'Least confident lines first. Select a word to hear it.')}</p>
      <div class="v3-panel-list">${lines.map(({line, index, confidence}) => `<section>
        <small>${index + 1} · ${confidence == null ? t('ثقة غير متاحة', 'Confidence unavailable') : `${Math.round(confidence * 100)}%`} · <bdi>${time(line.start)}</bdi></small>
        <div class="v3-dialog-actions" dir="${textDirection(line.text)}">${line.words.map(word => `<button type="button" class="quiet" data-v3-tool="audition" data-start="${word.start}" data-end="${word.end}">${esc(word.text)}</button>`).join('')}</div>
      </section>`).join('')}</div>
      <button class="primary" data-v3-tool="approve-timing">${t('اعتمد التوقيت', 'Apply timing')}</button>`);
  }

  async function openNotes() {
    if (!current || current.preview) return;
    const id = current.id;
    const doc = await call('GetLyricDocument', id);
    const [translation, pronunciation] = await Promise.all([
      call('GetTranslation', id, ui.translationTarget, ui.translationSource, false),
      call('GetPronunciation', id)
    ]);
    if (current?.id !== id) return;
    const lines = translation.lines || [];
    if (!lines.length) return toast(t('أضف كلمات الأغنية أولًا', 'Add lyrics first'));
    lyricsNotes = {id, revision: doc.revision, target: ui.translationTarget, source: ui.translationSource, hash: translation.originalHash, lines, pronunciation};
    modal(`<h2>${t('الأصل والترجمة والنطق', 'Original, translation and pronunciation')}</h2>
      <div class="v3-subtitles">${lines.map((line, index) => `<div class="v3-subtitle-row">
        <strong dir="auto">${esc(line.original)}</strong>
        <input data-v3-translation="${index}" dir="auto" aria-label="${t('ترجمة السطر', 'Line translation')} ${index + 1}" placeholder="${t('الترجمة', 'Translation')}" value="${esc(line.text || '')}">
        <input data-v3-pronunciation="${index}" dir="auto" aria-label="${t('نطق السطر', 'Line pronunciation')} ${index + 1}" placeholder="${t('النطق', 'Pronunciation')}" value="${esc(pronunciation[index] || '')}">
      </div>`).join('')}</div>
      <div class="v3-dialog-actions"><button class="primary" data-v3-tool="save-notes">${t('احفظ الترجمة والنطق', 'Save translation and pronunciation')}</button></div>`);
  }

  let audition = null;
  audio.addEventListener('timeupdate', () => {
    if (!audition) return;
    if (current?.id !== audition.id) audition = null;
    else if (audio.currentTime >= audition.end) {
      audition = null;
      audio.pause();
    }
  });

  function studioTools() {
    if (!current || current.preview || $('#v3-studio-tools')) return;
    $('.page-head')?.insertAdjacentHTML('afterend', `<section id="v3-studio-tools" class="v3-dialog-actions">
      <button class="primary" data-v3-tool="full-review">${t('حلّل الأغنية كاملة للمراجعة', 'Analyze whole track for review')}</button>
      <button class="quiet" data-v3-tool="review-timing">${t('راجع الثقة واسمع الكلمات', 'Review confidence and audition words')}</button>
      <button class="quiet" data-v3-tool="notes">${t('الأصل والترجمة والنطق', 'Original, translation and pronunciation')}</button>
      <button class="quiet" data-v3-clip="open">${t('صدّر فيديو كلمات', 'Export lyric video')}</button>
    </section>`);
  }

  function soundTools() {
    if ($('#v3-sound-tools')) return;
    $('#main').insertAdjacentHTML('beforeend', `<section id="v3-sound-tools" class="v2-settings-section">
      <h2>${t('الصوت أثناء التشغيل', 'Playback audio')}</h2>
      <p id="v3-audio-info" dir="auto"></p>
      <label class="check-line"><input id="v3-album-level" type="checkbox" ${ui.v3AlbumNormalization ? 'checked' : ''}>${t('استخدم مستوى صوت موحّدًا للألبوم', 'Use one gain for the whole album')}</label>
      <div class="v3-dialog-actions">
        <button class="quiet" data-v3-tool="album-measure">${t('قِس مستوى الألبوم', 'Measure album loudness')}</button>
        ${window.NawaMobile ? '' : `<button class="quiet" data-v3-tool="mini">${t('مشغّل فوق النوافذ', 'Always-on-top player')}</button>`}
      </div>
      <p id="v3-album-measurement" role="status"></p>
    </section>`);
    updatePlayback(current);
  }

  function updatePlayback(task) {
    const target = $('#v3-audio-info');
    if (!target) return;
    const quality = task?.quality || {};
    const effects = [
      !ui.bypass && ui.eq?.some(value => Number(value)) ? 'EQ' : '',
      !ui.bypass && Number(ui.reverb) ? 'Reverb' : '',
      !ui.bypass && Number(ui.pitch) ? t('طبقة الصوت','Pitch') : '',
      !ui.bypass && ui.normalizeVolume ? t('موازنة الصوت','Normalization') : ''
    ].filter(Boolean);
    target.textContent = [
      quality.codec, quality.bits ? `${quality.bits}-bit` : '', quality.rate ? `${quality.rate / 1000} kHz` : '',
      sourceInfo?.outputRoute || t('مخرج النظام', 'System output'),
      sourceInfo?.effectiveOutput, effects.length ? effects.join(', ') : ''
    ].filter(Boolean).join(' · ');
  }

  function outputState(state) {
    if (!state?.outputRoute || sourceInfo?.outputRoute === state.outputRoute && sourceInfo?.effectiveOutput === state.effectiveOutput) return;
    sourceInfo = state;
    updatePlayback(current);
  }

  async function setMini() {
    await call('SetMiniPlayer', !mini);
    mini = !mini;
    document.body.classList.toggle('v3-mini-player', mini);
    const button = $('#v3-mini-close');
    if (button) button.hidden = !mini;
  }

  function mount() {
    if (view === 'studio') studioTools();
    if (view === 'player') soundTools();
    if (view === 'offline') refreshOffline().catch(error => toast(error.message));
    if (view === 'settings' && !$('#v3-migration')) {
      $('.page-head')?.insertAdjacentHTML('beforeend', `<button id="v3-migration" class="quiet" data-v3-tool="migration">${t('تقرير الترقية', 'Upgrade report')}</button>`);
    }
    if (!window.NawaMobile && !$('#v3-mini-close')) {
      $('.titlebar')?.insertAdjacentHTML('beforeend', `<button id="v3-mini-close" class="quiet" data-v3-tool="mini" hidden>${t('فتح نَوى', 'Expand Nawa')}</button>`);
    }
  }

  document.addEventListener('input', event => {
    if (event.target.id !== 'v3-health-query') return;
    healthQuery = event.target.value;
    healthLimit = 60;
    updateHealth();
  });
  document.addEventListener('change', event => {
    if (event.target.id === 'v3-health-filter') {
      healthFilter = event.target.value;
      healthLimit = 60;
      updateHealth();
    } else if (event.target.id === 'v3-album-level') {
      ui.v3AlbumNormalization = event.target.checked;
      if (ui.v3AlbumNormalization) ui.normalizeVolume = true;
      persist();
      window.nawaV2Playback.trackChanged();
    }
  });

  document.addEventListener('click', async event => {
    const button = event.target.closest('[data-v3-tool]');
    if (!button || button.disabled) return;
    const action = button.dataset.v3Tool;
    button.disabled = true;
    try {
      switch (action) {
        case 'scan':
        case 'relink':
          libraryReport = await call('ReviewLibrary', action === 'relink');
          if (action === 'relink') {
            toast(`${libraryReport.relinked} ${t('مسار أعيد ربطه', 'tracks relinked')}`);
            await poll();
          }
          updateHealth();
          break;
        case 'health-more': healthLimit += 60; updateHealth(); break;
        case 'fingerprint': {
          const matches = await call('CompareAudioFingerprints', button.dataset.tracks.split(','));
          modal(`<h2>${t('نتائج بصمة الصوت', 'Audio fingerprint results')}</h2>${matches.length ? matches.map(match => `<p><bdi>${esc(taskByID(match.first)?.song.title)}</bdi> · <bdi>${esc(taskByID(match.second)?.song.title)}</bdi> · ${Math.round(match.similarity * 100)}%</p>`).join('') : `<p>${t('ما ظهر تطابق قوي في المقطع المفحوص.', 'No strong match in the analyzed segment.')}</p>`}`);
          break;
        }
        case 'estimate-pack': {
          const value = $('#v3-offline-source')?.value;
          const ids = packTracks(value);
          if (!ids.length) throw Error(t('اختر قائمة تحتوي أغاني محفوظة', 'Choose a playlist with saved tracks'));
          await estimatePack(ids, $('#v3-offline-source').selectedOptions[0]?.textContent || 'Nawa');
          break;
        }
        case 'create-pack': {
          const quota = Math.round(Number($('#v3-pack-quota').value) * 1024 ** 3);
          await call('StartOfflinePack', $('#v3-pack-name').value, pendingPack.ids, quota, ui.translationTarget, ui.translationSource);
          ui.v3OfflineQuota = quota;
          persist();
          pendingPack = null;
          $('#modal').close();
          offline.running = true;
          await refreshOffline();
          break;
        }
        case 'cancel-pack': await call('CancelOfflinePack'); break;
        case 'retry-pack': {
          const pack = packs.find(item => item.id === button.dataset.id);
          await estimatePack(pack.items.filter(item => !item.file).map(item => item.id), pack.name + ' +');
          break;
        }
        case 'confirm-delete-pack':
          modal(`<h2>${t('إزالة الحزمة من هذا الجهاز', 'Remove pack from this device')}</h2><button class="primary" data-v3-tool="delete-pack" data-id="${esc(button.dataset.id)}">${t('أزل الحزمة', 'Remove pack')}</button>`);
          break;
        case 'delete-pack':
          await call('DeleteOfflinePack', button.dataset.id);
          offlineLoaded = false;
          $('#modal').close();
          await refreshOffline();
          break;
        case 'play-pack': {
          const pack = packs.find(item => item.id === button.dataset.id);
          const queue = pack?.items.filter(item => item.file && taskByID(item.id)?.status === 'done').map(item => item.id) || [];
          if (queue.length) await playTask(queue[0], queue);
          break;
        }
        case 'full-review':
          if (window.nawaV2Lyrics?.dirty) throw Error(t('احفظ تعديلات الكلمات أولًا', 'Save lyric edits first'));
          await call('StartFullLyricReview', current.id, snapshot.preferences.alignmentLanguage || 'auto');
          await poll();
          break;
        case 'review-timing': await reviewTiming(); break;
        case 'audition':
          if (!current || !(duration() > 0)) break;
          audition = {id: current.id, end: Math.min(duration(), Math.max(Number(button.dataset.end), Number(button.dataset.start) + .22))};
          audio.currentTime = Number(button.dataset.start);
          if (audio.paused) await togglePlay();
          break;
        case 'approve-timing':
          if (studioReview?.taskId === current?.id) {
            if (window.nawaV2Lyrics?.dirty) throw Error(t('احفظ تعديلات الكلمات أولًا', 'Save lyric edits first'));
            await call('ApplyRangeAlignment', current.id);
            $('#modal').close();
            await poll();
            const id = current.id;
            navigate('lyrics');
            if (current?.id === id) navigate('studio');
          }
          break;
        case 'notes': await openNotes(); break;
        case 'save-notes': {
          if (!lyricsNotes || current?.id !== lyricsNotes.id) break;
          const translations = $$('[data-v3-translation]').map(input => input.value);
          const pronunciation = $$('[data-v3-pronunciation]').map(input => input.value);
          const changed = translations.some((value, i) => value !== (lyricsNotes.lines[i].text || ''));
          if (changed) await call('SaveTranslation', lyricsNotes.id, lyricsNotes.target, lyricsNotes.source, lyricsNotes.hash, translations);
          await call('SavePronunciation', lyricsNotes.id, lyricsNotes.revision, pronunciation);
          $('#modal').close();
          toast(t('حُفظت تعديلات الكلمات', 'Lyric notes saved'));
          break;
        }
        case 'album-measure': {
          const info = await call('AnalyzeAlbumLoudness', current?.id || '');
          const node = $('#v3-album-measurement');
          if (node) node.textContent = `${info.integratedLUFS.toFixed(1)} LUFS · ${info.gainDB.toFixed(1)} dB · ${info.measured}/${info.tracks}`;
          break;
        }
        case 'mini': await setMini(); break;
        case 'migration': {
          const report = await call('GetMigrationReport');
          modal(`<h2>${t('تقرير الترقية', 'Upgrade report')}</h2>
            <p>${report.backupCreated ? t('تم حفظ نسخة من بيانات المكتبة قبل الترقية.', 'Library data was backed up before upgrading.') : t('ما توجد نسخة ترقية سابقة في مجلد التطبيق.', 'No earlier upgrade backup exists in this app folder.')}</p>
            <p>${report.tracksBefore} ${t('قبل الترقية', 'before upgrade')} · ${report.tracks} ${t('الآن', 'now')} · ${report.needsRelink.length} ${t('تحتاج ربط', 'need relinking')}</p>
            <ul>${report.needsRelink.slice(0,120).map(title => `<li dir="auto">${esc(title)}</li>`).join('')}</ul>`);
          break;
        }
      }
    } catch (error) {
      toast(error.message || String(error));
    } finally {
      if (button.isConnected) button.disabled = false;
    }
  });

  setInterval(() => refreshOffline().catch(() => {}), 1800);
  window.nawaV3Tools = {mount, updatePlayback, outputState};
})();
