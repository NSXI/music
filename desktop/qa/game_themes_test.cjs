const {boot}=require('./app_boot_test.cjs'),vm=require('node:vm'),assert=require('node:assert/strict');
(async()=>{let checks=0;for(const language of ['ar','en']){
 const {ctx,window,document,audio,storage}=await boot(language,{lyricStyle:'focus',lyricFont:'Georgia',translationFont:'Tahoma'}),run=s=>vm.runInContext(s,ctx),before={src:audio.src,time:audio.currentTime,volume:run('ui.volume')};
 assert.equal(run('lyricStyles.length'),107);
 for(const [theme,style,font] of [['kingdom','game-kingdom','Nawa KHMenu'],['bloodborne','game-bloodborne','Nawa Hunter'],['reload','game-reload','Nawa Reload']]){
  run(`ui.theme=${JSON.stringify(theme)};applyAppearance();navigate('appearance')`);
  assert.equal(run('ui.lyricStyle'),style);assert.equal(run('ui.lyricFont'),font);assert.ok(document.querySelector('.nawa-game-avatar'));assert.ok(document.querySelector('[data-game-theme-lyrics]'));checks+=4;
 }
 run("ui.theme='kingdom';applyAppearance();navigate('appearance')");
 run("ui.lyricFont='Consolas';ui.gameThemeChoices.kingdom={lyricFont:'Consolas',translationFont:'Tahoma',lyricStyle:'game-kingdom'};applyTypography();persist();ui.theme='reload';applyAppearance();ui.theme='kingdom';applyAppearance()");assert.equal(run('ui.lyricFont'),'Consolas');assert.equal(run('ui.translationFont'),'Tahoma');checks+=2;
 run("ui.gameThemeLyrics=false;applyAppearance()");assert.equal(run('ui.lyricStyle'),'focus');assert.equal(run('ui.lyricFont'),'Georgia');assert.equal(run('ui.translationFont'),'Tahoma');checks+=3;
 run("ui.gameThemeLyrics=true;applyAppearance()");assert.equal(run('ui.lyricFont'),'Consolas');checks++;
 run("ui.gameThemeAvatar='symbol';applyAppearance()");assert.equal(document.body.dataset.gameAvatar,'symbol');run("ui.gameThemeAvatar='none';applyAppearance()");assert.equal(document.querySelector('.nawa-game-avatar'),null);checks+=2;
 run("ui.theme='noir';applyAppearance()");assert.equal(run('ui.lyricStyle'),'focus');assert.equal(run('ui.lyricFont'),'Georgia');assert.equal(run('ui.gameThemePrevious'),undefined);checks+=3;
 assert.deepEqual({src:audio.src,time:audio.currentTime,volume:run('ui.volume')},before);assert.equal(JSON.parse(storage.get('madar-ui-v1')).gameThemeChoices.kingdom.lyricFont,'Consolas');checks+=2;
}
console.log(`PASS: ${checks} approved game preset, per-theme font/style retention, restoration, avatar and unchanged audio checks in Arabic and English.`)
})().catch(error=>{console.error(error);process.exitCode=1});
