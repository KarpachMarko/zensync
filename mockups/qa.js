// Injected into html_preview for a browser-executed interaction check.
(function(){
 const check=(condition,message)=>{if(!condition)throw new Error(message)};
 const click=(s)=>{let el=document.querySelector(s);check(el,'Missing '+s);el.click()};
 for(const c of concepts){click('[data-action="concept"][data-id="'+c.id+'"]');check(current===c.id,'Concept selection');check(document.querySelector('#app').textContent.includes('Night Drive'),'Playlist content');check(document.documentElement.scrollWidth<=window.innerWidth,'Horizontal overflow in '+c.id)}
 click('[data-action="import"]');check(document.querySelector('#modal').open,'Import dialog');click('[data-action="import-preview"]');check(document.querySelectorAll('#modal input[type="checkbox"]').length===3,'XML playlist choices');click('[data-action="import-done"]');check(!document.querySelector('#modal').open,'Import completed');
 inspect(4);check(document.querySelectorAll('input[name="match"]').length===2,'Match candidates');click('[data-action="match-save"]');check(state(tracks[3])==='pending','Resolved match should be ready to download');
 click('[data-action="queue"]');check(queued,'Download queue');downloads();check(document.querySelector('#modal-body').textContent.includes('42%'),'Simulated queue progress');click('[data-action="cancel-demo"]');check(!queued,'Cancel simulation');
 click('[data-action="search"]');document.querySelector('#catalog-query').value='Radiohead';click('[data-action="catalog-search"]');check(document.querySelector('#catalog-results').textContent.includes('Everything in Its Right Place'),'Catalog search');check(!document.querySelector('#catalog-results').textContent.includes('Teardrop'),'Search filtering');close();
 current='workbench';query='';filter='all';render();let input=document.querySelector('[data-search]');input.value='Teardrop';input.dispatchEvent(new Event('input',{bubbles:true}));check(document.querySelectorAll('.table-row').length===1,'Playlist filtering');
 click('[data-action="settings"]');document.querySelector('#quality').value='MP3 320';click('[data-action="settings-save"]');check(preferredQuality==='MP3 320','Quality selection');
 resolved=false;queued=false;query='';filter='all';current='collection';render();document.querySelector('#toast').hidden=true;clearTimeout(toastTimer);
 console.log('PASS: five layouts, overflow, XML import preview, match resolution, queue/cancel, catalog search, playlist filter and quality selection');
})();
