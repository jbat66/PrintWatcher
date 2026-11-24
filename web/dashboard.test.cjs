const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function dashboard() {
    const elements = new Map(), classes = new Set(), played = [], fetched = [];
    const element = id => {
        if (!elements.has(id)) elements.set(id, {hidden: false, textContent: '', setAttribute() {}, appendChild() {}, focus() {}});
        return elements.get(id);
    };
    const wrapper = {classList: {add: v => classes.add(v), remove: v => classes.delete(v)}, querySelector: selector => element(selector.includes('overlay') ? 'overlay-remaining' : 'remaining')};
    const cards = new Map();
    const replaceCard = id => {
        const names = new Set();
        const card = {names, classList:{add(v){names.add(v);classes.add(v);},remove(v){names.delete(v);classes.delete(v);}},
            querySelector: () => element(`zoom-${id}`)};
        cards.set(id,card);return card;
    };
    [1,2].forEach(replaceCard);
    const classList = {add: v => classes.add(v), remove: v => classes.delete(v)};
    let now = 100000;
    class Context {
        constructor() { this.state = 'running'; this.destination = {}; this.sampleRate = 48000; }
        addEventListener() {}
        resume() { return Promise.resolve(); }
        createBuffer() { return {silence: true}; }
        decodeAudioData(data) { return Promise.resolve({sound: data}); }
        createBufferSource() {
            const source = {connect() {}, disconnect() {}, stop() { this.stopped = true; }, start() { if (!this.buffer.silence) played.push(this); }};
            return source;
        }
    }
    const ctx = {
        window: {AudioContext: Context, addEventListener() {}},
        document: {
            body: {classList}, getElementById: element,
            querySelector: selector => {
                if (selector.includes('.webcam-wrapper')) return wrapper;
                if (selector.includes('data-diagnostic')) return element(selector);
                const zoom=selector.match(/^\[data-printer-zoom="(\d+)"\]$/);
                if(zoom) return element(`zoom-${zoom[1]}`);
                const card=selector.match(/^\[data-printer-id="(\d+)"\]$/);
                return card ? cards.get(Number(card[1])) : {focus() {}};
            },
            querySelectorAll: selector => selector === '.camera-focused' && classes.has('camera-focused') ? [wrapper] : selector === '.printer-zoomed' ? [...cards.values()].filter(card=>card.names.has('printer-zoomed')) : [],
            addEventListener() {}, createElement: () => ({setAttribute() {}})
        },
        printers: [1, 2].map(id => ({config: {id, enabled: true, notification_sound: id === 1 ? 'Cuckoo.mp3' : 'RedAlert.mp3'}, status: {state: 'printing', job: {remaining_secs: 600}}})),
        getWebcamMode: () => 'live', formatTime: value => `${value}s`,
        Date: {now: () => now}, setInterval() {}, AbortController, setTimeout() {}, clearTimeout() {},
        fetch: async url => { fetched.push(url); return {ok: true, arrayBuffer: async () => url}; }
    };
    vm.createContext(ctx);
    const app = fs.readFileSync(__dirname + '/app.js', 'utf8');
    vm.runInContext(app.slice(app.indexOf('// Per-printer connection diagnostics'), app.indexOf('// Initialize')), ctx);
    return {ctx, played, fetched, classes, element, cards, replaceCard, advance: n => {now += n;}, pending: id => vm.runInContext(`printerAlerts.get(${id}).pending`, ctx)};
}

const settle = () => new Promise(resolve => setImmediate(resolve));

test('single camera activation toggles focus and retains live remaining time', () => {
    const d = dashboard();
    d.ctx.toggleCameraFocus(1);
    assert(d.classes.has('camera-focused'));
    assert.equal(d.element('remaining').textContent, '600s');
    d.ctx.printers[0].status.job.remaining_secs = 400;
    d.ctx.refreshCameraFocus();
    assert.equal(d.element('remaining').textContent, '400s');
    d.ctx.toggleCameraFocus(1);
    assert(!d.classes.has('camera-focused'));
    d.ctx.getWebcamMode = () => 'snapshot';
    d.ctx.toggleCameraFocus(1);
    assert(!d.classes.has('camera-focused'));
});

