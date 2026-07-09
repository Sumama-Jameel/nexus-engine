// Nexus Dashboard — Frontend (V10)
//
// Reads ~/.nexus/state.json via Tauri IPC and renders a tabbed dashboard.
// All mutations go through `exec_nexus_command` (which shells out to the
// Go engine sidecar with whitelisted subcommands).

const invoke = window.__TAURI__?.core?.invoke;
if (!invoke) {
    document.getElementById('content').innerHTML =
        '<div class="empty-state"><h3>Tauri IPC unavailable</h3><p>This page must be loaded inside the Nexus Dashboard Tauri shell.</p></div>';
    throw new Error('Tauri invoke not available');
}

const state = {
    nexus: null,        // parsed ~/.nexus/state.json
    activeTab: 'sync',
    busy: false,
    lastError: null,
    modesList: null,    // cached result of `mode_list`; null = not loaded
};

// ─── DOM refs ───
const $content = document.getElementById('content');
const $tabs = document.getElementById('tabs');
const $errorBar = document.getElementById('error-bar');
const $errorText = document.getElementById('error-text');
const $errorDismiss = document.getElementById('error-dismiss');
const $refreshBtn = document.getElementById('refresh-btn');
const $statusDot = document.getElementById('status-dot');
const $statusText = document.getElementById('status-text');
const $headerModeBadge = document.getElementById('header-mode-badge');
const $headerModeName = $headerModeBadge ? $headerModeBadge.querySelector('.mode-name') : null;

// ─── Event wiring ───
$tabs.addEventListener('click', (e) => {
    const btn = e.target.closest('.tab');
    if (!btn) return;
    state.activeTab = btn.dataset.tab;
    $tabs.querySelectorAll('.tab').forEach(t => t.classList.toggle('active', t === btn));
    render();
});

$refreshBtn.addEventListener('click', () => refresh());
$errorDismiss.addEventListener('click', () => {
    state.lastError = null;
    $errorBar.classList.add('hidden');
});

// ─── Modal wiring (singleton, reused for any confirm flow) ───
const $confirmModal = document.getElementById('confirm-modal');
const $confirmTitle = document.getElementById('confirm-modal-title');
const $confirmBody = document.getElementById('confirm-modal-body');
const $confirmCancel = document.getElementById('confirm-cancel');
const $confirmApply = document.getElementById('confirm-apply');

let pendingConfirm = null; // { onConfirm: async () => void }

function showConfirm(title, bodyHtml, onConfirm) {
    pendingConfirm = { onConfirm };
    $confirmTitle.textContent = title;
    $confirmBody.innerHTML = bodyHtml;
    $confirmModal.classList.remove('hidden');
    $confirmApply.disabled = false;
    $confirmCancel.focus();
}

function hideConfirm() {
    pendingConfirm = null;
    $confirmModal.classList.add('hidden');
    $confirmApply.disabled = false;
}

$confirmCancel.addEventListener('click', hideConfirm);
$confirmApply.addEventListener('click', async () => {
    if (!pendingConfirm) return;
    $confirmApply.disabled = true;
    try {
        await pendingConfirm.onConfirm();
    } finally {
        // Re-enable if the handler did not close the modal (e.g. error path).
        if (!$confirmModal.classList.contains('hidden')) {
            $confirmApply.disabled = false;
        }
    }
});
// Click on backdrop (but not the modal itself) cancels.
$confirmModal.addEventListener('click', (e) => {
    if (e.target === $confirmModal) hideConfirm();
});
document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !$confirmModal.classList.contains('hidden')) hideConfirm();
});

// ─── Status indicator ───
function setStatus(kind, text) {
    $statusDot.className = 'status-dot ' + kind;
    $statusText.textContent = text;
}

// ─── Error handling ───
function setError(msg) {
    state.lastError = msg;
    $errorText.textContent = msg;
    $errorBar.classList.remove('hidden');
}

