// Photography Blog JavaScript

// Current pagination state
const state = {
    offset: 0,
    limit: 20,
    totalCount: 0,
    allTags: []
};

// DOM elements
const photosContainer = document.getElementById('photos-container');
const showingInfo = document.getElementById('showing-info');
const prevPageBtn = document.getElementById('prev-page');
const nextPageBtn = document.getElementById('next-page');
const photoTemplate = document.getElementById('photo-card-template');
const allTagsContainer = document.getElementById('all-tags-container');

// Initialize state from URL parameters
function initFromUrl() {
    const urlParams = new URLSearchParams(window.location.search);
    const pageParam = urlParams.get('page');
    
    if (pageParam && !isNaN(parseInt(pageParam))) {
        const page = parseInt(pageParam);
        if (page > 0) {
            // Convert from 1-based page number to 0-based offset
            state.offset = (page - 1) * state.limit;
        }
    }
}

// Update URL with current page information
function updateUrl() {
    // Calculate current page (1-based) from offset
    const currentPage = Math.floor(state.offset / state.limit) + 1;
    
    // Create new URL with page parameter
    const url = new URL(window.location);
    url.searchParams.set('page', currentPage);
    
    // Update browser history without reloading the page
    window.history.pushState({}, '', url);
    
    // Update the document title with page info
    document.title = `Photography Blog - Page ${currentPage}`;
}

// Load photos from the API
async function loadPhotos() {
    try {
        // Show loading state
        photosContainer.innerHTML = '<div class="col-12 text-center"><div class="spinner-border" role="status"><span class="visually-hidden">Loading...</span></div></div>';
        
        // Fetch photos with pagination
        const response = await fetch(`/photos?offset=${state.offset}&limit=${state.limit}`);
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        const data = await response.json();
        
        // Update state with new data
        state.totalCount = data.totalCount;
        state.allTags = data.allTags || [];
        
        // Display all available tags
        displayAllTags();
        
        // Clear container
        photosContainer.innerHTML = '';
        
        // Display photos
        if (data.photos && data.photos.length > 0) {
            data.photos.forEach(photo => {
                displayPhoto(photo);
            });
        } else {
            photosContainer.innerHTML = '<div class="col-12"><div class="alert alert-info">No photos found.</div></div>';
        }
        
        // Update pagination info and buttons
        updatePagination();
        
        // Update URL with current page
        updateUrl();
        
    } catch (error) {
        console.error('Error loading photos:', error);
        photosContainer.innerHTML = `<div class="col-12"><div class="alert alert-danger">Failed to load photos: ${error.message}</div></div>`;
    }
}

