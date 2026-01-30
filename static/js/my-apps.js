        // my-apps.js - Custom personal app management for DashGate

        function renderMyAppsList() {
            const container = document.getElementById('myAppsList');
            const empty = document.getElementById('myAppsEmpty');

            if (myApps.length === 0) {
                container.innerHTML = '';
                empty.style.display = '';
                return;
            }

            empty.style.display = 'none';
            container.innerHTML = myApps.map((app, i) => `
                <div class="favorites-item">
                    <div class="favorites-item-icon">
                        ${app.icon ? `<img src="${escapeHtml(app.icon)}" alt="" onerror="this.parentElement.innerHTML='<span style=font-weight:600;font-size:14px>${escapeHtml(app.name[0])}</span>'">` : `<span style="font-weight:600;font-size:14px">${escapeHtml(app.name[0])}</span>`}
                    </div>
                    <span class="favorites-item-name">${escapeHtml(app.name)}</span>
                    <button class="favorites-item-remove" onclick="openEditMyAppModal(${i})" title="Edit" style="color:var(--text-secondary);">
                        <svg width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                            <path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/>
                            <path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/>
                        </svg>
                    </button>
                    <button class="favorites-item-remove" onclick="deleteMyApp(${i})" title="Delete">
                        <svg width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                            <polyline points="3 6 5 6 21 6"/>
                            <path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2"/>
                        </svg>
                    </button>
                </div>
            `).join('');
        }

        function openMyAppModal() {
            document.getElementById('myAppModalTitle').textContent = 'Add Personal App';
            document.getElementById('myAppEditIndex').value = '';
            document.getElementById('myAppName').value = '';
            document.getElementById('myAppUrl').value = '';
            document.getElementById('myAppDesc').value = '';
            document.getElementById('myAppIcon').value = '';
            document.getElementById('myAppModal').classList.add('open');
        }

        function openEditMyAppModal(index) {
            const app = myApps[index];
            if (!app) return;

            document.getElementById('myAppModalTitle').textContent = 'Edit Personal App';
            document.getElementById('myAppEditIndex').value = index;
            document.getElementById('myAppName').value = app.name;
            document.getElementById('myAppUrl').value = app.url;
            document.getElementById('myAppDesc').value = app.description || '';
            document.getElementById('myAppIcon').value = app.icon || '';
            document.getElementById('myAppModal').classList.add('open');
        }

        function closeMyAppModal() {
            document.getElementById('myAppModal').classList.remove('open');
        }

        function saveMyApp() {
            const indexStr = document.getElementById('myAppEditIndex').value;
            const name = document.getElementById('myAppName').value.trim();
            const url = document.getElementById('myAppUrl').value.trim();
            const description = document.getElementById('myAppDesc').value.trim();
            const icon = document.getElementById('myAppIcon').value.trim();

            if (!name || !url) {
                showToast('Name and URL are required');
                return;
            }

            // Validate URL scheme - only allow http and https
            const urlLower = url.toLowerCase().trim();
            if (!urlLower.startsWith('http://') && !urlLower.startsWith('https://')) {
                showToast('URL must start with http:// or https://', 'error');
                return;
            }

            const app = { name, url, description, icon };

            if (indexStr !== '') {
                myApps[parseInt(indexStr)] = app;
                showToast('App updated');
            } else {
                myApps.push(app);
                showToast('App added');
            }

            localStorage.setItem('dashgate-my-apps', JSON.stringify(myApps)); debouncedSyncPreferences();
            closeMyAppModal();
            renderMyAppsList();
            renderMyAppsOnDashboard();
        }

        function deleteMyApp(index) {
            if (!confirm(`Delete "${myApps[index].name}"?`)) return;
            myApps.splice(index, 1);
            localStorage.setItem('dashgate-my-apps', JSON.stringify(myApps)); debouncedSyncPreferences();
            renderMyAppsList();
            renderMyAppsOnDashboard();
            showToast('App deleted');
        }

        function exportMyApps() {
            if (myApps.length === 0) {
                showToast('No apps to export');
                return;
            }
            const data = {
                myApps: myApps,
                exportedAt: new Date().toISOString(),
                version: 1
            };
            const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `my-apps-${new Date().toISOString().split('T')[0]}.json`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
            showToast('My Apps exported');
        }

        function importMyApps(event) {
            const file = event.target.files[0];
            if (!file) return;

            const reader = new FileReader();
            reader.onload = (e) => {
                try {
                    const data = JSON.parse(e.target.result);
                    if (data.myApps && Array.isArray(data.myApps)) {
                        myApps = data.myApps;
                        localStorage.setItem('dashgate-my-apps', JSON.stringify(myApps)); debouncedSyncPreferences();
                        renderMyAppsList();
                        renderMyAppsOnDashboard();
                        showToast(`Imported ${myApps.length} apps`);
                    } else {
                        showToast('Invalid file format');
                    }
                } catch (err) {
                    showToast('Failed to import');
                }
            };
            reader.readAsText(file);
            event.target.value = '';
        }

        // Render My Apps on DashGate
        function renderMyAppsOnDashboard() {
            let myAppsSection = document.getElementById('myAppsSection');

            if (myApps.length === 0) {
                if (myAppsSection) myAppsSection.remove();
                return;
            }

            if (!myAppsSection) {
                myAppsSection = document.createElement('section');
                myAppsSection.id = 'myAppsSection';
                myAppsSection.className = 'category';
                const firstCategory = document.querySelector('.category');
                if (firstCategory) {
                    firstCategory.parentNode.insertBefore(myAppsSection, firstCategory);
                }
            }

            const target = settings.openInNewTab ? '_blank' : '_self';
            myAppsSection.innerHTML = `
                <h2 class="category-title">My Apps</h2>
                <div class="apps-grid">
                    ${myApps.map(app => `
                        <a href="${escapeHtml(app.url)}" target="${target}" class="app-item" data-name="${escapeHtml(app.name)}" data-url="${escapeHtml(app.url)}" data-desc="${escapeHtml(app.description || '')}" data-status="unknown">
                            <div class="app-icon">
                                ${app.icon ? `<img src="${escapeHtml(app.icon)}" alt="${escapeHtml(app.name)}" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'"><span class="app-icon-fallback" style="display:none">${escapeHtml(app.name[0])}</span>` : `<span class="app-icon-fallback">${escapeHtml(app.name[0])}</span>`}
                            </div>
                            <span class="app-name">${escapeHtml(app.name)}</span>
                            <div class="app-status unknown"></div>
                        </a>
                    `).join('')}
                </div>
            `;

            // Context menu is handled by delegated listener on .main-content
        }

        // Initialize My Apps on page load
        document.addEventListener('DOMContentLoaded', () => {
            renderMyAppsOnDashboard();
        });