// ─── Refresh: read state.json ───
async function refresh() {
    if (state.busy) return;
    state.busy = true;
    setStatus('busy', 'Loading…');
    $refreshBtn.disabled = true;
    try {
        const json = await invoke('read_state');
        state.nexus = JSON.parse(json);
        state.lastError = null;
        $errorBar.classList.add('hidden');
        renderActiveMode();
        setStatus('ready', 'Ready');
    } catch (e) {
        state.nexus = null;
        setError(String(e));
        setStatus('error', 'Error');
    } finally {
        state.busy = false;
        $refreshBtn.disabled = false;
        render();
    }
}

// ─── Execute nexus command via Rust backend ───
async function runNexus(command, filePath) {
    if (state.busy) return;
    state.busy = true;
    setStatus('busy', `Running ${command}…`);
    try {
        const args = filePath
            ? { command, filePath }
            : { command };
        const result = await invoke(
            filePath ? 'exec_nexus_with_path' : 'exec_nexus_command',
            args
        );
        if (!result.success) {
            setError(result.stderr || `nexus exited with code ${result.exit_code}`);
        } else {
            state.lastError = null;
            $errorBar.classList.add('hidden');
        }
    } catch (e) {
        setError(String(e));
    } finally {
        state.busy = false;
        setStatus('ready', 'Ready');
        // Refresh state after any command that may have changed it.
        await refresh();
    }
}

// ─── Renderers ───
function render() {
    if (!state.nexus) {
        $content.innerHTML = `
            <div class="empty-state">
                <h3>No nexus state found</h3>
                <p>Run <code class="mono">nexus init</code> from your terminal to initialize the state file,
                then refresh this dashboard.</p>
            </div>`;
        return;
    }

    switch (state.activeTab) {
        case 'sync':      renderSync();      break;
        case 'vault':     renderVault();     break;
        case 'system':    renderSystem();    break;
        case 'modes':     renderModes();     break;
        case 'containers': renderContainers(); break;
        case 'registry':  renderRegistry();  break;
    }
}

function field(label, value, opts = {}) {
    const display = value === '' || value == null
        ? '<span class="empty">not set</span>'
        : escapeHtml(String(value));
    return `
        <div class="field">
            <span class="field-label">${escapeHtml(label)}</span>
            <span class="field-value ${opts.empty ? 'empty' : ''}">${display}</span>
        </div>`;
}

function card(title, badge, body) {
    const badgeHtml = badge
        ? `<span class="card-badge badge-${badge.kind}">${escapeHtml(badge.text)}</span>`
        : '';
    return `
        <div class="card">
            <div class="card-header">
                <h2>${escapeHtml(title)}</h2>
                ${badgeHtml}
            </div>
            ${body}
        </div>`;
}

function renderSync() {
    const d = state.nexus.dotfiles || {};
    const source = d.source || '';
    const installed = !!d.installed;
    const hasSource = source !== '';

    const installedBadge = installed
        ? { kind: 'ok',   text: 'chezmoi installed' }
        : { kind: 'err',  text: 'chezmoi missing' };
    const sourceBadge = hasSource
        ? { kind: 'ok',   text: 'bound' }
        : { kind: 'warn', text: 'unbound' };

    const buttons = `
        <div class="actions">
            <button class="btn"              data-cmd="dotfiles status">Status</button>
            <button class="btn"              data-cmd="dotfiles diff">Diff</button>
            <button class="btn"              data-cmd="dotfiles pull">Pull</button>
            <button class="btn"              data-cmd="dotfiles push">Push</button>
            <button class="btn btn-secondary" data-cmd="dotfiles sync">Sync (pull+apply+push)</button>
        </div>`;

    $content.innerHTML = `
        ${card('Chezmoi', installedBadge, `
            ${field('Installed', installed ? 'yes' : 'no')}
            ${field('Version', d.version || '')}
        `)}
        ${card('Source Binding', sourceBadge, `
            ${field('Repository', source)}
            ${field('Initialized at', formatTime(d.initialized_at))}
            ${field('Last applied', formatTime(d.last_applied_at))}
            ${hasSource ? buttons : ''}
        `)}
        ${hasSource ? card('Sync History', null, `
            ${field('Last pushed', formatTime(d.last_pushed_at))}
            ${field('Last pulled', formatTime(d.last_pulled_at))}
            ${field('Last commit SHA', d.last_commit_sha || '')}
        `) : ''}
        ${card('Managed Files', null, `
            ${(d.managed_files || []).length === 0
                ? '<div class="empty-state"><p>No files managed yet. Run <code class="mono">nexus dotfiles add &lt;path&gt;</code> from your terminal.</p></div>'
                : (d.managed_files || []).map(p => field('', p)).join('')}
        `)}
    `;
    wireActionButtons();
}

