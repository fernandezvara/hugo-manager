// Import dependencies
import Alpine from 'alpinejs';

// Import styles
import './style.css';

// Import the main app logic
import { createApp } from './main.js';

// Initialize Alpine with the app
window.Alpine = Alpine;
Alpine.data('app', createApp);
Alpine.start();
