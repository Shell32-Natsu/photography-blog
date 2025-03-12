package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"metadata-editor/models"
)

// GetPhotos handles the /photos endpoint
func GetPhotos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query()

	// Parse offset parameter (default to 0)
	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err != nil || parsedOffset < 0 {
			http.Error(w, "Invalid offset parameter", http.StatusBadRequest)
			return
		}
		offset = parsedOffset
	}

	// Parse limit parameter (default to 20)
	limit := 20
	if limitStr := query.Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 || parsedLimit > 100 {
			http.Error(w, "Invalid limit parameter (must be between 1 and 100)", http.StatusBadRequest)
			return
		}
		limit = parsedLimit
	}

	// Get photos with pagination
	photoList, err := models.GetPhotos(r.Context(), offset, limit)
	if err != nil {
		http.Error(w, "Failed to retrieve photos: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the results as JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(photoList)
}

// UpdatePhotoMetadata handles the /photos/metadata endpoint for updating photo metadata
func UpdatePhotoMetadata(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse the request JSON
	var updateReq models.UpdatePhotoRequest
	if err := json.Unmarshal(body, &updateReq); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// Validate the request
	if updateReq.Key == "" {
		http.Error(w, "Missing required field: key", http.StatusBadRequest)
		return
	}

	// Update the photo metadata
	response, err := models.UpdatePhotoMetadata(r.Context(), updateReq)
	if err != nil {
		statusCode := http.StatusInternalServerError
		// If the photo doesn't exist, return 404 Not Found
		if response.Message != "" && response.Success == false {
			statusCode = http.StatusNotFound
		}
		http.Error(w, response.Message, statusCode)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
