// Auto-Icon Management
class AutoIconManager {
    constructor() {
        this.apiBase = '/api/admin/icons/selfhst';
    }

    async searchIcon(appUrl, appName) {
        try {
            const response = await fetch(`${this.apiBase}/search`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': this.getCSRFToken()
                },
                body: JSON.stringify({ appUrl, appName })
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Failed to search icon:', error);
            throw error;
        }
    }

    async autoUpdateAllIcons() {
        try {
            const response = await fetch(`${this.apiBase}/auto-update`, {
                method: 'POST',
                headers: {
                    'X-CSRF-Token': this.getCSRFToken()
                }
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            return await response.json();
        } catch (error) {
            console.error('Failed to auto-update icons:', error);
            throw error;
        }
    }

    getCSRFToken() {
        const cookies = document.cookie.split(';');
        for (let cookie of cookies) {
            const [name, value] = cookie.trim().split('=');
            if (name === 'dashgate_csrf') {
                return value;
            }
        }
        return '';
    }
}

// Global instance
window.autoIconManager = new AutoIconManager();

