package controllers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"noteletwebservice-development/middlewares"
	cloudinarysvc "noteletwebservice-development/services/cloudinary"
)

// UploadController handles file upload operations
type UploadController struct {
	UploadDir string
}

// NewUploadController creates a new upload controller
func NewUploadController(uploadDir string) *UploadController {
	return &UploadController{UploadDir: uploadDir}
}

// UploadImages handles POST /api/upload/images - Upload multiple images
func (uc *UploadController) UploadImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Get user ID from context (authentication required)
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}

	// Parse multipart form (max 10MB per file, 50MB total)
	err := r.ParseMultipartForm(50 << 20)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to parse form", err.Error())
		return
	}

	files := r.MultipartForm.File["images"]
	if len(files) == 0 {
		respondWithError(w, http.StatusBadRequest, "No images provided", "")
		return
	}

	// Limit to maximum 5 images
	if len(files) > 5 {
		respondWithError(w, http.StatusBadRequest, "Maximum 5 images allowed", "")
		return
	}

	uploadedUrls := []string{}
	allowedExtensions := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
	}

	for _, fileHeader := range files {
		// Check file size (max 10MB)
		if fileHeader.Size > 10<<20 {
			respondWithError(w, http.StatusBadRequest, fmt.Sprintf("File %s exceeds 10MB limit", fileHeader.Filename), "")
			return
		}

		// Check file extension
		ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
		if !allowedExtensions[ext] {
			respondWithError(w, http.StatusBadRequest, fmt.Sprintf("Invalid file type %s. Allowed: jpg, jpeg, png, gif, webp", ext), "")
			return
		}

		// Open uploaded file
		file, err := fileHeader.Open()
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to read uploaded file", err.Error())
			return
		}
		defer file.Close()

		// Upload to Cloudinary
		publicID := fmt.Sprintf("%d_%d_%s", userCtx.UserId, time.Now().Unix(), sanitizeFilename(fileHeader.Filename))
		imageUrl, err := cloudinarysvc.UploadImage(r.Context(), file, publicID, "notelet/devices")
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to upload image", err.Error())
			return
		}

		uploadedUrls = append(uploadedUrls, imageUrl)
		fmt.Printf("Uploaded image to Cloudinary: %s\n", imageUrl)
	}

	respondWithSuccess(w, http.StatusOK, "Images uploaded successfully", map[string]interface{}{
		"urls":  uploadedUrls,
		"count": len(uploadedUrls),
	})
}

// sanitizeFilename removes unsafe characters from filename
func sanitizeFilename(filename string) string {
	// Remove extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	// Replace unsafe characters
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, name)
	// Limit length
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}
