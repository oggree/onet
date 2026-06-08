document.addEventListener('DOMContentLoaded', () => {
    
    // Status Check
    fetch('/api/status')
        .then(async res => {
            if (!res.ok) {
                const text = await res.text();
                throw new Error(text || 'Network Error');
            }
            return res.json();
        })
        .then(data => {
            if (data.is_set) {
                document.getElementById('dashboard-view').style.display = 'block';
                initDashboard();
            } else {
                document.getElementById('setup-wizard').style.display = 'block';
            }
        })
        .catch(err => console.error("Error fetching status:", err));

    // Setup Wizard Navigation
    document.getElementById('btn-has-domain').addEventListener('click', () => showStep(2));
    document.getElementById('btn-next-2').addEventListener('click', () => {
        const domain = document.getElementById('setup-domain').value.trim();
        if (domain) showStep(3);
        else alert("Please enter a valid domain.");
    });
    document.getElementById('btn-next-3').addEventListener('click', () => showStep(4));
    
    document.getElementById('btn-finish').addEventListener('click', () => {
        const domain = document.getElementById('setup-domain').value.trim();
        const token = document.getElementById('setup-token').value.trim();
        if (!token) {
            alert("Please enter your Cloudflare API Token.");
            return;
        }

        showStep('loading');

        fetch('/api/setup', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ domain, token })
        })
        .then(async res => {
            if (!res.ok) {
                const text = await res.text();
                throw new Error(text || 'Network Error');
            }
            return res.json();
        })
        .then(data => {
            if (data.success) {
                setTimeout(() => {
                    window.location.reload();
                }, 2000);
            } else {
                alert("Setup failed.");
                showStep(4);
            }
        })
        .catch(err => {
            console.error(err);
            alert("Error: " + err.message);
            showStep(4);
        });
    });

    // Clear Config Button (Dev)
    document.getElementById('btn-clear-config').addEventListener('click', () => {
        if (confirm("Are you sure you want to clear the configuration? This will stop services if restarted.")) {
            fetch('/api/clear', { method: 'POST' })
                .then(async res => {
                    if (!res.ok) {
                        const text = await res.text();
                        throw new Error(text || 'Network Error');
                    }
                    return res.json();
                })
                .then(data => {
                    if (data.success) {
                        alert("Configuration cleared. Reloading...");
                        window.location.reload();
                    }
                });
        }
    });

    function showStep(stepNum) {
        document.querySelectorAll('.wizard-step').forEach(el => el.style.display = 'none');
        if (stepNum === 'loading') {
            document.getElementById('setup-loading').style.display = 'block';
        } else {
            document.getElementById('step-' + stepNum).style.display = 'block';
        }
    }

    // Tab Navigation Logic
    const navLinks = document.querySelectorAll('.nav-link');
    const tabViews = document.querySelectorAll('.tab-view');

    navLinks.forEach(link => {
        link.addEventListener('click', (e) => {
            e.preventDefault();
            const targetTab = link.id.replace('nav-', '');

            // Update nav links active class
            navLinks.forEach(l => l.classList.remove('active'));
            link.classList.add('active');

            // Show target view, hide others
            tabViews.forEach(view => {
                if (view.id === `view-${targetTab}`) {
                    view.style.display = 'block';
                } else {
                    view.style.display = 'none';
                }
            });

            // Trigger specific tab loading logic
            if (targetTab === 'security') {
                loadTokens();
            } else if (targetTab === 'settings') {
                loadSettings();
            } else if (targetTab === 'tunnels') {
                loadTunnels();
            }
        });
    });

    // Uptime & Requests Metrics Simulation
    function initDashboard() {
        const requestsEl = document.getElementById('requests-count');
        let count = 0;
        
        setInterval(() => {
            const increase = Math.floor(Math.random() * 5);
            count += increase;
            
            if (increase > 0) {
                requestsEl.style.transform = 'scale(1.1)';
                requestsEl.style.color = '#fff';
                setTimeout(() => {
                    requestsEl.style.transform = 'scale(1)';
                    requestsEl.style.color = 'var(--accent)';
                }, 150);
            }
            
            requestsEl.textContent = count.toLocaleString();
        }, 2000);

        requestsEl.textContent = '0';
    }

    // Load active tunnel domain values
    function loadTunnels() {
        fetch('/api/config')
            .then(res => res.json())
            .then(config => {
                if (config.domain) {
                    document.getElementById('arc-hostname').textContent = `arc.${config.domain}`;
                    document.getElementById('wildcard-domain').textContent = config.domain;
                }
            })
            .catch(err => console.error("Error loading tunnels config:", err));
    }

    // SQLite Tokens Management
    const tokensTableBody = document.getElementById('tokens-table-body');
    const formAddToken = document.getElementById('form-add-token');
    const btnGenerateToken = document.getElementById('btn-generate-token');

    btnGenerateToken.addEventListener('click', () => {
        const chars = 'abcdef0123456789';
        let token = '';
        for (let i = 0; i < 32; i++) {
            token += chars.charAt(Math.floor(Math.random() * chars.length));
        }
        document.getElementById('token-val').value = token;
    });

    formAddToken.addEventListener('submit', (e) => {
        e.preventDefault();
        const organization = document.getElementById('token-org').value.trim();
        const token = document.getElementById('token-val').value.trim();

        fetch('/api/tokens/add', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ organization, token })
        })
        .then(res => {
            if (!res.ok) throw new Error("Failed to add token");
            return res.json();
        })
        .then(data => {
            if (data.success) {
                document.getElementById('token-org').value = '';
                document.getElementById('token-val').value = '';
                loadTokens();
            }
        })
        .catch(err => alert("Error: " + err.message));
    });

    window.removeToken = function(token) {
        if (confirm("Are you sure you want to revoke this token? Client agents using it will be disconnected.")) {
            fetch('/api/tokens/remove', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ token })
            })
            .then(res => {
                if (!res.ok) throw new Error("Failed to remove token");
                return res.json();
            })
            .then(data => {
                if (data.success) {
                    loadTokens();
                }
            })
            .catch(err => alert("Error: " + err.message));
        }
    };

    function loadTokens() {
        tokensTableBody.innerHTML = '<tr><td colspan="3" style="text-align: center;">Loading tokens...</td></tr>';
        fetch('/api/tokens')
            .then(res => res.json())
            .then(tokens => {
                tokensTableBody.innerHTML = '';
                if (!tokens || tokens.length === 0) {
                    tokensTableBody.innerHTML = '<tr><td colspan="3" style="text-align: center; color: var(--text-muted);">No remote tokens issued yet.</td></tr>';
                    return;
                }
                tokens.forEach(t => {
                    const row = document.createElement('tr');
                    row.innerHTML = `
                        <td><b>${escapeHtml(t.Organization)}</b></td>
                        <td style="font-family: monospace; color: var(--accent);">${escapeHtml(t.Token)}</td>
                        <td>
                            <button class="btn warning" style="padding: 0.25rem 0.75rem; font-size: 0.8rem;" onclick="removeToken('${escapeHtml(t.Token)}')">Revoke</button>
                        </td>
                    `;
                    tokensTableBody.appendChild(row);
                });
            })
            .catch(err => {
                console.error(err);
                tokensTableBody.innerHTML = '<tr><td colspan="3" style="text-align: center; color: #ef4444;">Error loading tokens.</td></tr>';
            });
    }

    // Settings Configuration Display & Audit
    function loadSettings() {
        fetch('/api/config')
            .then(res => res.json())
            .then(config => {
                document.getElementById('settings-domain').textContent = config.domain || 'Not configured';
                document.getElementById('settings-account-id').textContent = config.account_id || 'Not configured';
                document.getElementById('settings-zone-id').textContent = config.zone_id || 'Not configured';
            })
            .catch(err => console.error("Error loading settings config:", err));
    }

    const btnVerifyPermissions = document.getElementById('btn-verify-permissions');
    const permissionAuditContainer = document.getElementById('permission-audit-container');

    btnVerifyPermissions.addEventListener('click', () => {
        permissionAuditContainer.innerHTML = `
            <div style="text-align: center; padding: 2rem;">
                <div class="spinner"></div>
                <p style="color: var(--text-muted); margin-top: 1rem;">Auditing Cloudflare API Token scopes...</p>
            </div>
        `;

        fetch('/api/verify')
            .then(async res => {
                if (!res.ok) {
                    const text = await res.text();
                    throw new Error(text || 'Verification suite error');
                }
                return res.json();
            })
            .then(reports => {
                permissionAuditContainer.innerHTML = '';
                if (!reports || reports.length === 0) {
                    permissionAuditContainer.innerHTML = `
                        <div style="text-align: center; padding: 2rem; color: #ef4444;">
                            No verification results returned. Please ensure your token is configured.
                        </div>
                    `;
                    return;
                }

                reports.forEach(r => {
                    const item = document.createElement('div');
                    item.className = `audit-item ${r.passed ? 'passed' : 'failed'}`;
                    
                    const badgeClass = r.passed ? 'status-success' : 'status-failed';
                    const badgeText = r.passed ? 'Passed' : 'Failed';
                    const icon = r.passed ? '✓' : '✗';

                    let errorHtml = '';
                    if (!r.passed && r.error) {
                        errorHtml = `<div class="error-msg">${escapeHtml(r.error)}</div>`;
                    }

                    item.innerHTML = `
                        <div class="audit-info">
                            <h4>${escapeHtml(r.Name)}</h4>
                            <p>${escapeHtml(r.Description)}</p>
                            ${errorHtml}
                        </div>
                        <span class="badge ${badgeClass}" style="gap: 0.25rem;">
                            <span>${icon}</span>
                            <span>${badgeText}</span>
                        </span>
                    `;
                    permissionAuditContainer.appendChild(item);
                });
            })
            .catch(err => {
                console.error(err);
                permissionAuditContainer.innerHTML = `
                    <div style="text-align: center; padding: 2rem; color: #ef4444;">
                        Verification audit failed: ${escapeHtml(err.message)}
                    </div>
                `;
            });
    });

    function escapeHtml(str) {
        if (!str) return '';
        return str.replace(/&/g, "&amp;")
                  .replace(/</g, "&lt;")
                  .replace(/>/g, "&gt;")
                  .replace(/"/g, "&quot;")
                  .replace(/'/g, "&#039;");
    }
});