test('baseline and disconnections do not alert; errors repeat until acknowledged', async () => {
    const d = dashboard();
    d.ctx.syncPrinterAlerts();
    await d.ctx.enableAlertSounds(); await settle();
    assert.equal(d.played.length, 0);
    const p = d.ctx.printers[0];
    p.status.state = 'offline'; d.ctx.observePrinterAlert(p);
    assert(!d.pending(1));
    p.status.state = 'error'; d.ctx.observePrinterAlert(p);
    await d.ctx.tickPrinterAlerts();
    assert.equal(d.fetched[0], '/sounds/Cuckoo.mp3');
    assert(d.pending(1));
    d.played[0].onended();
    d.advance(59999); await d.ctx.tickPrinterAlerts();
    assert.equal(d.played.length, 1);
    d.advance(1); await d.ctx.tickPrinterAlerts();
    assert.equal(d.played.length, 2);
    d.ctx.acknowledgePrinterAlert(1);
    assert(!d.pending(1)); assert(d.played[1].stopped);
    p.status.state = 'idle'; d.ctx.observePrinterAlert(p);
    assert(!d.pending(1));
});

test('printer mute is independent; unmute retains pending and acknowledgement is independent', async () => {
    const d = dashboard(); d.ctx.syncPrinterAlerts();
    await d.ctx.enableAlertSounds(); await settle();
    for (const p of d.ctx.printers) { p.status.state = 'error'; d.ctx.observePrinterAlert(p); }
    d.ctx.toggleAlertMute(1); await settle();
    assert.equal(d.played.length, 1);
    assert.deepEqual(d.fetched, ['/sounds/RedAlert.mp3']);
    d.played[0].onended();
    d.ctx.acknowledgePrinterAlert(2);
    assert(d.pending(1)); assert(!d.pending(2));
    d.ctx.toggleAlertMute(1); await settle();
    assert.equal(d.played.length, 2);
    assert.equal(d.fetched[1], '/sounds/Cuckoo.mp3');
    d.ctx.printers[0].config.notification_sound = '';
    d.ctx.syncPrinterAlerts();
    assert(d.played[1].stopped);
});

test('acknowledge during a sound download prevents late playback', async () => {
    const d = dashboard(); d.ctx.syncPrinterAlerts();
    await d.ctx.enableAlertSounds(); await settle();
    d.ctx.printers[0].status.state = 'error'; d.ctx.observePrinterAlert(d.ctx.printers[0]);
    let resolveFetch;
    d.ctx.fetch = () => new Promise(resolve => {resolveFetch = resolve;});
    const play = d.ctx.tickPrinterAlerts();
    d.ctx.acknowledgePrinterAlert(1);
    resolveFetch({ok: true, arrayBuffer: async () => 'sound'});
    await play;
    assert.equal(d.played.length, 0);
});


test('only errors, filament requests and confirmed completion trigger new alerts', () => {
    const d = dashboard(); d.ctx.syncPrinterAlerts();
    const p = d.ctx.printers[0];
    for (const state of ['paused','printing','busy','idle','offline','disconnected','printing']) {
        p.status = {state, job: {job_state: state.toUpperCase()}};
        d.ctx.observePrinterAlert(p); assert(!d.pending(1), state);
    }
    p.status = {state:'idle', job:{job_state:'STOPPED',progress:100}};
    d.ctx.observePrinterAlert(p); assert(!d.pending(1));
    p.status = {state:'printing',job:{job_state:'PRINTING'}}; d.ctx.observePrinterAlert(p);
    p.status = {state:'idle',job:{job_state:'FINISHED'}}; d.ctx.observePrinterAlert(p);
    assert(d.pending(1)); d.ctx.acknowledgePrinterAlert(1);
    d.ctx.observePrinterAlert(p); assert(!d.pending(1), 'finished must not rearm every poll');
    p.status = {state:'paused',state_message:'Change filament (M600)',job:{job_state:'PAUSED'}};
    d.ctx.observePrinterAlert(p); assert(d.pending(1));
    d.ctx.acknowledgePrinterAlert(1); d.ctx.observePrinterAlert(p); assert(!d.pending(1));
    p.status = {state:'printing',job:{job_state:'PRINTING'}}; d.ctx.observePrinterAlert(p);
    p.status = {state:'paused',state_message:'Filament runout'}; d.ctx.observePrinterAlert(p); assert(d.pending(1));
});