// Display a single photo
function displayPhoto(photo) {
    // Clone the template
    const photoCard = photoTemplate.content.cloneNode(true);
    
    // Find elements to update
    const img = photoCard.querySelector('img');
    const title = photoCard.querySelector('.card-title');
    const tagsContainer = photoCard.querySelector('.tags');
    const lastModified = photoCard.querySelector('.last-modified');
    
    // Store the photo key as a data attribute for later use
    const cardDiv = photoCard.querySelector('.card');
    cardDiv.dataset.photoKey = photo.key;
    
    // Add drop capabilities to the card
    cardDiv.addEventListener('dragover', (e) => {
        e.preventDefault(); // Allow drop
        cardDiv.classList.add('drag-over');
    });
    
    cardDiv.addEventListener('dragleave', () => {
        cardDiv.classList.remove('drag-over');
    });
    
    cardDiv.addEventListener('drop', async (e) => {
        e.preventDefault();
        cardDiv.classList.remove('drag-over');
        
        // Get the dropped tag
        const droppedTag = e.dataTransfer.getData('text/plain');
        if (!droppedTag) return;
        
        // Get current tags
        const currentTags = photo.metadata && photo.metadata.tag ? 
            photo.metadata.tag.split(',').map(t => t.trim()).filter(t => t) : 
            [];
        
        // Check if tag already exists
        if (currentTags.includes(droppedTag)) {
            showFeedback(tagsContainer, `Tag "${droppedTag}" already exists`, 'warning');
            return;
        }
        
        // Add the new tag
        currentTags.push(droppedTag);
        const newTagsString = currentTags.join(',');
        
        // Show saving indicator
        const savingIndicator = document.createElement('div');
        savingIndicator.className = 'saving-indicator';
        savingIndicator.innerHTML = '<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span> Adding tag...';
        cardDiv.appendChild(savingIndicator);
        
        try {
            // Update metadata via API
            const success = await updatePhotoMetadata(photo.key, newTagsString);
            
            if (success) {
                // Update local state
                photo.metadata = photo.metadata || {};
                photo.metadata.tag = newTagsString;
                
                // Update the display
                const tagsDisplay = tagsContainer.querySelector('.tags-display');
                updateTagsDisplay(tagsDisplay, newTagsString);
                
                // Show success message
                showFeedback(tagsContainer, `Added tag "${droppedTag}"`, 'success');
                
                // Make sure the tag is in the available tags list
                if (!state.allTags.includes(droppedTag)) {
                    state.allTags.push(droppedTag);
                    // Refresh the tags panel
                    displayAllTags();
                }
            } else {
                showFeedback(tagsContainer, 'Failed to add tag', 'danger');
            }
        } catch (error) {
            console.error('Error adding tag:', error);
            showFeedback(tagsContainer, 'Error adding tag', 'danger');
        } finally {
            // Remove the saving indicator
            cardDiv.removeChild(savingIndicator);
        }
    });
    
    // Set photo data
    img.src = photo.url;
    img.alt = photo.name;
    title.textContent = photo.name;
    
    // Create tags container with edit functionality
    tagsContainer.innerHTML = `
        <div class="d-flex justify-content-between align-items-center mb-2">
            <label>Tags:</label>
            <button class="btn btn-sm btn-outline-secondary edit-tags-btn">Edit</button>
        </div>
        <div class="tags-display"></div>
        <div class="tags-edit d-none">
            <input type="text" class="form-control form-control-sm mb-2 tag-input" placeholder="Add tags (comma separated)">
            <div class="d-flex">
                <button class="btn btn-sm btn-primary save-tags-btn me-2">Save</button>
                <button class="btn btn-sm btn-outline-secondary cancel-tags-btn">Cancel</button>
            </div>
        </div>
        <div class="mt-2 small text-muted">Drop tags here from the sidebar</div>
    `;
    
    const tagsDisplay = tagsContainer.querySelector('.tags-display');
    const tagsInput = tagsContainer.querySelector('.tag-input');
    const editBtn = tagsContainer.querySelector('.edit-tags-btn');
    const saveBtn = tagsContainer.querySelector('.save-tags-btn');
    const cancelBtn = tagsContainer.querySelector('.cancel-tags-btn');
    
    // Display current tags
    let currentTags = photo.metadata && photo.metadata.tag ? photo.metadata.tag : '';
    updateTagsDisplay(tagsDisplay, currentTags);
    
    // Set up tag input with current tags
    tagsInput.value = currentTags;
    
    // Edit button handler
    editBtn.addEventListener('click', () => {
        // Make sure tagsInput has the latest tags (including any that were dragged)
        tagsInput.value = photo.metadata && photo.metadata.tag ? photo.metadata.tag : '';
        
        tagsContainer.querySelector('.tags-display').classList.add('d-none');
        tagsContainer.querySelector('.tags-edit').classList.remove('d-none');
        editBtn.classList.add('d-none');
        tagsInput.focus();
    });
    
    // Cancel button handler
    cancelBtn.addEventListener('click', () => {
        tagsContainer.querySelector('.tags-display').classList.remove('d-none');
        tagsContainer.querySelector('.tags-edit').classList.add('d-none');
        editBtn.classList.remove('d-none');
    });
    
    // Save button handler
    saveBtn.addEventListener('click', async () => {
        const newTags = tagsInput.value.trim();
        
        // Show saving state
        saveBtn.disabled = true;
        saveBtn.innerHTML = '<span class="spinner-border spinner-border-sm" role="status" aria-hidden="true"></span> Saving...';
        
        try {
            // Update metadata via API
            const success = await updatePhotoMetadata(photo.key, newTags);
            if (success) {
                // Update local state and UI on success
                photo.metadata = photo.metadata || {};
                photo.metadata.tag = newTags;
                updateTagsDisplay(tagsDisplay, newTags);
                
                // Update available tags list with any new tags
                const newTagList = newTags.split(',').map(t => t.trim()).filter(t => t);
                let tagsUpdated = false;
                
                // Add any new tags to the allTags array
                newTagList.forEach(tag => {
                    if (!state.allTags.includes(tag)) {
                        state.allTags.push(tag);
                        tagsUpdated = true;
                    }
                });
                
                // Refresh the tags panel if needed
                if (tagsUpdated) {
                    displayAllTags();
                }
                
                // Show success feedback
                showFeedback(tagsContainer, 'Tags updated successfully', 'success');
                
                // Exit edit mode
                tagsContainer.querySelector('.tags-display').classList.remove('d-none');
                tagsContainer.querySelector('.tags-edit').classList.add('d-none');
                editBtn.classList.remove('d-none');
            } else {
                showFeedback(tagsContainer, 'Failed to update tags', 'danger');
            }
        } catch (error) {
            console.error('Error updating tags:', error);
            showFeedback(tagsContainer, 'Error updating tags', 'danger');
        } finally {
            // Reset button state
            saveBtn.disabled = false;
            saveBtn.textContent = 'Save';
        }
    });
    
    // Format and set last modified date
    const date = new Date(photo.lastModified);
    lastModified.textContent = `Last modified: ${date.toLocaleDateString()}`;
    
    // Add the photo card to the container
    photosContainer.appendChild(photoCard);
}

