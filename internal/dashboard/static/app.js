// Pier Dashboard — Fetches routes from Traefik API

const TRAEFIK_API = 'http://127.0.0.1:8881';

async function loadServices() {
    const list = document.getElementById('services-list');
    const empty = document.getElementById('empty-state');
    const statusEl = document.getElementById('status-traefik');

    try {
        const resp = await fetch(`${TRAEFIK_API}/api/http/routers`);
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        
        const routers = await resp.json();
        statusEl.textContent = `✅ Traefik connected (${routers.length} routes)`;
        statusEl.className = 'status-item ok';

        // Filter out internal Traefik routes
        const services = routers.filter(r => 
            !r.name.includes('api@internal') && 
            !r.name.includes('dashboard@internal') &&
            !r.name.includes('acme')
        );

        if (services.length === 0) {
            list.style.display = 'none';
            empty.style.display = 'block';
            return;
        }

        list.style.display = 'flex';
        empty.style.display = 'none';
        list.innerHTML = services.map(svc => renderService(svc)).join('');

    } catch (err) {
        statusEl.textContent = '❌ Cannot reach Traefik API';
        statusEl.className = 'status-item error';
        list.innerHTML = `<div class="loading">Cannot connect to Traefik at ${TRAEFIK_API}<br><small>Make sure Pier is running: <code>pier status</code></small></div>`;
    }
}

function renderService(router) {
    const domain = extractDomain(router.rule);
    const provider = router.provider || 'unknown';
    const type = provider.includes('docker') ? 'docker' : 
                 provider.includes('file') ? 'proxy' : provider;
    const status = router.status === 'enabled' ? 'enabled' : 'disabled';
    const statusIcon = status === 'enabled' ? '✅' : '❌';
    const name = router.name.split('@')[0];

    return `
        <div class="service-card">
            <div class="service-info">
                <span class="service-name">${escapeHtml(name)}</span>
                <a class="service-domain" href="http://${escapeHtml(domain)}" target="_blank">
                    ${escapeHtml(domain)}
                </a>
                <span class="service-meta">${escapeHtml(type)} • ${escapeHtml(router.service || '')}</span>
            </div>
            <div class="service-actions">
                <span class="service-status ${status}">${statusIcon} ${status}</span>
                <button class="btn-copy" onclick="copyDomain('${escapeHtml(domain)}')" title="Copy domain">
                    📋
                </button>
            </div>
        </div>
    `;
}

function extractDomain(rule) {
    if (!rule) return 'unknown';
    const match = rule.match(/Host\(`([^`]+)`\)/);
    return match ? match[1] : rule;
}

function copyDomain(domain) {
    navigator.clipboard.writeText(domain).then(() => {
        // Brief visual feedback
        const btn = event.target;
        const original = btn.textContent;
        btn.textContent = '✓';
        setTimeout(() => btn.textContent = original, 1000);
    });
}

function escapeHtml(str) {
    if (!str) return '';
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

// Load on page load
loadServices();

// Auto-refresh every 10 seconds
setInterval(loadServices, 10000);