test('initial finished status does not alert and job clearing cannot manufacture completion', () => {
    const d = dashboard(), p = d.ctx.printers[0];
    p.status = {state:'idle',job:{job_state:'FINISHED'}}; d.ctx.syncPrinterAlerts(); assert(!d.pending(1));
    p.status = {state:'printing',job:{job_state:'PRINTING',progress:100}}; d.ctx.observePrinterAlert(p);
    p.status = {state:'idle',job:null}; d.ctx.observePrinterAlert(p); assert(!d.pending(1));
});

test('printer-specific file type validation blocks Klipper BGCODE before sending a request', async () => {
    const d = dashboard();
    const app=fs.readFileSync(__dirname+'/app.js','utf8');
    vm.runInContext(app.slice(app.indexOf('function uploadFileError('),app.indexOf('function hideTestResult(')),d.ctx);
    const u1={config:{id:1,type:'snapmaker-u1'}}, prusa={config:{id:2,type:'prusalink'}};
    d.ctx.printers=[u1,prusa];
    assert.equal(d.ctx.uploadFileError(u1,'test.GCODE'),'');
    assert.match(d.ctx.uploadFileError(u1,'test.bgcode'), /gcode files only/);
    for (const name of ['test.gcode','test.BGCODE']) assert.equal(d.ctx.uploadFileError(prusa,name),'');
    d.element('upload-printer').value='1';d.ctx.updateUploadFileTypes();assert.equal(d.element('upload-file').accept,'.gcode');
    d.element('upload-file').files=[{name:'wrong.bgcode'}];d.element('upload-result').style={};
    await d.ctx.submitUpload({preventDefault(){}});assert.equal(d.fetched.length,0);assert.match(d.element('upload-result').textContent,/gcode files only/);
    d.element('upload-printer').value='2';d.ctx.updateUploadFileTypes();assert.equal(d.element('upload-file').accept,'.gcode,.bgcode');
});

test('a single activation enters mobile video-only mode and the next returns', () => {
    const d = dashboard(); d.ctx.navigator = {userAgent:'iPhone'};
    d.ctx.toggleCameraFocus(1);assert(d.classes.has('camera-mobile'));
    assert.equal(d.element('overlay-remaining').textContent,'600s');
    d.ctx.printers[0].status.job.remaining_secs=420;
    d.ctx.refreshCameraFocus();assert.equal(d.element('overlay-remaining').textContent,'420s');
    d.ctx.toggleCameraFocus(1);assert(!d.classes.has('camera-focused'));assert(!d.classes.has('camera-mobile'));
    const source=fs.readFileSync(__dirname+'/app.js','utf8');
    assert(source.includes('onclick="toggleCameraFocus(${cfg.id})"'));
    assert(!source.includes('ondblclick='));
    assert(!source.includes('onpointerup='));
});