// Update the tags display with badges
function updateTagsDisplay(tagsDisplay, tagsString) {
    tagsDisplay.innerHTML = '';
    
    if (!tagsString) {
        const noneTag = document.createElement('span');
        noneTag.className = 'badge bg-secondary';
        noneTag.textContent = 'None';
        tagsDisplay.appendChild(noneTag);
        return;
    }
    
    // Color palette for tags - same as in displayAllTags to keep consistency
    const colorClasses = [
        'bg-primary',    // Blue
        'bg-success',    // Green
        'bg-danger',     // Red
        'bg-warning',    // Yellow
        'bg-info',       // Light blue
        'bg-dark',       // Dark gray/black
        'bg-secondary',  // Gray
        'custom-purple', // Purple (custom)
        'custom-pink',   // Pink (custom)
        'custom-orange'  // Orange (custom)
    ];
    
    // Split tags by comma
    const tagList = tagsString.split(',').map(tag => tag.trim()).filter(tag => tag);
    
    // Create badges for each tag
    tagList.forEach(tag => {
        // Get a consistent color based on the tag name
        const hashCode = tag.split('').reduce((acc, char) => {
            return char.charCodeAt(0) + ((acc << 5) - acc);
        }, 0);
        const colorIndex = Math.abs(hashCode) % colorClasses.length;
        
        const badge = document.createElement('span');
        badge.className = `badge ${colorClasses[colorIndex]} me-1 mb-1`;
        badge.textContent = tag;
        tagsDisplay.appendChild(badge);
    });
}

// Show feedback message
function showFeedback(container, message, type) {
    // Create feedback element
    const feedback = document.createElement('div');
    feedback.className = `alert alert-${type} mt-2 mb-0 py-1 small`;
    feedback.textContent = message;
    
    // Add to container
    container.appendChild(feedback);
    
    // Auto-remove after 3 seconds
    setTimeout(() => {
        feedback.remove();
    }, 3000);
}

// Update photo metadata via API
async function updatePhotoMetadata(key, tagsString) {
    try {
        const response = await fetch('/photos/metadata', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                key: key,
                metadata: {
                    tag: tagsString
                }
            })
        });
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        const result = await response.json();
        return result.success;
    } catch (error) {
        console.error('Error updating metadata:', error);
        return false;
    }
}

// Update pagination information and button states
function updatePagination() {
    // Calculate current range
    const start = state.offset + 1;
    const end = Math.min(state.offset + state.limit, state.totalCount);
    
    // Update showing text
    showingInfo.textContent = `Showing ${start}-${end} of ${state.totalCount}`;
    
    // Update button states
    prevPageBtn.disabled = state.offset <= 0;
    nextPageBtn.disabled = end >= state.totalCount;
    
    // Generate page links
    generatePageLinks();
}

// Generate numeric page links
function generatePageLinks() {
    const paginationLinks = document.getElementById('pagination-links');
    paginationLinks.innerHTML = '';
    
    // Calculate current page and total pages
    const currentPage = Math.floor(state.offset / state.limit) + 1;
    const totalPages = Math.ceil(state.totalCount / state.limit);
    
    // Don't show pagination for just one page
    if (totalPages <= 1) {
        return;
    }
    
    // Determine range of pages to show
    let startPage = Math.max(1, currentPage - 2);
    let endPage = Math.min(totalPages, startPage + 4);
    
    // Adjust if we're near the end
    if (endPage - startPage < 4 && startPage > 1) {
        startPage = Math.max(1, endPage - 4);
    }
    
    // Add first page link if not starting at page 1
    if (startPage > 1) {
        addPageLink(paginationLinks, 1, currentPage);
        if (startPage > 2) {
            // Add ellipsis if there's a gap
            const ellipsis = document.createElement('span');
            ellipsis.className = 'pagination-item mx-1';
            ellipsis.textContent = '...';
            paginationLinks.appendChild(ellipsis);
        }
    }
    
    // Add numbered page links
    for (let i = startPage; i <= endPage; i++) {
        addPageLink(paginationLinks, i, currentPage);
    }
    
    // Add last page link if not ending at last page
    if (endPage < totalPages) {
        if (endPage < totalPages - 1) {
            // Add ellipsis if there's a gap
            const ellipsis = document.createElement('span');
            ellipsis.className = 'pagination-item mx-1';
            ellipsis.textContent = '...';
            paginationLinks.appendChild(ellipsis);
        }
        addPageLink(paginationLinks, totalPages, currentPage);
    }
}