function renderVault() {
    const v = (state.nexus.dotfiles && state.nexus.dotfiles.vault) || {};
    const initialized = !!v.initialized;
    const fileCount = v.file_count || (v.encrypted_files ? Object.keys(v.encrypted_files).length : 0);

    const initBadge = initialized
        ? { kind: 'ok',   text: 'initialized' }
        : { kind: 'warn', text: 'not initialized' };

    const initBtn = initialized
        ? `<button class="btn btn-secondary" data-cmd="vault status">Re-check</button>`
        : `<button class="btn" data-cmd="vault init">Init vault</button>`;

    const fileList = v.encrypted_files
        ? Object.entries(v.encrypted_files).map(([orig, enc]) => field('', `${orig} → ${enc.split('/').pop()}`)).join('')
        : '';

    $content.innerHTML = `
        ${card('Vault Status', initBadge, `
            ${field('Initialized', initialized ? 'yes' : 'no')}
            ${field('Public key (short)', v.public_key_short || '')}
            ${field('Private key path', v.key_path || '')}
            ${field('Keyring ID', v.keyring_id || '')}
            ${field('Created', formatTime(v.created_at))}
            <div class="actions">${initBtn}
                <button class="btn btn-secondary" data-cmd="vault list">List files</button>
            </div>
        `)}
        ${initialized && fileCount > 0 ? `
            <div class="card">
                <div class="card-header"><h2>Encrypted Files (${fileCount})</h2></div>
                ${fileList || '<div class="empty-state"><p>No files encrypted yet.</p></div>'}
            </div>` : ''}
        ${initialized ? `
            <div class="card">
                <div class="card-header"><h2>Add a file</h2></div>
                <p style="color: var(--text-secondary); font-size: 13px; margin-bottom: 12px;">
                    Encrypt a sensitive file (SSH key, GPG key, credentials) for storage in the dotfile repo.
                    The file is encrypted with age and stored as <code class="mono">.age</code> in the chezmoi source dir.
                </p>
                <div style="display: flex; gap: 8px;">
                    <input type="text" id="vault-add-path" placeholder="/home/user/.ssh/id_rsa"
                        style="flex: 1; padding: 8px; background: var(--bg-tertiary); border: 1px solid var(--border);
                        color: var(--text-primary); border-radius: 4px; font-family: 'SF Mono', Menlo, monospace; font-size: 12px;" />
                    <button class="btn" id="vault-add-btn">Encrypt</button>
                </div>
            </div>` : ''}
    `;
    wireActionButtons();

    const addBtn = document.getElementById('vault-add-btn');
    if (addBtn) {
        addBtn.addEventListener('click', async () => {
            const path = document.getElementById('vault-add-path').value.trim();
            if (!path) {
                setError('Please enter a file path');
                return;
            }
            await runNexus('dotfiles vault add', path);
        });
    }
}

