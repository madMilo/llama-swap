/**
 * realtime.js - Minimal WebSocket/SSE client for htmz-style real-time updates
 *
 * Philosophy:
 * - Declarative: use data-ws-channel="channel.name" on elements
 * - Auto-connect based on data attributes
 * - Server sends HTML fragments, client injects
 * - No client-side state management
 * - Progressive enhancement (fallback to polling)
 */

(function() {
  'use strict';

  const RT = {
    ws: null,
    reconnectAttempts: 0,
    maxReconnectDelay: 30000,
    subscribers: new Map(), // channel -> Set of elements
    connected: false,
    reconnectTimer: null,

    // Initialize WebSocket connection
    init() {
      // Find all elements with data-ws-channel attribute
      this.scanSubscriptions();

      // Connect if we have subscribers
      if (this.subscribers.size > 0) {
        this.connect();
      }

      // Re-scan on htmz content updates
      document.addEventListener('htmz:afterSwap', () => {
        this.scanSubscriptions();
      });
    },

    // Scan DOM for subscription attributes
    scanSubscriptions() {
      document.querySelectorAll('[data-ws-channel]').forEach(el => {
        const channel = el.getAttribute('data-ws-channel');
        if (!channel) return;

        if (!this.subscribers.has(channel)) {
          this.subscribers.set(channel, new Set());
        }
        this.subscribers.get(channel).add(el);

        // Subscribe if connected
        if (this.connected) {
          this.subscribe(channel);
        }
      });
    },

    // Connect to WebSocket
    connect() {
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        return;
      }

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/api/ws`;

      try {
        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
          console.log('[realtime] WebSocket connected');
          this.connected = true;
          this.reconnectAttempts = 0;
          this.dispatchEvent('connected');

          // Subscribe to all channels
          this.subscribers.forEach((_, channel) => {
            this.subscribe(channel);
          });
        };

        this.ws.onmessage = (event) => {
          try {
            const message = JSON.parse(event.data);
            this.handleMessage(message);
          } catch (err) {
            console.error('[realtime] Failed to parse message:', err);
          }
        };

        this.ws.onerror = (error) => {
          console.error('[realtime] WebSocket error:', error);
        };

        this.ws.onclose = () => {
          console.log('[realtime] WebSocket closed');
          this.connected = false;
          this.dispatchEvent('disconnected');
          this.scheduleReconnect();
        };

      } catch (err) {
        console.error('[realtime] Failed to connect:', err);
        this.scheduleReconnect();
      }
    },

    // Subscribe to a channel
    subscribe(channel) {
      if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
        return;
      }

      this.ws.send(JSON.stringify({
        action: 'subscribe',
        channel: channel
      }));
    },

    // Unsubscribe from a channel
    unsubscribe(channel) {
      if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
        return;
      }

      this.ws.send(JSON.stringify({
        action: 'unsubscribe',
        channel: channel
      }));
    },

    // Handle incoming message
    handleMessage(message) {
      const { channel, target, action, content } = message;

      // Find all elements subscribed to this channel
      const subscribers = this.subscribers.get(channel);
      if (!subscribers) return;

      subscribers.forEach(el => {
        const targetSelector = target || el.getAttribute('data-ws-target');
        const targetEl = targetSelector ? el.querySelector(targetSelector) : el;

        if (!targetEl) return;

        // Perform action
        switch (action) {
          case 'append':
            targetEl.insertAdjacentHTML('beforeend', content);
            break;
          case 'prepend':
            targetEl.insertAdjacentHTML('afterbegin', content);
            break;
          case 'replace':
            targetEl.innerHTML = content;
            break;
          default:
            console.warn('[realtime] Unknown action:', action);
        }

        // Auto-scroll if enabled
        if (el.hasAttribute('data-ws-autoscroll')) {
          targetEl.scrollTop = targetEl.scrollHeight;
        }
      });
    },

    // Schedule reconnection with exponential backoff
    scheduleReconnect() {
      if (this.reconnectTimer) {
        clearTimeout(this.reconnectTimer);
      }

      const delay = Math.min(
        1000 * Math.pow(2, this.reconnectAttempts),
        this.maxReconnectDelay
      );

      console.log(`[realtime] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts + 1})`);

      this.reconnectTimer = setTimeout(() => {
        this.reconnectAttempts++;
        this.connect();
      }, delay);
    },

    // Dispatch custom events
    dispatchEvent(name) {
      document.dispatchEvent(new CustomEvent(`realtime:${name}`));
    }
  };

  // Auto-initialize on DOMContentLoaded
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => RT.init());
  } else {
    RT.init();
  }

  // Expose to window for manual control if needed
  window.RT = RT;

  // Connection status indicator handler
  const ConnectionStatus = {
    element: null,
    hideTimer: null,

    init() {
      this.element = document.getElementById('connection-status');
      if (!this.element) return;

      // Listen to realtime events
      document.addEventListener('realtime:connected', () => this.onConnected());
      document.addEventListener('realtime:disconnected', () => this.onDisconnected());
    },

    onConnected() {
      if (!this.element) return;

      this.element.className = 'connection-status connected';
      this.element.querySelector('.connection-status-text').textContent = 'Connected';
      this.element.style.display = 'flex';

      // Auto-hide after 2 seconds
      if (this.hideTimer) clearTimeout(this.hideTimer);
      this.hideTimer = setTimeout(() => {
        this.element.style.opacity = '0';
        setTimeout(() => {
          this.element.style.display = 'none';
          this.element.style.opacity = '1';
        }, 300);
      }, 2000);
    },

    onDisconnected() {
      if (!this.element) return;

      if (this.hideTimer) clearTimeout(this.hideTimer);

      this.element.className = 'connection-status disconnected';
      this.element.querySelector('.connection-status-text').textContent = 'Disconnected';
      this.element.style.display = 'flex';
      this.element.style.opacity = '1';
    }
  };

  // Initialize connection status
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => ConnectionStatus.init());
  } else {
    ConnectionStatus.init();
  }

})();
