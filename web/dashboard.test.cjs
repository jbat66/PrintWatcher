const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

function dashboard() {
    const elements = new Map(), classes = new Set(), played = [], fetched = [];
    const element = id => {
        if (!elements.has(id)) elements.set(id, {hidden: false, textContent: '', setAttribute() {}, appendChild() {}});
        return elements.get(id);
    };
    const wrapper = {classList: {add: v => classes.add(v), remove: v => classes.delete(v)}, querySelector: () => element('remaining')};
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
        window: {AudioContext: Context},
        document: {
            body: {classList}, getElementById: element,
            querySelector: selector => selector.includes('.webcam-wrapper') ? wrapper : {focus() {}},
            querySelectorAll: selector => selector === '.camera-focused' && classes.has('camera-focused') ? [wrapper] : [],
            addEventListener() {}, createElement: () => ({setAttribute() {}})
        },
        printers: [1, 2].map(id => ({config: {id, enabled: true, notification_sound: id === 1 ? 'Cuckoo.mp3' : 'RedAlert.mp3'}, status: {state: 'printing', job: {remaining_secs: 600}}})),
        getWebcamMode: () => 'live', formatTime: value => `${value}s`,
        Date: {now: () => now}, setInterval() {},
        fetch: async url => { fetched.push(url); return {ok: true, arrayBuffer: async () => url}; }
    };
    vm.createContext(ctx);
    const app = fs.readFileSync(__dirname + '/app.js', 'utf8');
    vm.runInContext(app.slice(app.indexOf('// Camera focus uses'), app.indexOf('// Initialize')), ctx);
    return {ctx, played, fetched, classes, element, advance: n => {now += n;}, pending: id => vm.runInContext(`printerAlerts.get(${id}).pending`, ctx)};
}

const settle = () => new Promise(resolve => setImmediate(resolve));

test('camera double click toggles focus and retains live remaining time', () => {
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