function renderSystem() {
    // System info comes from `nexus probe --json`. We don't have it
    // cached in state.json, so we run the command on first visit and
    // cache the result for the session.
    if (!state.systemInfo) {
        $content.innerHTML = '<div class="loading">Probing system…</div>';
        invoke('exec_nexus_command', { command: 'probe' })
            .then(r => {
                if (r.success) {
                    try {
                        state.systemInfo = parseProbeOutput(r.stdout);
                    } catch (e) {
                        state.systemInfo = { raw: r.stdout };
                    }
                } else {
                    state.systemInfo = { error: r.stderr || 'probe failed' };
                }
                renderSystem();
            })
            .catch(e => {
                state.systemInfo = { error: String(e) };
                renderSystem();
            });
        return;
    }

    const s = state.systemInfo;
    if (s.error) {
        $content.innerHTML = `
            <div class="card">
                <div class="card-header"><h2>System</h2><span class="card-badge badge-err">error</span></div>
                <p style="color: var(--error); font-family: 'SF Mono', Menlo, monospace; font-size: 12px;">${escapeHtml(s.error)}</p>
                <div class="actions">
                    <button class="btn btn-secondary" id="probe-retry">Retry</button>
                </div>
            </div>`;
        document.getElementById('probe-retry').addEventListener('click', () => {
            delete state.systemInfo;
            renderSystem();
        });
        return;
    }

    $content.innerHTML = `
        ${card('System', null, `
            ${field('Hostname', s.hostname)}
            ${field('OS', s.os)}
            ${field('Kernel', s.kernel)}
            ${field('Architecture', s.arch)}
            ${field('Shell', s.shell)}
            ${field('Package manager', s.package_manager)}
        `)}
        ${s.cpu ? card('CPU', null, `
            ${field('Model', s.cpu.model)}
            ${field('Cores', s.cpu.cores)}
            ${field('Architecture', s.cpu.arch)}
        `) : ''}
        ${s.memory ? card('Memory', null, `
            ${field('Total', formatBytes(s.memory.total))}
            ${field('Available', formatBytes(s.memory.available))}
            ${field('Used', s.memory.used_percent ? `${s.memory.used_percent}%` : '')}
        `) : ''}
        ${s.disk ? card('Disk', null, `
            ${field('Total', formatBytes(s.disk.total))}
            ${field('Free', formatBytes(s.disk.free))}
            ${field('Used', s.disk.used_percent ? `${s.disk.used_percent}%` : '')}
        `) : ''}
    `;
}

// Parse the plain-text output of `nexus probe` into a structured object.
// The Go engine prints lines like "OS: linux" or "CPU cores: 8". This
// is a best-effort parser — if the format changes, we fall back to raw.
function parseProbeOutput(text) {
    const result = {};
    const lines = text.split('\n');
    for (const line of lines) {
        const m = line.match(/^([A-Z][A-Za-z ]+):\s*(.+)$/);
        if (!m) continue;
        const key = m[1].trim().toLowerCase().replace(/\s+/g, '_');
        const val = m[2].trim();
        if (key === 'os') result.os = val;
        else if (key === 'kernel') result.kernel = val;
        else if (key === 'arch' || key === 'architecture') result.arch = val;
        else if (key === 'hostname') result.hostname = val;
        else if (key === 'shell') result.shell = val;
        else if (key === 'package_manager') result.package_manager = val;
        else if (key === 'cpu_cores') result.cpu = { ...(result.cpu || {}), cores: val };
        else if (key === 'cpu_model') result.cpu = { ...(result.cpu || {}), model: val };
        else if (key === 'memory_total') result.memory = { ...(result.memory || {}), total: parseBytes(val) };
        else if (key === 'memory_available') result.memory = { ...(result.memory || {}), available: parseBytes(val) };
        else if (key === 'disk_total') result.disk = { ...(result.disk || {}), total: parseBytes(val) };
        else if (key === 'disk_free') result.disk = { ...(result.disk || {}), free: parseBytes(val) };
    }
    return result;
}