test('mobile requests native fullscreen and landscape then releases both on return', async () => {
    const d=dashboard();d.ctx.navigator={userAgent:'Android'};
    let locks=0,unlocks=0,exits=0;
    const root={requestFullscreen(){d.ctx.document.fullscreenElement=root;return Promise.resolve();}};
    d.ctx.document.documentElement=root;
    d.ctx.document.exitFullscreen=()=>{exits++;d.ctx.document.fullscreenElement=null;return Promise.resolve();};
    d.ctx.window.screen={orientation:{lock(orientation){assert.equal(orientation,'landscape');locks++;return Promise.resolve();},unlock(){unlocks++;}}};
    d.ctx.toggleCameraFocus(1);await settle();assert.equal(locks,1);assert(d.classes.has('camera-mobile'));
    d.ctx.closeCameraFocus();assert.equal(exits,1);assert.equal(unlocks,1);assert(!d.classes.has('camera-focused'));
});

test('unsupported fullscreen or orientation locking retains the landscape fallback', async () => {
    const d=dashboard();d.ctx.navigator={userAgent:'iPhone'};
    d.ctx.document.documentElement={requestFullscreen(){return Promise.reject(Error('unsupported'));}};
    d.ctx.toggleCameraFocus(1);await settle();assert(d.classes.has('camera-mobile'));
    d.ctx.closeCameraFocus();assert(!d.classes.has('camera-focused'));
});

test('native fullscreen exit restores the dashboard', async () => {
    const d=dashboard();d.ctx.navigator={userAgent:'Android'};
    const root={requestFullscreen(){d.ctx.document.fullscreenElement=root;return Promise.resolve();}};
    d.ctx.document.documentElement=root;
    d.ctx.toggleCameraFocus(1);await settle();
    d.ctx.document.fullscreenElement=null;d.ctx.cameraFullscreenChanged();assert(!d.classes.has('camera-mobile'));assert(!d.classes.has('camera-focused'));
});

test('a late fullscreen request cannot reopen a camera after return', async () => {
    const d=dashboard();d.ctx.navigator={userAgent:'Android'};
    let resolveRequest;
    const root={requestFullscreen(){return new Promise(resolve=>{resolveRequest=resolve;});}};
    d.ctx.document.documentElement=root;
    d.ctx.document.exitFullscreen=()=>{d.ctx.document.fullscreenElement=null;return Promise.resolve();};
    d.ctx.toggleCameraFocus(1);d.ctx.closeCameraFocus();
    d.ctx.document.fullscreenElement=root;resolveRequest();await settle();
    assert.equal(d.ctx.document.fullscreenElement,null);assert(!d.classes.has('camera-focused'));
});

test('orientation lock rejection keeps video-only mode and returns cleanly', async () => {
    const d=dashboard();d.ctx.navigator={userAgent:'Android'};
    const root={requestFullscreen(){d.ctx.document.fullscreenElement=root;return Promise.resolve();}};
    d.ctx.document.documentElement=root;
    d.ctx.window.screen={orientation:{lock(){return Promise.reject(Error('not allowed'));},unlock(){}}};
    d.ctx.document.exitFullscreen=()=>{d.ctx.document.fullscreenElement=null;return Promise.resolve();};
    d.ctx.toggleCameraFocus(1);await settle();assert(d.classes.has('camera-mobile'));
    d.ctx.closeCameraFocus();assert.equal(d.ctx.document.fullscreenElement,null);
});

test('a late fullscreen request is adopted by a newly focused mobile camera', async () => {
    const d=dashboard();d.ctx.navigator={userAgent:'Android'};
    let resolveRequest, requests=0;
    const root={requestFullscreen(){if(requests++)return Promise.reject(Error('already pending'));return new Promise(resolve=>{resolveRequest=resolve;});}};
    d.ctx.document.documentElement=root;
    d.ctx.document.exitFullscreen=()=>{d.ctx.document.fullscreenElement=null;return Promise.resolve();};
    d.ctx.toggleCameraFocus(1);d.ctx.toggleCameraFocus(2);
    d.ctx.document.fullscreenElement=root;resolveRequest();await settle();
    assert(d.classes.has('camera-mobile'));
    d.ctx.closeCameraFocus();assert.equal(d.ctx.document.fullscreenElement,null);
});


