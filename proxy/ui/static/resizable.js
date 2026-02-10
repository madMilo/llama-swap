/**
 * resizable.js - Minimal resizable panels for htmz-style UIs
 *
 * Philosophy:
 * - Declarative: add class .resizable-container to enable
 * - Auto-initialize on page load
 * - Persist size in localStorage
 * - No global state
 *
 * HTML structure:
 * <div class="resizable-container" data-resize-key="unique-key">
 *   <div class="panel-left">...</div>
 *   <div class="resize-handle"></div>
 *   <div class="panel-right">...</div>
 * </div>
 */

(function() {
  'use strict';

  const Resizable = {
    activeHandle: null,
    startX: 0,
    startWidth: 0,
    container: null,
    leftPanel: null,
    storageKey: null,

    // Initialize all resizable containers
    init() {
      document.querySelectorAll('.resizable-container').forEach(container => {
        this.setupContainer(container);
      });

      // Re-initialize on htmz content updates
      document.addEventListener('htmz:afterSwap', () => {
        document.querySelectorAll('.resizable-container').forEach(container => {
          if (!container.dataset.resizableInit) {
            this.setupContainer(container);
          }
        });
      });
    },

    // Setup a single resizable container
    setupContainer(container) {
      const handle = container.querySelector('.resize-handle');
      const leftPanel = container.querySelector('.panel-left');
      const rightPanel = container.querySelector('.panel-right');

      if (!handle || !leftPanel || !rightPanel) {
        console.warn('[resizable] Container missing required elements:', container);
        return;
      }

      // Mark as initialized
      container.dataset.resizableInit = 'true';

      // Get storage key for persistence
      const storageKey = container.dataset.resizeKey || 'resizable-default';

      // Restore saved width
      const savedWidth = localStorage.getItem(`resizable-${storageKey}`);
      if (savedWidth) {
        leftPanel.style.flexBasis = savedWidth;
      }

      // Add mousedown listener to handle
      handle.addEventListener('mousedown', (e) => {
        e.preventDefault();
        this.startResize(e, container, leftPanel, storageKey);
      });
    },

    // Start resizing
    startResize(e, container, leftPanel, storageKey) {
      this.activeHandle = true;
      this.startX = e.clientX;
      this.startWidth = leftPanel.offsetWidth;
      this.container = container;
      this.leftPanel = leftPanel;
      this.storageKey = storageKey;

      // Add dragging class for visual feedback
      container.classList.add('resizing');

      // Add global listeners
      document.addEventListener('mousemove', this.handleMouseMove);
      document.addEventListener('mouseup', this.handleMouseUp);

      // Prevent text selection during drag
      document.body.style.userSelect = 'none';
    },

    // Handle mouse move
    handleMouseMove(e) {
      if (!Resizable.activeHandle) return;

      const delta = e.clientX - Resizable.startX;
      const newWidth = Resizable.startWidth + delta;
      const containerWidth = Resizable.container.offsetWidth;

      // Enforce min/max constraints (10% - 90%)
      const minWidth = containerWidth * 0.1;
      const maxWidth = containerWidth * 0.9;

      if (newWidth >= minWidth && newWidth <= maxWidth) {
        Resizable.leftPanel.style.flexBasis = `${newWidth}px`;
      }
    },

    // Handle mouse up
    handleMouseUp() {
      if (!Resizable.activeHandle) return;

      // Save to localStorage
      const width = Resizable.leftPanel.style.flexBasis;
      localStorage.setItem(`resizable-${Resizable.storageKey}`, width);

      // Remove dragging class
      Resizable.container.classList.remove('resizing');

      // Remove global listeners
      document.removeEventListener('mousemove', Resizable.handleMouseMove);
      document.removeEventListener('mouseup', Resizable.handleMouseUp);

      // Restore text selection
      document.body.style.userSelect = '';

      // Reset state
      Resizable.activeHandle = null;
      Resizable.container = null;
      Resizable.leftPanel = null;
      Resizable.storageKey = null;
    }
  };

  // Bind event handlers to maintain context
  Resizable.handleMouseMove = Resizable.handleMouseMove.bind(Resizable);
  Resizable.handleMouseUp = Resizable.handleMouseUp.bind(Resizable);

  // Auto-initialize on DOMContentLoaded
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => Resizable.init());
  } else {
    Resizable.init();
  }

  // Expose to window for manual control if needed
  window.Resizable = Resizable;

})();