function parseBytes(s) {
    const m = String(s).match(/([\d.]+)\s*(B|KB|MB|GB|TB|KiB|MiB|GiB|TiB)?/i);
    if (!m) return 0;
    const n = parseFloat(m[1]);
    const unit = (m[2] || 'B').toUpperCase();
    const mult = { B: 1, KB: 1024, MB: 1024**2, GB: 1024**3, TB: 1024**4,
                   KIB: 1024, MIB: 1024**2, GIB: 1024**3, TIB: 1024**4 }[unit] || 1;
    return Math.round(n * mult);
}

function formatBytes(n) {
    if (!n) return '';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
    return `${v.toFixed(v < 10 ? 2 : 1)} ${units[i]}`;
}

// ─── Modes (V11) ───
//
// One-click profile switcher. Reads the active mode from state.json (fast path
// for the header badge), lists all available modes (built-ins + user-defined)
// on demand via `mode_list`, and applies a mode via the dedicated `mode_apply`
// Tauri command after a two-click confirmation modal.
//
// Flow:
//   1. User picks a mode from the dropdown → Apply Mode button enables.
//   2. User clicks Apply Mode → modal shows the service plan → [Cancel] [Apply].
//   3. User clicks Apply → mode_apply invoked → on success, refresh state and
//      the modes list (active markers update).

function renderModes() {
    // Header badge reflects state.nexus.mode — always cheap, no IPC.
    renderActiveMode();

    if (state.modesList === null) {
        $content.innerHTML = '<div class="loading">Loading modes…</div>';
        loadModes();
        return;
    }

    const modes = state.modesList || [];
    const activeMode = (state.nexus && state.nexus.mode) || {};
    const activeName = activeMode.active || '';

    if (modes.length === 0) {
        $content.innerHTML = `
            <div class="mode-empty">
                <h3>No modes available</h3>
                <p>Three built-in modes ship with the engine. If you see this message, check your installation or run <code class="mono">nexus mode list</code> from your terminal.</p>
            </div>`;
        return;
    }

    const activeRecord = modes.find(m => m.name === activeName);
    const activeBadgeHtml = activeRecord
        ? renderModeBadge(activeRecord, true)
        : (activeName
            ? renderModeBadge({ name: activeName, description: 'Definition not found for the active mode.' }, true)
            : `<div class="mode-badge">
                    <div class="mode-badge-content">
                        <div class="mode-badge-name">no active mode</div>
                        <div class="mode-badge-desc">No mode is currently active. Pick one below to apply it.</div>
                    </div>
                </div>`);

    // Dropdown options: built-ins first (in the order returned), then user-defined.
    // The active option gets a marker but is still selectable so the user can
    // re-apply or use the modal to inspect the plan.
    const optionsHtml = modes.map(m => {
        const selected = m.name === activeName ? 'selected' : '';
        const marker = m.is_active ? ' (active)' : '';
        const userTag = m.source === 'built-in' ? '' : ' ★';
        return `<option value="${escapeHtml(m.name)}" ${selected}>${escapeHtml(m.name + marker + userTag)}</option>`;
    }).join('');

    const lastSwitchHtml = renderLastSwitch(activeMode);

    $content.innerHTML = `
        ${activeBadgeHtml}
        ${card('Switch Mode', null, `
            <p style="color: var(--text-secondary); font-size: 13px; margin-bottom: 12px;">
                Pick a mode and click <strong>Apply Mode</strong>. The engine will atomically switch
                the active profile, rebind dotfiles, and start or stop the listed services.
            </p>
            <select id="mode-select" class="mode-select" aria-label="Select mode">
                <option value="" disabled ${activeName ? '' : 'selected'}>-- choose a mode --</option>
                ${optionsHtml}
            </select>
            <div class="actions">
                <button class="btn" id="apply-mode-btn" disabled>Apply Mode</button>
            </div>
            ${lastSwitchHtml}
        `)}
    `;

    wireModesControls();
}

