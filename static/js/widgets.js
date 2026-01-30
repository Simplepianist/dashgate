        // widgets.js - Weather, notes, and quick links widgets

        // Weather Widget
        let weatherLocation = (() => { try { return JSON.parse(localStorage.getItem('dashgate-weather-location') || 'null'); } catch { return null; } })();
        // Migrate from old city-only storage
        if (!weatherLocation && localStorage.getItem('dashgate-weather-city')) {
            const oldCity = localStorage.getItem('dashgate-weather-city');
            weatherLocation = { search: oldCity };
            localStorage.removeItem('dashgate-weather-city');
        }

        function initWeatherWidget() {
            if (weatherLocation && weatherLocation.lat != null) {
                fetchWeather();
            } else if (weatherLocation && weatherLocation.search) {
                searchWeatherCity(weatherLocation.search);
            }
        }

        function showWeatherSetup(prefill, errorMsg) {
            const content = document.getElementById('weatherContent');
            content.innerHTML = `
                <div class="weather-setup">
                    <input type="text" id="weatherCity" placeholder="Enter city name..." value="${escapeHtml(prefill || '')}">
                    <button onclick="searchWeatherCity()">Set Location</button>
                    ${errorMsg ? `<span style="color: var(--red); font-size: 12px; margin-top: 4px;">${escapeHtml(errorMsg)}</span>` : ''}
                </div>
            `;
        }

        async function searchWeatherCity(cityOverride) {
            const city = cityOverride || document.getElementById('weatherCity')?.value.trim();
            if (!city) return;

            const content = document.getElementById('weatherContent');
            content.innerHTML = '<div class="weather-desc">Searching...</div>';

            try {
                const geoResp = await fetch(`https://geocoding-api.open-meteo.com/v1/search?name=${encodeURIComponent(city)}&count=5`);
                const geoData = await geoResp.json();

                if (!geoData.results || geoData.results.length === 0) {
                    showWeatherSetup(city, 'City not found');
                    return;
                }

                // Deduplicate results that resolve to the same place
                const seen = new Set();
                const unique = geoData.results.filter(r => {
                    const key = `${r.latitude.toFixed(2)},${r.longitude.toFixed(2)}`;
                    if (seen.has(key)) return false;
                    seen.add(key);
                    return true;
                });

                if (unique.length === 1) {
                    // Only one match, use it directly
                    selectWeatherLocation(unique[0]);
                    return;
                }

                // Multiple matches — show picker
                content.innerHTML = `
                    <div class="weather-setup">
                        <div style="font-size: 12px; color: var(--text-secondary); margin-bottom: 4px;">Multiple matches for "${escapeHtml(city)}":</div>
                        <select id="weatherLocationSelect" class="admin-input" style="font-size: 13px; padding: 8px 10px;">
                            ${unique.map((r, i) => {
                                const parts = [r.name];
                                if (r.admin1) parts.push(r.admin1);
                                parts.push(r.country);
                                return `<option value="${i}">${escapeHtml(parts.join(', '))}</option>`;
                            }).join('')}
                        </select>
                        <button onclick="pickWeatherLocation()">Use Selected</button>
                        <button onclick="showWeatherSetup('${escapeHtml(city)}')" style="background: var(--bg-tertiary); color: var(--text-secondary);">Back</button>
                    </div>
                `;
                window._weatherGeoResults = unique;
            } catch (e) {
                showWeatherSetup(city, 'Failed to search');
            }
        }

        function pickWeatherLocation() {
            const select = document.getElementById('weatherLocationSelect');
            if (!select || !window._weatherGeoResults) return;
            const result = window._weatherGeoResults[parseInt(select.value)];
            if (result) selectWeatherLocation(result);
        }

        function selectWeatherLocation(geo) {
            const parts = [geo.name];
            if (geo.admin1) parts.push(geo.admin1);
            parts.push(geo.country);
            weatherLocation = {
                lat: geo.latitude,
                lon: geo.longitude,
                name: geo.name,
                region: geo.admin1 || '',
                country: geo.country,
                displayName: parts.join(', ')
            };
            localStorage.setItem('dashgate-weather-location', JSON.stringify(weatherLocation));
            delete window._weatherGeoResults;
            fetchWeather();
        }

        function changeWeatherLocation() {
            showWeatherSetup(weatherLocation?.name || '');
        }

        async function fetchWeather() {
            if (!weatherLocation || weatherLocation.lat == null) return;

            const content = document.getElementById('weatherContent');
            content.innerHTML = '<div class="weather-desc">Loading...</div>';

            try {
                const tempUnit = (settings && settings.temperatureUnit) || 'celsius';
                const weatherResp = await fetch(`https://api.open-meteo.com/v1/forecast?latitude=${weatherLocation.lat}&longitude=${weatherLocation.lon}&current=temperature_2m,weather_code&temperature_unit=${tempUnit}`);
                const weatherData = await weatherResp.json();

                const temp = Math.round(weatherData.current.temperature_2m);
                const weatherCode = weatherData.current.weather_code;
                const { icon, desc } = getWeatherDescription(weatherCode);
                const locDisplay = weatherLocation.displayName || `${weatherLocation.name}, ${weatherLocation.country}`;
                const unitSymbol = tempUnit === 'celsius' ? '°C' : '°F';

                content.innerHTML = `
                    <div class="weather-content">
                        <div class="weather-icon">${escapeHtml(icon)}</div>
                        <div class="weather-details">
                            <div class="weather-temp">${escapeHtml(String(temp))}${unitSymbol}</div>
                            <div class="weather-desc">${escapeHtml(desc)}</div>
                            <div class="weather-location" style="cursor: pointer; display: flex; align-items: center; gap: 4px;" onclick="changeWeatherLocation()" title="Change location">
                                ${escapeHtml(locDisplay)}
                                <svg width="10" height="10" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24" style="opacity: 0.5; flex-shrink: 0;">
                                    <path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7"/>
                                    <path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z"/>
                                </svg>
                            </div>
                        </div>
                    </div>
                `;

                localStorage.setItem('dashgate-weather-cache', JSON.stringify({
                    temp, icon, desc, location: locDisplay, unitSymbol, timestamp: Date.now()
                }));

            } catch (error) {
                const cached = localStorage.getItem('dashgate-weather-cache');
                if (cached) {
                    const data = JSON.parse(cached);
                    const cachedUnitSymbol = data.unitSymbol || '°F';
                    content.innerHTML = `
                        <div class="weather-content">
                            <div class="weather-icon">${escapeHtml(data.icon)}</div>
                            <div class="weather-details">
                                <div class="weather-temp">${escapeHtml(data.temp)}${cachedUnitSymbol}</div>
                                <div class="weather-desc">${escapeHtml(data.desc)}</div>
                                <div class="weather-location">${escapeHtml(data.location || '')}</div>
                            </div>
                        </div>
                    `;
                } else {
                    showWeatherSetup(weatherLocation?.name || '', 'Failed to load weather');
                }
            }
        }

        function getWeatherDescription(code) {
            const weatherCodes = {
                0: { icon: '☀️', desc: 'Clear sky' },
                1: { icon: '🌤️', desc: 'Mainly clear' },
                2: { icon: '⛅', desc: 'Partly cloudy' },
                3: { icon: '☁️', desc: 'Overcast' },
                45: { icon: '🌫️', desc: 'Foggy' },
                48: { icon: '🌫️', desc: 'Rime fog' },
                51: { icon: '🌧️', desc: 'Light drizzle' },
                53: { icon: '🌧️', desc: 'Drizzle' },
                55: { icon: '🌧️', desc: 'Heavy drizzle' },
                61: { icon: '🌧️', desc: 'Light rain' },
                63: { icon: '🌧️', desc: 'Rain' },
                65: { icon: '🌧️', desc: 'Heavy rain' },
                71: { icon: '🌨️', desc: 'Light snow' },
                73: { icon: '🌨️', desc: 'Snow' },
                75: { icon: '🌨️', desc: 'Heavy snow' },
                80: { icon: '🌦️', desc: 'Rain showers' },
                81: { icon: '🌦️', desc: 'Rain showers' },
                82: { icon: '⛈️', desc: 'Heavy showers' },
                95: { icon: '⛈️', desc: 'Thunderstorm' },
                96: { icon: '⛈️', desc: 'Thunderstorm with hail' },
                99: { icon: '⛈️', desc: 'Heavy thunderstorm' }
            };
            return weatherCodes[code] || { icon: '🌡️', desc: 'Unknown' };
        }

        // Notes Widget
        function initNotesWidget() {
            const notes = localStorage.getItem('dashgate-quick-notes') || '';
            const textarea = document.getElementById('quickNotes');
            if (textarea) {
                textarea.value = notes;
                updateNotesCharCount();
                textarea.addEventListener('input', () => {
                    updateNotesCharCount();
                    debouncedSaveNotes();
                });
            }
        }

        function updateNotesCharCount() {
            const textarea = document.getElementById('quickNotes');
            const count = document.getElementById('notesCharCount');
            if (textarea && count) {
                count.textContent = `${textarea.value.length} characters`;
            }
        }

        let notesTimeout;
        function debouncedSaveNotes() {
            clearTimeout(notesTimeout);
            notesTimeout = setTimeout(saveQuickNotes, 500);
        }

        function saveQuickNotes() {
            const textarea = document.getElementById('quickNotes');
            if (textarea) {
                localStorage.setItem('dashgate-quick-notes', textarea.value);
                const saved = document.getElementById('notesSaved');
                if (saved) {
                    saved.style.opacity = '1';
                    setTimeout(() => saved.style.opacity = '0', 1500);
                }
            }
        }

        // Quick Links Widget
        let quickLinks = (() => { try { return JSON.parse(localStorage.getItem('dashgate-quick-links') || '[]'); } catch { return []; } })();

        function initQuickLinksWidget() {
            renderQuickLinks();
        }

        function renderQuickLinks() {
            const container = document.getElementById('quickLinksList');
            if (!container) return;

            if (quickLinks.length === 0) {
                container.innerHTML = '<div class="quick-links-empty">No quick links yet</div>';
                return;
            }

            container.innerHTML = quickLinks.map((link, i) => {
                const safeColor = /^#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/.test(link.color) ? link.color : 'var(--accent)';
                return `
                <a href="${escapeHtml(link.url)}" target="_blank" rel="noopener" class="quick-link-item" data-index="${i}">
                    <div class="quick-link-icon" style="background: ${safeColor};">
                        <svg width="14" height="14" fill="white" viewBox="0 0 24 24">
                            <path d="M17 7h-4v2h4c1.65 0 3 1.35 3 3s-1.35 3-3 3h-4v2h4c2.76 0 5-2.24 5-5s-2.24-5-5-5zm-6 8H7c-1.65 0-3-1.35-3-3s1.35-3 3-3h4V7H7c-2.76 0-5 2.24-5 5s2.24 5 5 5h4v-2z"/>
                        </svg>
                    </div>
                    <span class="quick-link-name">${escapeHtml(link.name)}</span>
                    <button onclick="event.preventDefault(); event.stopPropagation(); deleteQuickLink(${i});" style="background: none; border: none; padding: 4px; cursor: pointer; opacity: 0.5;">
                        <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                            <path d="M18 6L6 18M6 6l12 12"/>
                        </svg>
                    </button>
                </a>
            `;
            }).join('');
        }

        function openQuickLinkModal() {
            const modal = document.createElement('div');
            modal.className = 'admin-modal open';
            modal.id = 'quickLinkModal';
            modal.setAttribute('role', 'dialog');
            modal.setAttribute('aria-modal', 'true');
            modal.setAttribute('aria-label', 'Quick link');
            modal.innerHTML = `
                <div class="admin-modal-backdrop" onclick="closeQuickLinkModal()"></div>
                <div class="admin-modal-content" style="max-width: 400px;">
                    <div class="admin-modal-header">
                        <h3>Add Quick Link</h3>
                        <button class="settings-close" onclick="closeQuickLinkModal()">
                            <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                                <path d="M18 6L6 18M6 6l12 12"/>
                            </svg>
                        </button>
                    </div>
                    <div class="admin-modal-body">
                        <div class="admin-form-group">
                            <label>Name</label>
                            <input type="text" class="admin-input" id="quickLinkName" placeholder="Link name">
                        </div>
                        <div class="admin-form-group">
                            <label>URL</label>
                            <input type="url" class="admin-input" id="quickLinkUrl" placeholder="https://...">
                        </div>
                        <div class="admin-form-group">
                            <label>Color</label>
                            <div class="color-picker">
                                <button type="button" class="color-option active" data-color="#0a84ff" style="--color: #0a84ff"></button>
                                <button type="button" class="color-option" data-color="#bf5af2" style="--color: #bf5af2"></button>
                                <button type="button" class="color-option" data-color="#30d158" style="--color: #30d158"></button>
                                <button type="button" class="color-option" data-color="#ff9f0a" style="--color: #ff9f0a"></button>
                                <button type="button" class="color-option" data-color="#ff375f" style="--color: #ff375f"></button>
                            </div>
                        </div>
                    </div>
                    <div class="admin-modal-footer">
                        <button class="settings-btn" onclick="closeQuickLinkModal()">Cancel</button>
                        <button class="settings-btn admin-btn-primary" onclick="saveQuickLink()">Add Link</button>
                    </div>
                </div>
            `;
            document.body.appendChild(modal);

            // Color picker functionality
            modal.querySelectorAll('.color-option').forEach(btn => {
                btn.addEventListener('click', (e) => {
                    e.preventDefault();
                    modal.querySelectorAll('.color-option').forEach(b => b.classList.remove('active'));
                    btn.classList.add('active');
                });
            });
        }

        function closeQuickLinkModal() {
            const modal = document.getElementById('quickLinkModal');
            if (modal) modal.remove();
        }

        function saveQuickLink() {
            const name = document.getElementById('quickLinkName').value.trim();
            const url = document.getElementById('quickLinkUrl').value.trim();
            const activeColor = document.querySelector('#quickLinkModal .color-option.active');
            const color = activeColor ? activeColor.dataset.color : '#0a84ff';

            if (!name || !url) {
                showToast('Please fill in all fields');
                return;
            }

            // Validate URL scheme - only allow http and https
            const urlLower = url.toLowerCase().trim();
            if (!urlLower.startsWith('http://') && !urlLower.startsWith('https://')) {
                showToast('URL must start with http:// or https://', 'error');
                return;
            }

            quickLinks.push({ name, url, color });
            localStorage.setItem('dashgate-quick-links', JSON.stringify(quickLinks));
            renderQuickLinks();
            closeQuickLinkModal();
            showToast('Link added');
        }

        function deleteQuickLink(index) {
            quickLinks.splice(index, 1);
            localStorage.setItem('dashgate-quick-links', JSON.stringify(quickLinks));
            renderQuickLinks();
            showToast('Link removed');
        }

        // Initialize widgets on page load
        document.addEventListener('DOMContentLoaded', () => {
            initWeatherWidget();
            initNotesWidget();
            initQuickLinksWidget();

            // Refresh weather every 30 minutes
            setInterval(fetchWeather, 30 * 60 * 1000);
        });