// Add a single page link
function addPageLink(container, pageNumber, currentPage) {
    const link = document.createElement('a');
    link.href = `?page=${pageNumber}`;
    link.className = 'pagination-link';
    link.textContent = pageNumber;
    
    // Highlight current page
    if (pageNumber === currentPage) {
        link.className += ' pagination-link-active';
    } else {
        link.addEventListener('click', (e) => {
            e.preventDefault();
            state.offset = (pageNumber - 1) * state.limit;
            loadPhotos();
        });
    }
    
    container.appendChild(link);
}

// Handle previous page button click
prevPageBtn.addEventListener('click', () => {
    if (state.offset > 0) {
        state.offset = Math.max(0, state.offset - state.limit);
        loadPhotos();
    }
});

// Handle next page button click
nextPageBtn.addEventListener('click', () => {
    if (state.offset + state.limit < state.totalCount) {
        state.offset += state.limit;
        loadPhotos();
    }
});

// Handle browser back/forward navigation
window.addEventListener('popstate', () => {
    // Get the state from the URL
    initFromUrl();
    // Load photos with the new state
    loadPhotos();
});

// Display all available tags
function displayAllTags() {
    // Clear the container
    allTagsContainer.innerHTML = '';
    
    if (!state.allTags || state.allTags.length === 0) {
        const noneTag = document.createElement('span');
        noneTag.className = 'badge bg-secondary';
        noneTag.textContent = 'No tags available';
        allTagsContainer.appendChild(noneTag);
        return;
    }
    
    // Sort tags alphabetically
    const sortedTags = [...state.allTags].sort();
    
    // Color palette for tags
    const colorClasses = [
        'bg-primary',    // Blue
        'bg-success',    // Green
        'bg-danger',     // Red
        'bg-warning',    // Yellow
        'bg-info',       // Light blue
        'bg-dark',       // Dark gray/black
        'bg-secondary',  // Gray
        'custom-purple', // Purple (custom)
        'custom-pink',   // Pink (custom)
        'custom-orange'  // Orange (custom)
    ];
    
    // Map of tags to colors to ensure consistent colors
    const tagColorMap = {};
    
    // Assign colors to tags
    sortedTags.forEach((tag, index) => {
        // Get a consistent color for the same tag
        if (!tagColorMap[tag]) {
            // Use tag's string hash to pick a consistent color
            const hashCode = tag.split('').reduce((acc, char) => {
                return char.charCodeAt(0) + ((acc << 5) - acc);
            }, 0);
            const colorIndex = Math.abs(hashCode) % colorClasses.length;
            tagColorMap[tag] = colorClasses[colorIndex];
        }
    });
    
    // Create badges for each tag
    sortedTags.forEach(tag => {
        if (!tag) return; // Skip empty tags
        
        const badge = document.createElement('span');
        badge.className = `badge ${tagColorMap[tag]} me-1 mb-1 tag-badge`;
        badge.textContent = tag;
        badge.draggable = true;
        badge.setAttribute('data-tag', tag);
        badge.title = "Drag to add to a photo";
        
        // Add drag start event
        badge.addEventListener('dragstart', (e) => {
            e.dataTransfer.setData('text/plain', tag);
            e.dataTransfer.effectAllowed = 'copy';
            badge.classList.add('dragging');
        });
        
        badge.addEventListener('dragend', () => {
            badge.classList.remove('dragging');
        });
        
        allTagsContainer.appendChild(badge);
    });
}

// Initialize: load photos when the page loads
document.addEventListener('DOMContentLoaded', () => {
    // Initialize state from URL parameters
    initFromUrl();
    // Load photos with the initialized state
    loadPhotos();
});