// Render a single mode's badge card. `isActive` toggles the accent border.
function renderModeBadge(m, isActive) {
    const desc = m.description || 'No description.';
    const tag = isActive ? '<span class="mode-badge-tag">active</span>' : '';
    return `
        <div class="mode-badge ${isActive ? 'active' : ''}">
            <div class="mode-badge-content">
                <div class="mode-badge-name">${escapeHtml(m.name)}</div>
                <div class="mode-badge-desc">${escapeHtml(desc)}</div>
            </div>
            ${tag}
        </div>`;
}

// "Last switched 3m ago (from dev)" — only renders when the engine has recorded a switch.
function renderLastSwitch(mode) {
    if (!mode || !mode.last_switch_at) return '';
    const rel = formatTime(mode.last_switch_at);
    if (!rel) return '';
    const fromPart = mode.last_switch_from
        ? ` (from <strong>${escapeHtml(mode.last_switch_from)}</strong>)`
        : '';
    return `<div class="last-switch">Last switched ${rel}${fromPart}</div>`;
}

// Update the header badge from state.nexus.mode. Called after every refresh().
function renderActiveMode() {
    if (!$headerModeBadge || !$headerModeName) return;
    const mode = (state.nexus && state.nexus.mode) || {};
    const active = mode.active || '';
    if (active) {
        $headerModeBadge.classList.remove('no-mode');
        $headerModeName.textContent = active;
        $headerModeBadge.title = `Active mode: ${active}`;
    } else {
        $headerModeBadge.classList.add('no-mode');
        $headerModeName.textContent = 'no mode';
        $headerModeBadge.title = 'No mode is currently active';
    }
}

// Fetch the full modes list and re-render if the Modes tab is visible.
async function loadModes() {
    try {
        const json = await invoke('mode_list');
        const parsed = JSON.parse(json);
        // Accept either a top-level array or a { modes: [...] } wrapper.
        state.modesList = Array.isArray(parsed) ? parsed : (parsed.modes || []);
        if (state.activeTab === 'modes') renderModes();
    } catch (e) {
        state.modesList = [];
        const msg = String(e);
        if (state.activeTab === 'modes') {
            $content.innerHTML = `
                <div class="card">
                    <div class="card-header"><h2>Modes</h2><span class="card-badge badge-err">error</span></div>
                    <p style="color: var(--error); font-family: 'SF Mono', Menlo, monospace; font-size: 12px;">${escapeHtml(msg)}</p>
                    <div class="actions">
                        <button class="btn btn-secondary" id="modes-retry">Retry</button>
                    </div>
                </div>`;
            const retry = document.getElementById('modes-retry');
            if (retry) retry.addEventListener('click', () => {
                state.modesList = null;
                renderModes();
            });
        }
        setError(msg);
    }
}

// Render a service list, or "(none)" for empty.
function formatServiceList(services) {
    if (!services || services.length === 0) {
        return `<div class="service-list none">(none)</div>`;
    }
    return `<div class="service-list">${services.map(escapeHtml).join(', ')}</div>`;
}

// Wire the dropdown change and Apply Mode button. Called after every renderModes().
function wireModesControls() {
    const $select = document.getElementById('mode-select');
    const $apply = document.getElementById('apply-mode-btn');
    if (!$select || !$apply) return;

    const activeName = ((state.nexus && state.nexus.mode) || {}).active || '';

    $select.addEventListener('change', () => {
        const selected = $select.value;
        // Enable only when a non-placeholder mode is picked AND it differs from active.
        $apply.disabled = !selected || selected === activeName;
    });

    $apply.addEventListener('click', () => {
        const name = $select.value;
        if (!name) return;
        const record = (state.modesList || []).find(m => m.name === name);
        confirmAndApplyMode(name, record);
    });
}

