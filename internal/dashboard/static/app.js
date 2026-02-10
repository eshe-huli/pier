// Pier Dashboard — Dev Hub

let allServices = [];

async function loadServices() {
    const grid = document.getElementById('services-grid');
    const empty = document.getElementById('empty-state');
    const pill = document.getElementById('status-pill');
    const statusText = document.getElementById('status-text');

    try {
        const resp = await fetch('/api/services');
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);

        const data = await resp.json();
        allServices = data.services || [];

        // Update status
        pill.className = 'status-pill ok';
        statusText.textContent = `${allServices.length} services`;

        // Update stats
        const running = allServices.filter(s => s.status === 'enabled' || s.status === 'running').length;
        const docker = allServices.filter(s => s.type === 'docker').length;
        const linked = allServices.filter(s => s.type === 'linked' || s.type === 'proxy').length;

        document.getElementById('stat-total').textContent = allServices.length;
        document.getElementById('stat-running').textContent = running;
        document.getElementById('stat-docker').textContent = docker;
        document.getElementById('stat-linked').textContent = linked;

        // Update last refresh
        document.getElementById('last-refresh').textContent = 
            `Updated ${new Date().toLocaleTimeString()}`;

        renderServices(allServices);

    } catch (err) {
        pill.className = 'status-pill error';
        statusText.textContent = 'Disconnected';
        grid.innerHTML = `
            <div class="loading-state">
                <p>Cannot connect to Pier</p>
                <p style="font-size: 12px; opacity: 0.5;">Make sure pier dashboard is running</p>
            </div>
        `;
    }
}

function renderServices(services) {
    const grid = document.getElementById('services-grid');
    const empty = document.getElementById('empty-state');

    // Apply search filter
    const query = document.getElementById('search').value.toLowerCase();
    const filtered = query 
        ? services.filter(s => 
            s.name.toLowerCase().includes(query) || 
            s.domain.toLowerCase().includes(query))
        : services;

    if (filtered.length === 0 && services.length === 0) {
        grid.style.display = 'none';
        empty.style.display = 'block';
        return;
    }

    grid.style.display = 'flex';
    empty.style.display = 'none';

    if (filtered.length === 0) {
        grid.innerHTML = `
            <div class="loading-state">
                <p>No services match "${esc(query)}"</p>
            </div>
        `;
        return;
    }

    grid.innerHTML = filtered.map(svc => {
        const icon = getIcon(svc.type);
        const isUp = svc.status === 'enabled' || svc.status === 'running';
        const statusBadge = isUp ? 'running' : 'stopped';
        const statusLabel = isUp ? 'Running' : 'Stopped';
        const isLinked = svc.type === 'linked';

        let actionBtn = '';
        if (isLinked && !isUp) {
            actionBtn = `<button class="btn-launch" onclick="launchService('${esc(svc.name)}')" title="Launch">▶ Launch</button>`;
        } else if (isLinked && isUp) {
            actionBtn = `<button class="btn-stop" onclick="stopService('${esc(svc.name)}')" title="Stop">■ Stop</button>`;
        }

        const dirLabel = svc.dir ? `<div class="service-dir" title="${esc(svc.dir)}">${esc(shortenPath(svc.dir))}</div>` : '';
        const fwBadge = svc.framework ? `<span class="badge framework">${esc(svc.framework)}</span>` : '';

        return `
            <div class="service-card${!isUp ? ' service-stopped' : ''}">
                <div class="service-left">
                    <div class="service-icon ${svc.type}">${icon}</div>
                    <div class="service-info">
                        <div class="service-name">${esc(svc.name)}</div>
                        <a class="service-domain" href="${esc(svc.url)}" target="_blank">${esc(svc.domain)}</a>
                        ${dirLabel}
                    </div>
                </div>
                <div class="service-right">
                    ${fwBadge}
                    <span class="badge ${svc.type}">${esc(svc.type)}</span>
                    <span class="badge ${statusBadge}">${statusLabel}</span>
                    ${actionBtn}
                    ${isUp ? `<a class="btn-open" href="${esc(svc.url)}" target="_blank">Open ↗</a>` : ''}
                </div>
            </div>
        `;
    }).join('');
}

function getIcon(type) {
    switch (type) {
        case 'docker': return '🐳';
        case 'linked': return '🔗';
        case 'proxy':  return '🔀';
        default:       return '📦';
    }
}

function shortenPath(p) {
    if (!p) return '';
    return p.replace(/^\/Users\/[^/]+/, '~');
}

function esc(str) {
    if (!str) return '';
    const d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
}

async function launchService(name) {
    try {
        const btn = event.target;
        btn.disabled = true;
        btn.textContent = '⏳ Starting...';
        
        const resp = await fetch('/api/services/start', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name }),
        });
        const data = await resp.json();
        
        if (!resp.ok) {
            alert(data.error || 'Failed to start');
            btn.disabled = false;
            btn.textContent = '▶ Launch';
            return;
        }

        // Refresh after a brief delay to let the server boot
        setTimeout(loadServices, 2000);
    } catch (err) {
        alert('Failed to start service: ' + err.message);
    }
}

async function stopService(name) {
    try {
        const resp = await fetch('/api/services/stop', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name }),
        });
        if (!resp.ok) {
            const data = await resp.json();
            alert(data.error || 'Failed to stop');
            return;
        }
        setTimeout(loadServices, 500);
    } catch (err) {
        alert('Failed to stop service: ' + err.message);
    }
}

// Search filter
document.getElementById('search').addEventListener('input', () => {
    renderServices(allServices);
});

// Keyboard shortcut: / to focus search
document.addEventListener('keydown', (e) => {
    if (e.key === '/' && document.activeElement.tagName !== 'INPUT') {
        e.preventDefault();
        document.getElementById('search').focus();
    }
    if (e.key === 'Escape') {
        document.getElementById('search').value = '';
        document.getElementById('search').blur();
        renderServices(allServices);
    }
});

// Initial load
loadServices();

// Auto-refresh every 5 seconds
setInterval(loadServices, 5000);