test('a recovered camera becomes visible after an earlier failed image request', () => {
    const app=fs.readFileSync(__dirname+'/app.js','utf8');
    const ctx={};vm.createContext(ctx);
    vm.runInContext(app.slice(app.indexOf('function webcamRecovered('),app.indexOf('function webcamSrc(')),ctx);
    const placeholder={style:{display:'flex'}}, beside={style:{display:'none'}};
    let uncollapsed=false;
    const container={querySelector:()=>placeholder,classList:{remove(name){uncollapsed=name==='webcam-collapsed';}},closest:()=>({querySelector:()=>beside})};
    for(const mode of ['small','enlarged']) {
        const image={style:{display:'none'},parentElement:container};
        ctx.webcamRecovered(image);
        assert.equal(image.style.display,'',mode);
        assert.equal(placeholder.style.display,'none');
        assert(uncollapsed);assert.equal(beside.style.display,'');
    }
});

test('dashboard fullscreen toggles the whole page and follows native exit', async () => {
    const d=dashboard();let requests=0,exits=0;
    const root={requestFullscreen(){requests++;d.ctx.document.fullscreenElement=root;return Promise.resolve();}};
    d.ctx.document.documentElement=root;
    d.ctx.document.exitFullscreen=()=>{exits++;d.ctx.document.fullscreenElement=null;return Promise.resolve();};
    d.ctx.updateFullscreenButton();assert.equal(d.element('dashboard-fullscreen').hidden,false);
    await d.ctx.toggleDashboardFullscreen();assert.equal(requests,1);assert.equal(d.element('dashboard-fullscreen').textContent,'Exit full screen');
    await d.ctx.toggleDashboardFullscreen();assert.equal(exits,1);assert.equal(d.element('dashboard-fullscreen').textContent,'Full screen');
    await d.ctx.toggleDashboardFullscreen();d.ctx.document.fullscreenElement=null;d.ctx.updateFullscreenButton();
    assert.equal(d.element('dashboard-fullscreen').textContent,'Full screen');
});

test('unsupported fullscreen stays hidden and rejected requests report failure', async () => {
    const d=dashboard();d.ctx.updateFullscreenButton();assert.equal(d.element('dashboard-fullscreen').hidden,true);
    let notice='';d.ctx.alert=text=>{notice=text;};
    d.ctx.document.documentElement={requestFullscreen(){return Promise.reject(Error('blocked'));}};
    await d.ctx.toggleDashboardFullscreen();assert.match(notice,/unavailable/);
});


test('printer Zoom fills the viewport with the same card and Unzoom restores it', () => {
    const d=dashboard(), original=d.cards.get(1);
    d.ctx.togglePrinterZoom(1);
    assert(d.classes.has('printer-zoom-open'));assert(original.names.has('printer-zoomed'));
    assert.equal(d.element('zoom-1').textContent,'Unzoom');
    assert.equal(d.cards.get(1),original);assert.equal(d.fetched.length,0);
    d.ctx.togglePrinterZoom(1);
    assert(!d.classes.has('printer-zoom-open'));assert(!original.names.has('printer-zoomed'));
    assert.equal(d.element('zoom-1').textContent,'Zoom');
});

test('printer zoom survives a card rebuild and exits if the printer is removed', () => {
    const d=dashboard();d.ctx.togglePrinterZoom(1);
    const rebuilt=d.replaceCard(1);d.ctx.refreshPrinterZoom();assert(rebuilt.names.has('printer-zoomed'));
    assert.equal(d.element('zoom-1').textContent,'Unzoom');
    d.ctx.printers=d.ctx.printers.filter(p=>p.config.id!==1);d.ctx.refreshPrinterZoom();
    assert(!d.classes.has('printer-zoom-open'));assert.equal(d.element('zoom-1').textContent,'Zoom');
});

