(function() {
  // Toggle unlisted models visibility
  window.toggleUnlisted = function() {
    const checked = document.getElementById('toggle-unlisted').checked;
    localStorage.setItem('models-show-unlisted', checked ? '1' : '0');
    const rows = document.querySelectorAll('#models-table tbody tr');
    rows.forEach(row => {
      if (row.dataset.unlisted === 'true') {
        row.style.display = checked ? '' : 'none';
      }
    });
  };

  // Toggle display mode (ID vs Name)
  window.toggleDisplay = function() {
    const checked = document.getElementById('toggle-display').checked;
    localStorage.setItem('models-display-mode', checked ? 'name' : 'id');
    const idCols = document.querySelectorAll('.col-id');
    const nameCols = document.querySelectorAll('.col-name');
    if (checked) {
      idCols.forEach(col => col.style.display = 'none');
      nameCols.forEach(col => col.style.display = '');
    } else {
      idCols.forEach(col => col.style.display = '');
      nameCols.forEach(col => col.style.display = 'none');
    }
  };

  // Refresh models table
  window.refreshModels = function() {
    htmz.get('/ui/partials/models', '#models-list');
  };

  // Restore saved preferences
  const showUnlisted = localStorage.getItem('models-show-unlisted') === '1';
  const displayMode = localStorage.getItem('models-display-mode') === 'name';

  const unlistedCheckbox = document.getElementById('toggle-unlisted');
  const displayCheckbox = document.getElementById('toggle-display');

  if (unlistedCheckbox) {
    unlistedCheckbox.checked = showUnlisted;
    window.toggleUnlisted();
  }

  if (displayCheckbox) {
    displayCheckbox.checked = displayMode;
    window.toggleDisplay();
  }
})();
