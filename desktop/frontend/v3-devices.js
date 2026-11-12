'use strict';

(() => {
  const {t} = window.nawaV2;
  let status = {hosting: false, paired: false, links: [], changes: 0};
  let result = null;
  let syncError = '';
  let syncing = false;
  let statusBusy = false;
  let lastSync = 0;

  function body() {
    return `<h2>${t('كمّل على جهاز ثاني', 'Continue on another device')}</h2>
      <p>${t('اربط الجهازين على نفس الشبكة لمتابعة التشغيل ومزامنة تعديلات مكتبتك.', 'Pair on the same network to continue listening and sync library edits.')}</p>
      <div class="v3-dialog-actions">
        <button class="${status.hosting ? 'quiet' : 'primary'}" data-v3-device="host">${status.hosting ? t('أوقف استقبال الربط', 'Stop accepting connections') : t('فعّل الربط على هذا الجهاز', 'Enable pairing here')}</button>
        ${status.hosting && status.links?.length ? `<button class="quiet" data-v3-device="show-link">${t('اعرض رابط الربط', 'Show pairing link')}</button>` : ''}
      </div>
      <div class="v3-dialog-actions">
        <input id="v3-device-link" type="password" inputmode="url" autocomplete="off" placeholder="${t('رابط من جهاز نَوى الآخر', 'Pairing link from the other Nawa device')}" aria-label="${t('رابط الربط', 'Pairing link')}" dir="ltr">
        <button class="quiet" data-v3-device="pair">${status.paired ? t('حدّث الربط', 'Update pairing') : t('اربط الجهاز', 'Pair device')}</button>
        ${status.paired ? `<button class="quiet" data-v3-device="forget">${t('ألغِ الربط المحفوظ', 'Forget paired device')}</button>` : ''}
      </div>
      ${status.paired ? `<label class="check-line"><input id="v3-device-auto" type="checkbox" ${ui.v3ConnectedSync !== false ? 'checked' : ''}>${t('زامن المفضلة والقوائم والكلمات المعدّلة عند الاتصال', 'Sync favorites, playlists and edited lyrics when connected')}</label>
      <div class="v3-dialog-actions">
        <button class="primary" data-v3-device="continue">${t('كمّل على هذا الجهاز', 'Continue on this device')}</button>
        <button class="quiet" data-v3-device="sync">${t('زامن الآن', 'Sync now')}</button>
      </div>` : ''}
      <div id="v3-sync-result" role="status">${summary()}</div>`;
  }

  function summary() {
    if (syncError) return `<p class="v2-error">${esc(syncError)}</p>`;
    if (!result) return '';
    const conflict = result.conflicts?.length;
    return `<p>${t('استلمت', 'Received')} ${result.received} · ${t('أرسلت', 'Sent')} ${result.sent}${result.missing ? ` · ${result.missing} ${t('تحتاج ملفًا محليًا أو مراجعة', 'need local files or review')}` : ''}</p>
      ${conflict ? `<details open><summary>${conflict} ${t('تعديلات مختلفة على الجهازين', 'edits differ between devices')}</summary>
      <ul>${result.conflicts.slice(0,60).map(title => `<li dir="auto">${esc(title)}</li>`).join('')}</ul>
      <div class="v3-dialog-actions"><button class="quiet" data-v3-device="resolve-local">${t('اعتمد نسخة هذا الجهاز للتعارضات', 'Use this device for conflicts')}</button><button class="quiet" data-v3-device="resolve-peer">${t('اعتمد نسخة الجهاز الآخر للتعارضات', 'Use the other device for conflicts')}</button></div></details>` : ''}`;
  }

  function renderPanel() {
    const panel = $('#v3-device-panel');
    if (panel) panel.innerHTML = body();
  }

  function mount() {
    if (view !== 'devices' || $('#v3-device-panel')) return;
    $('.page-head')?.insertAdjacentHTML('afterend', `<section id="v3-device-panel" class="v2-settings-section">${body()}</section>`);
    refreshStatus().catch(error => toast(error.message));
  }

  async function refreshStatus() {
    if (statusBusy) return;
    statusBusy = true;
    try {
      const next = await call('GetContinuityStatus');
      const changed = JSON.stringify([next.hosting, next.paired, next.links]) !== JSON.stringify([status.hosting, status.paired, status.links]);
      const previous = status.changes;
      status = next;
      if (next.changes !== previous && !syncing) await window.nawaV2.refreshCollections();
      if (changed) renderPanel();
      if (status.hosting) {
        const queue = playOrder.slice(0,10000);
        const pause = await call('UpdateContinuityPlayback', current?.preview ? '' : current?.id || '', audio.currentTime || 0, queue, audio.paused);
        if (pause && !audio.paused) audio.pause();
      }
    } finally {
      statusBusy = false;
    }
  }

  async function synchronize(choice = 'review', interactive = false) {
    if (syncing || !status.paired) return;
    syncing = true;
    lastSync = Date.now();
    try {
      await window.nawaV2.flush();
      result = await call('SyncContinuity', choice);
      syncError = '';
      if (result.received) await window.nawaV2.refreshCollections();
    } catch (error) {
      if (interactive) syncError = error.message || String(error);
    } finally {
      syncing = false;
      const node = $('#v3-sync-result');
      if (node) node.innerHTML = summary();
    }
  }

  async function continuePlayback() {
    const session = await call('GetContinuityPlayback');
    if (!session.trackKey) throw Error(t('ما فيه أغنية على الجهاز الآخر', 'The other device has no active track'));
    if (!session.taskId) throw Error(t('أضف هذه الأغنية إلى مكتبة هذا الجهاز أولًا', 'Add this track to this device’s library first'));
    await playTask(session.taskId, [...new Set([session.taskId, ...session.queue])]);
    if (current?.id !== session.taskId) return;
    if (!(duration() > 0)) {
      await new Promise((resolve, reject) => {
        const timeout = setTimeout(() => {
          audio.removeEventListener('loadedmetadata', ready);
          reject(Error(t('الصوت ما زال يجهّز', 'Audio is still loading')));
        }, 8000);
        function ready() {clearTimeout(timeout); resolve()}
        audio.addEventListener('loadedmetadata', ready, {once: true});
      });
    }
    window.nawaV3Stage.seek(session.position);
    if (!session.paused) await call('ConfirmContinuityPlayback', session.trackKey);
    window.nawaV3.openDeck();
  }

  document.addEventListener('change', event => {
    if (event.target.id !== 'v3-device-auto') return;
    ui.v3ConnectedSync = event.target.checked;
    persist();
    if (ui.v3ConnectedSync) synchronize();
  });

  document.addEventListener('click', async event => {
    const button = event.target.closest('[data-v3-device]');
    if (!button || button.disabled) return;
    button.disabled = true;
    try {
      switch (button.dataset.v3Device) {
        case 'host':
          await call('EnableContinuityHost', !status.hosting);
          await refreshStatus();
          break;
        case 'show-link':
          modal(`<h2>${t('رابط ربط الجهاز', 'Device pairing link')}</h2><p>${t('انسخ الرابط إلى صفحة الأجهزة في نَوى على الجهاز الآخر.', 'Paste the link into Devices in Nawa on the other device.')}</p>${status.links.map(link => `<input class="v3-pair-link" readonly dir="ltr" value="${esc(link)}" aria-label="${t('رابط الربط', 'Pairing link')}">`).join('')}`);
          $('.v3-pair-link')?.select();
          break;
        case 'pair':
          if (!$('#v3-device-link')?.value.trim()) throw Error(t('ألصق رابط الجهاز الآخر', 'Paste the other device’s pairing link'));
          await call('PairContinuity', $('#v3-device-link').value);
          await refreshStatus();
          await synchronize('review', true);
          break;
        case 'forget':
          await call('PairContinuity', '');
          result = null;
          await refreshStatus();
          break;
        case 'sync': await synchronize('review', true); break;
        case 'resolve-local': await synchronize('local', true); break;
        case 'resolve-peer': await synchronize('peer', true); break;
        case 'continue': await continuePlayback(); break;
      }
    } catch (error) {
      toast(error.message || String(error));
    } finally {
      if (button.isConnected) button.disabled = false;
    }
  });

  async function heartbeat() {
    if (document.hidden) return;
    if (status.hosting || status.paired || view === 'devices') await refreshStatus();
    if (status.paired && ui.v3ConnectedSync !== false && Date.now() - lastSync > 60000) await synchronize();
  }
  setInterval(() => heartbeat().catch(() => {}), 1500);
  document.addEventListener('visibilitychange', () => heartbeat().catch(() => {}));
  (async () => {
    try {await call('ResumeContinuity'); await refreshStatus()}
    catch { /* Settings can show a connection error when the device is opened. */ }
  })();
  window.nawaV3Devices = {mount};
})();