test('native fullscreen exit also restores the printer zoom view', async () => {
    const d=dashboard();const root={requestFullscreen(){d.ctx.document.fullscreenElement=root;return Promise.resolve();}};
    d.ctx.document.documentElement=root;
    d.ctx.togglePrinterZoom(1);await settle();assert(d.classes.has('printer-zoom-open'));
    d.ctx.document.fullscreenElement=null;d.ctx.printerZoomFullscreenChanged();
    assert(!d.classes.has('printer-zoom-open'));assert.equal(d.element('zoom-1').textContent,'Zoom');
});

test('rejected fullscreen retains full-card zoom and a late request cannot reopen it', async () => {
    const d=dashboard();d.ctx.document.documentElement={requestFullscreen(){return Promise.reject(Error('unsupported'));}};
    d.ctx.togglePrinterZoom(1);await settle();assert(d.classes.has('printer-zoom-open'));d.ctx.closePrinterZoom();
    let resolve;const root={requestFullscreen(){return new Promise(r=>{resolve=r;});}};
    d.ctx.document.documentElement=root;d.ctx.document.exitFullscreen=()=>{d.ctx.document.fullscreenElement=null;return Promise.resolve();};
    d.ctx.togglePrinterZoom(1);d.ctx.closePrinterZoom();d.ctx.document.fullscreenElement=root;resolve();await settle();
    assert.equal(d.ctx.document.fullscreenElement,null);assert(!d.classes.has('printer-zoom-open'));
});


test('diagnostics compact format separates loss from successful latency', () => {
    const d = dashboard();
    const samples = Array.from({length:8}, (_,i)=>({ms:18+i%3})).concat([{error:'No ping reply'},{error:'Host unreachable'}]);
    assert.equal(d.ctx.formatDiagnosticSamples('Printer', samples), 'Printer: 18.9 ms avg · 18–20 ms · 2/10 lost');
    assert.equal(d.ctx.formatDiagnosticSamples('Camera', [{same_host:true}]), 'Camera: Same host');
    assert.equal(d.ctx.formatDiagnosticSamples('Printer', Array(10).fill({error:'Host unreachable'})), 'Printer: Host unreachable · 10/10 lost');
});

test('diagnostics repeats ten-sample batches and stops requests independently', async () => {
    const d = dashboard(), timers = [], requests = [];
    d.ctx.setTimeout = callback => {timers.push(callback); return timers.length;};
    d.ctx.fetch = async (url, options) => {requests.push({url,signal:options.signal}); return {ok:true,json:async()=>({printer:{ms:20},camera:{same_host:true}})};};
    d.ctx.toggleDiagnostics(1); await settle();
    for(let i=1;i<10;i++){timers.shift()();await settle();}
    assert.equal(requests.length,10);
    assert.equal(vm.runInContext('printerDiagnostics.get(1).result',d.ctx),'Printer: 20 ms avg · 20–20 ms | Camera: Same host');
    timers.shift()();await settle();
    assert.equal(requests.length,11);
    d.ctx.toggleDiagnostics(2);await settle();
    d.ctx.toggleDiagnostics(1);
    assert(requests[0].signal.aborted);
    assert(!requests.at(-1).signal.aborted);
    assert(!vm.runInContext('printerDiagnostics.has(1)',d.ctx));
    d.ctx.printers=[];d.ctx.refreshDiagnostics();
    assert(requests.at(-1).signal.aborted);
});

test('turning diagnostics off ignores late replies and schedules no more pings', async () => {
    const d = dashboard(); let resolveFetch, scheduled=0;
    d.ctx.setTimeout=()=>{scheduled++;};
    d.ctx.fetch=()=>new Promise(resolve=>{resolveFetch=resolve;});
    d.ctx.toggleDiagnostics(1);d.ctx.toggleDiagnostics(1);
    resolveFetch({ok:true,json:async()=>({printer:{ms:20}})});await settle();
    assert.equal(scheduled,0);
    assert(!vm.runInContext('printerDiagnostics.has(1)',d.ctx));
});