// Build the confirmation modal body and wire the Apply action.
function confirmAndApplyMode(name, record) {
    const stop = (record && record.stop_services) || [];
    const start = (record && record.start_services) || [];

    const bodyHtml = `
        <p>Apply mode <strong>${escapeHtml(name)}</strong>?</p>
        <p>This will stop:</p>
        ${formatServiceList(stop)}
        <p>This will start:</p>
        ${formatServiceList(start)}
        <p style="color: var(--text-muted); font-size: 12px; margin-top: 12px;">
            The profile, dotfile binding, and CPU governor will be updated atomically.
            If any step fails the engine will short-circuit without auto-rollback &mdash;
            run <code class="mono">nexus mode rollback</code> to revert.
        </p>`;

    showConfirm(`Apply mode '${name}'`, bodyHtml, async () => {
        if (state.busy) return;
        state.busy = true;
        setStatus('busy', `Applying ${name}…`);
        try {
            await invoke('mode_apply', { name });
            state.lastError = null;
            $errorBar.classList.add('hidden');
            hideConfirm();
            // Refresh state.json (updates header badge) and the modes list (updates active markers).
            await refresh();
            await loadModes();
        } catch (e) {
            setError(`mode apply failed: ${e}`);
            // Keep modal open so the user can retry or cancel.
        } finally {
            state.busy = false;
            setStatus('ready', 'Ready');
        }
    });
}

// ─── V16: Registry Tab ───
//
// Lists community profiles from the registry, lets the user search and fetch,
// and shows compatibility badges based on the current distro.

async function renderRegistry() {
    $content.innerHTML = '<div class="loading">Loading registry…</div>';
    try {
        const json = await invoke('exec_nexus_command', { command: 'registry list --json' });
        const parsed = JSON.parse(json);
        const profiles = parsed.profiles || [];
        renderRegistryContent(profiles);
    } catch (err) {
        $content.innerHTML = `
            <div class="empty-state">
                <h3>Registry unavailable</h3>
                <p>Could not fetch the community registry: ${escapeHtml(String(err))}</p>
                <button class="btn btn-secondary" onclick="renderRegistry()">Retry</button>
            </div>`;
    }
}

function renderRegistryContent(profiles) {
    if (profiles.length === 0) {
        $content.innerHTML = `
            <div class="tab-section">
                <h2>Profile Registry</h2>
                <div class="empty-state">
                    <p>No profiles in the community registry yet.</p>
                    <p>Submit your own with <code>nexus registry submit &lt;file&gt;</code>.</p>
                </div>
            </div>`;
        return;
    }

    const cards = profiles.map(p => {
        const families = (p.target_families || []).join(', ');
        return `
            <div class="registry-card">
                <div class="registry-card-header">
                    <h3>${escapeHtml(p.name)}</h3>
                    <span class="badge badge-neutral">v${escapeHtml(p.version)}</span>
                </div>
                <div class="registry-card-body">
                    <p class="registry-desc">${escapeHtml(p.description || 'No description')}</p>
                    <p class="registry-author">by ${escapeHtml(p.author || 'unknown')}</p>
                    ${families ? `<p class="registry-families">${escapeHtml(families)}</p>` : ''}
                </div>
                <div class="registry-card-actions">
                    <button class="btn btn-secondary btn-sm" onclick="registryFetch('${escapeHtml(p.name)}')">Fetch</button>
                </div>
            </div>`;
    }).join('');

    $content.innerHTML = `
        <div class="tab-section">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px;">
                <h2>Profile Registry (${profiles.length})</h2>
                <button class="btn btn-secondary" onclick="renderRegistry()">⟳ Refresh</button>
            </div>
            <div class="registry-grid">${cards}</div>
        </div>`;
}

async function registryFetch(name) {
    setStatus('busy', `Fetching ${name}…`);
    try {
        await invoke('exec_nexus_command', { command: `registry fetch ${name} --json` });
        setStatus('ready', 'Ready');
        showConfirm(`Fetched ${name}`, `
            <p>Profile <strong>${escapeHtml(name)}</strong> fetched and saved to your profile store.</p>
            <p>You can now apply it from the terminal:</p>
            <p><code>nexus profile apply ${escapeHtml(name)}</code></p>
            <p>Or click Apply to run it now:</p>
        `, async () => {
            await invoke('exec_nexus_command', { command: `profile apply ${name}` });
            hideConfirm();
            await refresh();
        });
    } catch (err) {
        setError(`Failed to fetch ${name}: ${err}`);
        setStatus('error', 'Error');
    }
}

