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

        return `
            <div class="service-card">
                <div class="service-left">
                    <div class="service-icon ${svc.type}">${icon}</div>
                    <div class="service-info">
                        <div class="service-name">${esc(svc.name)}</div>
                        <a class="service-domain" href="${esc(svc.url)}" target="_blank">${esc(svc.domain)}</a>
                    </div>
                </div>
                <div class="service-right">
                    <span class="badge ${svc.type}">${esc(svc.type)}</span>
                    <span class="badge ${statusBadge}">${statusLabel}</span>
                    <a class="btn-open" href="${esc(svc.url)}" target="_blank">
                        Open ↗
                    </a>
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

function esc(str) {
    if (!str) return '';
    const d = document.createElement('div');
    d.textContent = str;
    return d.innerHTML;
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