// ─── Helpers ───
function escapeHtml(s) {
    return String(s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

function formatTime(iso) {
    if (!iso || iso.startsWith('0001')) return '';
    try {
        const d = new Date(iso);
        if (isNaN(d.getTime())) return iso;
        const now = new Date();
        const diff = (now - d) / 1000;
        if (diff < 60) return `${Math.floor(diff)}s ago`;
        if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
        if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`;
        if (diff < 2592000) return `${Math.floor(diff / 86400)}d ago`;
        return d.toLocaleDateString();
    } catch { return iso; }
}

function wireActionButtons() {
    $content.querySelectorAll('[data-cmd]').forEach(btn => {
        btn.addEventListener('click', () => runNexus(btn.dataset.cmd));
    });
}

// ─── Boot ───
refresh();

// ─── V12: Containers Tab ───

async function renderContainers() {
    $content.innerHTML = '<div class="loading">Loading containers...</div>';
    try {
        const json = await invoke('container_list');
        const data = JSON.parse(json);
        renderContainersContent(data);
    } catch (err) {
        showError('Failed to load containers: ' + err);
        $content.innerHTML = '<div class="empty-state">No containers found. Create one with `nexus container create <name> --image <image>`.</div>';
    }
}

function renderContainersContent(data) {
    const containers = data.containers || [];
    if (containers.length === 0) {
        $content.innerHTML = `
            <div class="tab-section">
                <h2>Containers</h2>
                <div class="empty-state">
                    <p>No Distrobox containers yet.</p>
                    <p>Create one with: <code>nexus container create &lt;name&gt; --image &lt;image&gt;</code></p>
                    <p>Example: <code>nexus container create fedora --image fedora:39</code></p>
                </div>
            </div>
        `;
        return;
    }
    const cards = containers.map(c => `
        <div class="container-card ${c.managed ? 'managed' : 'unmanaged'}">
            <div class="container-card-header">
                <h3>${escapeHtml(c.name)}</h3>
                <span class="badge ${c.managed ? 'badge-success' : 'badge-neutral'}">${c.managed ? 'managed' : 'unmanaged'}</span>
            </div>
            <div class="container-card-body">
                <p><strong>Status:</strong> ${escapeHtml(c.status || 'unknown')}</p>
                <p><strong>Image:</strong> ${escapeHtml(c.image || 'unknown')}</p>
            </div>
            <div class="container-card-actions">
                ${c.managed ? `<button class="btn btn-danger btn-sm" onclick="removeContainer('${escapeHtml(c.name)}')">Remove</button>` : ''}
            </div>
        </div>
    `).join('');
    $content.innerHTML = `
        <div class="tab-section">
            <h2>Containers</h2>
            <p class="section-desc">Distrobox containers. Managed containers were created by Nexus.</p>
            <div class="container-grid">${cards}</div>
        </div>
    `;
}

async function removeContainer(name) {
    if (!await showConfirm({
        title: 'Remove Container',
        body: `Remove container <strong>${escapeHtml(name)}</strong>? This will delete the container and its data.`,
        applyLabel: 'Remove',
        cancelLabel: 'Cancel'
    })) {
        return;
    }
    try {
        setStatus('busy', `Removing ${name}...`);
        await invoke('container_remove', { name });
        setStatus('ready', 'Ready');
        await renderContainers();
    } catch (err) {
        setStatus('error', 'Error');
        showError('Failed to remove container: ' + err);
    }
}

// Wire tabs into the tab switcher — containers and registry need async loading.
const _origSwitchTab = window.switchTab || null;
window.switchTab = function(tab) {
    if (tab === 'containers') {
        renderContainers();
    } else if (tab === 'registry') {
        renderRegistry();
    } else if (_origSwitchTab) {
        _origSwitchTab(tab);
    }
};
