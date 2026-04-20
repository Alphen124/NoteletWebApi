// not used yet
package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"noteletwebservice-development/middlewares"
	"noteletwebservice-development/types/requests"
)

// DeviceController handles device-related HTTP requests
type DeviceController struct {
	DB *sql.DB
}

// NewDeviceController creates a new device controller
func NewDeviceController(db *sql.DB) *DeviceController {
	return &DeviceController{DB: db}
}

// CreateDevice handles POST /api/devices - Create a new device
func (dc *DeviceController) CreateDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Get user ID from context (set by auth middleware)
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	userID := userCtx.UserId

	var req requests.CreateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Debug logging
	fmt.Printf("CreateDevice - UserID: %d, Name: %s, Type: %s, Price: %.2f\n",
		userID, req.Name, req.Type, req.Price)

	// Validate required fields (admin อนุญาตให้ price=0 สำหรับอุปกรณ์ยืมจากคณะ)
	if req.Name == "" || req.Type == "" {
		respondWithError(w, http.StatusBadRequest, "Name and type are required", "")
		return
	}
	if !userCtx.IsAdmin && req.Price <= 0 {
		respondWithError(w, http.StatusBadRequest, "Price must be greater than 0", "")
		return
	}

	// Get DeviceTypeNo based on type name
	var deviceTypeNo int
	err := dc.DB.QueryRow(
		"SELECT DeviceTypeNo FROM DeviceType WHERE DeviceTypeName = $1",
		req.Type,
	).Scan(&deviceTypeNo)
	if err != nil {
		fmt.Printf("Error getting DeviceTypeNo: %v\n", err)
		respondWithError(w, http.StatusBadRequest, "Invalid device type", err.Error())
		return
	}
	fmt.Printf("Found DeviceTypeNo: %d for type: %s\n", deviceTypeNo, req.Type)

	// Get OwnerNo for the user (TypeNo references Owner table)
	var ownerNo int
	err = dc.DB.QueryRow(
		"SELECT OwnerNo FROM Owner WHERE UserId = $1",
		userID,
	).Scan(&ownerNo)
	if err != nil {
		fmt.Printf("Error getting OwnerNo for UserId %d: %v\n", userID, err)
		// If no owner record exists, create one
		err = dc.DB.QueryRow(
			"INSERT INTO Owner (UserId) VALUES ($1) RETURNING OwnerNo",
			userID,
		).Scan(&ownerNo)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to create owner record", err.Error())
			return
		}
		fmt.Printf("Created new Owner with OwnerNo: %d for UserId: %d\n", ownerNo, userID)
	}
	fmt.Printf("Found OwnerNo: %d for UserId: %d\n", ownerNo, userID)

	// Insert device (set is_admin_device if user is admin or authorized lender)
	var deviceNo int
	isAdminDevice := userCtx.IsAdmin || userCtx.IsAuthorizedLender
	query := `
		INSERT INTO Device (DeviceName, Description, RentPrice, DeviceTypeNo, TypeNo, UserId, ImageUrl, is_admin_device, CPU, RAM, Storage, GPU, Status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 1)
		RETURNING DeviceNo
	`
	fmt.Printf("Executing query with params: Name=%s, Desc=%s, Price=%.2f, DeviceTypeNo=%d, TypeNo=%d, UserID=%d, ImageUrl=%s, IsAdminDevice=%v\n",
		req.Name, req.Description, req.Price, deviceTypeNo, ownerNo, userID, req.ImageUrl, isAdminDevice)

	var cpuVal, ramVal, storageVal, gpuVal interface{}
	if req.CPU != "" {
		cpuVal = req.CPU
	}
	if req.RAM != "" {
		ramVal = req.RAM
	}
	if req.Storage != "" {
		storageVal = req.Storage
	}
	if req.GPU != "" {
		gpuVal = req.GPU
	}

	err = dc.DB.QueryRow(
		query,
		req.Name,
		req.Description,
		req.Price,
		deviceTypeNo,
		ownerNo,
		userID,
		req.ImageUrl,
		isAdminDevice,
		cpuVal,
		ramVal,
		storageVal,
		gpuVal,
	).Scan(&deviceNo)

	if err != nil {
		fmt.Printf("Error inserting device: %v\n", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to create device", err.Error())
		return
	}

	fmt.Printf("Successfully created device with DeviceNo: %d\n", deviceNo)

	// Insert into DeviceOwner table to link device with owner
	_, err = dc.DB.Exec(
		"INSERT INTO DeviceOwner (DeviceNo, OwnerNo) VALUES ($1, $2)",
		deviceNo,
		ownerNo,
	)
	if err != nil {
		fmt.Printf("Error inserting into DeviceOwner: %v\n", err)
		// Rollback: delete the device if DeviceOwner insertion fails
		dc.DB.Exec("DELETE FROM Device WHERE DeviceNo = $1", deviceNo)
		respondWithError(w, http.StatusInternalServerError, "Failed to link device with owner", err.Error())
		return
	}

	fmt.Printf("Successfully linked Device %d with Owner %d in DeviceOwner table\n", deviceNo, ownerNo)

	// Insert initial status into DeviceStatusHistory (StatusNo=1 = Available)
	_, err = dc.DB.Exec(
		`INSERT INTO DeviceStatusHistory (DeviceNo, StatusNo, ChangedBy, ChangedAt)
		 VALUES ($1, 1, $2, $3)`,
		deviceNo,
		userID,
		time.Now(),
	)
	if err != nil {
		fmt.Printf("Warning: Failed to insert DeviceStatusHistory for Device %d: %v\n", deviceNo, err)
		// Not a fatal error — device and owner link already created successfully
	} else {
		fmt.Printf("Successfully inserted DeviceStatusHistory for Device %d (StatusNo=1 Available)\n", deviceNo)
	}

	respondWithSuccess(w, http.StatusCreated, "Device created successfully", map[string]interface{}{
		"deviceNo": deviceNo,
	})
}

// GetMyDevices handles GET /api/devices/my - Get all devices owned by the current user
func (dc *DeviceController) GetMyDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Get user ID from context
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	userID := userCtx.UserId

	query := `
		SELECT 
			d.DeviceNo, d.DeviceName, COALESCE(d.Description, '') as Description, d.RentPrice,
			COALESCE(dt.DeviceTypeName, 'อื่นๆ') as DeviceTypeName,
			COALESCE(d.Rating, 0) as Rating,
			COALESCE(s.Name, d.Status::TEXT, 'Request Pending') as StatusName,
			COALESCE(d.ImageUrl, '') as ImageUrl,
			d.CreatedAt,
			d.CPU, d.RAM, d.Storage, d.GPU
		FROM Device d
		LEFT JOIN DeviceType dt ON d.DeviceTypeNo = dt.DeviceTypeNo
		LEFT JOIN Status s ON d.Status = s.StatusNo
		WHERE d.UserId = $1
		ORDER BY d.CreatedAt DESC
	`

	rows, err := dc.DB.Query(query, userID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch devices", err.Error())
		return
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var deviceNo int
		var deviceName, description, statusName, imageUrl string
		var rentPrice, rating float64
		var deviceTypeName string
		var createdAt sql.NullTime
		var cpu, ram, storage, gpu sql.NullString

		err := rows.Scan(
			&deviceNo, &deviceName, &description, &rentPrice,
			&deviceTypeName, &rating, &statusName, &imageUrl, &createdAt,
			&cpu, &ram, &storage, &gpu,
		)
		if err != nil {
			continue
		}

		device := map[string]interface{}{
			"deviceNo":    deviceNo,
			"name":        deviceName,
			"description": description,
			"price":       rentPrice,
			"type":        deviceTypeName,
			"rating":      rating,
			"status":      statusName,
			"imageUrl":    imageUrl,
			"cpu":         cpu.String,
			"ram":         ram.String,
			"storage":     storage.String,
			"gpu":         gpu.String,
		}
		if createdAt.Valid {
			device["createdAt"] = createdAt.Time
		}
		devices = append(devices, device)
	}

	if devices == nil {
		devices = []map[string]interface{}{}
	}

	respondWithSuccess(w, http.StatusOK, "Devices retrieved successfully", map[string]interface{}{
		"data": devices,
	})
}

// GetAllDevices handles GET /api/devices/browse - Get available devices for public browsing (no auth needed)
func (dc *DeviceController) GetAllDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	deviceType := r.URL.Query().Get("type")

	// Status 1 = Available (INTEGER column after migration 001)
	query := `
		SELECT 
			d.DeviceNo, d.DeviceName, d.Description, d.RentPrice,
			COALESCE(dt.DeviceTypeName, 'อื่นๆ') as DeviceTypeName,
			d.Rating,
			COALESCE(s.Name, 'Available') as StatusName,
			COALESCE(d.ImageUrl, '') as ImageUrl,
			d.CreatedAt,
			COALESCE(au.Email, '') as OwnerEmail,
			COALESCE(d.is_admin_device, false) as IsAdminDevice,
			d.CPU, d.RAM, d.Storage, d.GPU
		FROM Device d
		LEFT JOIN DeviceType dt ON d.DeviceTypeNo = dt.DeviceTypeNo
		LEFT JOIN AppUser au ON d.UserId = au.UserId
		LEFT JOIN Status s ON d.Status = s.StatusNo
		WHERE d.Status = 1
	`

	args := []interface{}{}
	argCount := 1

	if deviceType != "" && deviceType != "all" {
		query += " AND dt.DeviceTypeName ILIKE $" + strconv.Itoa(argCount)
		args = append(args, "%"+deviceType+"%")
		argCount++
	}

	query += " ORDER BY d.CreatedAt DESC"

	rows, err := dc.DB.Query(query, args...)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch devices", err.Error())
		return
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var deviceNo int
		var deviceName, description, imageUrl, ownerEmail, deviceTypeName, statusName string
		var rentPrice, rating float64
		var createdAt sql.NullTime
		var isAdminDevice bool
		var cpu, ram, storage, gpu sql.NullString

		err := rows.Scan(
			&deviceNo, &deviceName, &description, &rentPrice,
			&deviceTypeName, &rating, &statusName,
			&imageUrl, &createdAt, &ownerEmail, &isAdminDevice,
			&cpu, &ram, &storage, &gpu,
		)
		if err != nil {
			fmt.Printf("Error scanning browse row: %v\n", err)
			continue
		}

		device := map[string]interface{}{
			"deviceNo":      deviceNo,
			"name":          deviceName,
			"description":   description,
			"price":         rentPrice,
			"type":          deviceTypeName,
			"rating":        rating,
			"status":        statusName,
			"imageUrl":      imageUrl,
			"ownerEmail":    ownerEmail,
			"isAdminDevice": isAdminDevice,
			"cpu":           cpu.String,
			"ram":           ram.String,
			"storage":       storage.String,
			"gpu":           gpu.String,
		}
		if createdAt.Valid {
			device["createdAt"] = createdAt.Time
		}
		devices = append(devices, device)
	}

	if err = rows.Err(); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error reading devices", err.Error())
		return
	}

	if devices == nil {
		devices = []map[string]interface{}{}
	}

	respondWithSuccess(w, http.StatusOK, "Devices retrieved successfully", devices)
}

// GetDevice handles GET /api/devices/{id} - Get a specific device by ID
func (dc *DeviceController) GetDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Extract device ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/devices/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondWithError(w, http.StatusBadRequest, "Device ID is required", "")
		return
	}

	deviceID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid device ID", err.Error())
		return
	}

	query := `
		SELECT 
			d.DeviceNo, d.DeviceName, COALESCE(d.Description, '') as Description, d.RentPrice,
			COALESCE(dt.DeviceTypeName, 'อื่นๆ') as DeviceTypeName,
			COALESCE(d.Rating, 0) as Rating,
			COALESCE(s.Name, d.Status::TEXT) as StatusName,
			COALESCE(d.ImageUrl, '') as ImageUrl,
			d.CreatedAt,
			COALESCE(au.Email, '') as OwnerEmail,
			d.UserId as OwnerUserId,
			COALESCE(d.is_admin_device, false) as IsAdminDevice,
			d.CPU, d.RAM, d.Storage, d.GPU
		FROM Device d
		LEFT JOIN DeviceType dt ON d.DeviceTypeNo = dt.DeviceTypeNo
		LEFT JOIN AppUser au ON d.UserId = au.UserId
		LEFT JOIN Status s ON d.Status = s.StatusNo
		WHERE d.DeviceNo = $1
	`

	var deviceNo, ownerUserID int
	var deviceName, description, statusName, imageUrl, ownerEmail, deviceTypeName string
	var rentPrice, rating float64
	var createdAt sql.NullTime
	var isAdminDevice bool
	var cpu, ram, storage, gpu sql.NullString

	err = dc.DB.QueryRow(query, deviceID).Scan(
		&deviceNo, &deviceName, &description, &rentPrice,
		&deviceTypeName, &rating, &statusName, &imageUrl, &createdAt, &ownerEmail, &ownerUserID, &isAdminDevice,
		&cpu, &ram, &storage, &gpu,
	)

	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Device not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch device", err.Error())
		return
	}

	device := map[string]interface{}{
		"deviceNo":      deviceNo,
		"name":          deviceName,
		"description":   description,
		"price":         rentPrice,
		"type":          deviceTypeName,
		"rating":        rating,
		"status":        statusName,
		"imageUrl":      imageUrl,
		"ownerEmail":    ownerEmail,
		"ownerUserId":   ownerUserID,
		"isAdminDevice": isAdminDevice,
		"cpu":           cpu.String,
		"ram":           ram.String,
		"storage":       storage.String,
		"gpu":           gpu.String,
	}

	if createdAt.Valid {
		device["createdAt"] = createdAt.Time
	}

	respondWithSuccess(w, http.StatusOK, "Device retrieved successfully", device)
}

// UpdateDevice handles PUT /api/devices/{id} - Update device details
func (dc *DeviceController) UpdateDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Get user ID from context
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	userID := userCtx.UserId

	// Extract device ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/devices/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondWithError(w, http.StatusBadRequest, "Device ID is required", "")
		return
	}

	deviceID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid device ID", err.Error())
		return
	}

	// Check device ownership - admin can update any device
	var ownerUserID, currentStatus int
	err = dc.DB.QueryRow("SELECT UserId, Status FROM Device WHERE DeviceNo = $1", deviceID).Scan(&ownerUserID, &currentStatus)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Device not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch device", err.Error())
		return
	}

	if ownerUserID != userID && !userCtx.IsAdmin {
		respondWithError(w, http.StatusForbidden, "You are not authorized to update this device", "")
		return
	}

	// Cannot edit if rental is in progress (Booking Confirmed=2 or Rental Active=3)
	if currentStatus == 2 || currentStatus == 3 {
		statusName := "Booking Confirmed"
		if currentStatus == 3 {
			statusName = "Rental Active"
		}
		respondWithError(w, http.StatusBadRequest, fmt.Sprintf("Cannot edit device with status %s", statusName), "")
		return
	}

	// Parse request body
	var req requests.CreateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate required fields (admin/authorized lender อนุญาตให้ price=0 สำหรับอุปกรณ์ยืมจากคณะ)
	if req.Name == "" || req.Type == "" {
		respondWithError(w, http.StatusBadRequest, "Name and type are required", "")
		return
	}
	if !userCtx.IsAdmin && !userCtx.IsAuthorizedLender && req.Price <= 0 {
		respondWithError(w, http.StatusBadRequest, "Price must be greater than 0", "")
		return
	}

	// Get DeviceTypeNo based on type name
	var deviceTypeNo int
	err = dc.DB.QueryRow(
		"SELECT DeviceTypeNo FROM DeviceType WHERE DeviceTypeName = $1",
		req.Type,
	).Scan(&deviceTypeNo)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid device type", err.Error())
		return
	}

	// Update device (admin can update any device)
	var result sql.Result
	if userCtx.IsAdmin {
		updateQ := `
			UPDATE Device 
			SET DeviceName = $1, Description = $2, RentPrice = $3, DeviceTypeNo = $4, ImageUrl = $5,
			    CPU = $6, RAM = $7, Storage = $8, GPU = $9, UpdatedAt = CURRENT_TIMESTAMP
			WHERE DeviceNo = $10
		`
		var cpuVal, ramVal, storageVal, gpuVal interface{}
		if req.CPU != "" {
			cpuVal = req.CPU
		}
		if req.RAM != "" {
			ramVal = req.RAM
		}
		if req.Storage != "" {
			storageVal = req.Storage
		}
		if req.GPU != "" {
			gpuVal = req.GPU
		}
		result, err = dc.DB.Exec(updateQ, req.Name, req.Description, req.Price, deviceTypeNo, req.ImageUrl, cpuVal, ramVal, storageVal, gpuVal, deviceID)
	} else {
		updateQ := `
			UPDATE Device 
			SET DeviceName = $1, Description = $2, RentPrice = $3, DeviceTypeNo = $4, ImageUrl = $5,
			    CPU = $6, RAM = $7, Storage = $8, GPU = $9, UpdatedAt = CURRENT_TIMESTAMP
			WHERE DeviceNo = $10 AND UserId = $11
		`
		var cpuVal, ramVal, storageVal, gpuVal interface{}
		if req.CPU != "" {
			cpuVal = req.CPU
		}
		if req.RAM != "" {
			ramVal = req.RAM
		}
		if req.Storage != "" {
			storageVal = req.Storage
		}
		if req.GPU != "" {
			gpuVal = req.GPU
		}
		result, err = dc.DB.Exec(updateQ, req.Name, req.Description, req.Price, deviceTypeNo, req.ImageUrl, cpuVal, ramVal, storageVal, gpuVal, deviceID, userID)
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update device", err.Error())
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondWithError(w, http.StatusNotFound, "Device not found or unauthorized", "")
		return
	}

	respondWithSuccess(w, http.StatusOK, "Device updated successfully", map[string]interface{}{
		"deviceNo": deviceID,
	})
}

// DeleteDevice handles DELETE /api/devices/{id} - Delete a device
func (dc *DeviceController) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Get user ID from context
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	userID := userCtx.UserId

	// Extract device ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/devices/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondWithError(w, http.StatusBadRequest, "Device ID is required", "")
		return
	}

	deviceID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid device ID", err.Error())
		return
	}

	// Check device ownership - admin can delete any device
	var ownerUserID, deviceStatus int
	err = dc.DB.QueryRow("SELECT UserId, Status FROM Device WHERE DeviceNo = $1", deviceID).Scan(&ownerUserID, &deviceStatus)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Device not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch device", err.Error())
		return
	}

	if ownerUserID != userID && !userCtx.IsAdmin {
		respondWithError(w, http.StatusForbidden, "You are not authorized to delete this device", "")
		return
	}

	// Cannot delete if rental is in progress (Booking Confirmed=2 or Rental Active=3)
	if deviceStatus == 2 || deviceStatus == 3 {
		statusName := "Booking Confirmed"
		if deviceStatus == 3 {
			statusName = "Rental Active"
		}
		respondWithError(w, http.StatusBadRequest, fmt.Sprintf("Cannot delete device with status %s. Please wait until the rental is completed.", statusName), "")
		return
	}

	// Delete the device (admin can delete any, regular user only their own)
	var result sql.Result
	if userCtx.IsAdmin {
		result, err = dc.DB.Exec("DELETE FROM Device WHERE DeviceNo = $1", deviceID)
	} else {
		result, err = dc.DB.Exec("DELETE FROM Device WHERE DeviceNo = $1 AND UserId = $2", deviceID, userID)
	}

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to delete device", err.Error())
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondWithError(w, http.StatusNotFound, "Device not found or unauthorized", "")
		return
	}

	respondWithSuccess(w, http.StatusOK, "Device deleted successfully", nil)
}

// UpdateDeviceStatus updates the status of a device
func (dc *DeviceController) UpdateDeviceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Get user ID from context
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	userID := userCtx.UserId

	// Extract device ID from URL path (/api/devices/{id}/status)
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/devices/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondWithError(w, http.StatusBadRequest, "Device ID is required", "")
		return
	}

	deviceID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid device ID", err.Error())
		return
	}

	// Parse request body
	var req requests.UpdateDeviceStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate status value
	validStatuses := map[string]int{
		"Request Pending":   1,
		"Booking Confirmed": 2,
		"Rental Active":     3,
		"Rental Completed":  4,
	}
	statusNo, ok := validStatuses[req.Status]
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Invalid status value. Must be: Request Pending, Booking Confirmed, Rental Active, or Rental Completed", "")
		return
	}

	// If trying to set status to "Rental Active" (3), device must have been Booking Confirmed (2) first
	if statusNo == 3 {
		var wasConfirmed bool
		err := dc.DB.QueryRow(`
			SELECT EXISTS(
				SELECT 1 FROM devicestatushistory 
				WHERE deviceno = $1 AND statusno = 2
			)
		`, deviceID).Scan(&wasConfirmed)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to check booking history", err.Error())
			return
		}
		if !wasConfirmed {
			respondWithError(w, http.StatusBadRequest, "Cannot mark device as Rental Active if it was never Booking Confirmed", "")
			return
		}
	}

	// Check if device exists and user is owner
	var ownerUserID int
	err = dc.DB.QueryRow(`
		SELECT u.userid 
		FROM device d
		JOIN deviceowner dv ON d.deviceno = dv.deviceno
		JOIN owner o ON dv.ownerno = o.ownerno
		JOIN appuser u ON o.userid = u.userid
		WHERE d.deviceno = $1
	`, deviceID).Scan(&ownerUserID)

	if err != nil {
		if err == sql.ErrNoRows {
			respondWithError(w, http.StatusNotFound, "Device not found", "")
		} else {
			respondWithError(w, http.StatusInternalServerError, "Failed to verify device ownership", err.Error())
		}
		return
	}

	if ownerUserID != userID {
		respondWithError(w, http.StatusForbidden, "You are not authorized to update this device", "")
		return
	}

	// Get current status for history
	var currentStatus int
	err = dc.DB.QueryRow("SELECT status FROM device WHERE deviceno = $1", deviceID).Scan(&currentStatus)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to get current status", err.Error())
		return
	}

	// Update device status
	_, err = dc.DB.Exec("UPDATE device SET status = $1, updatedat = CURRENT_TIMESTAMP WHERE deviceno = $2", statusNo, deviceID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update device status", err.Error())
		return
	}

	// Insert into DeviceStatusHistory
	_, err = dc.DB.Exec(`
		INSERT INTO devicestatushistory (deviceno, statusno, changedby, changedat)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
	`, deviceID, statusNo, userID)
	if err != nil {
		// Log error but don't fail the request
		fmt.Printf("Warning: Failed to insert status history: %v\n", err)
	}

	respondWithSuccess(w, http.StatusOK, "Device status updated successfully", map[string]interface{}{
		"deviceNo": deviceID,
		"status":   req.Status,
	})
}

// GetDeviceStatusHistory retrieves the status change history for a device
func (dc *DeviceController) GetDeviceStatusHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	// Extract device ID from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/devices/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		respondWithError(w, http.StatusBadRequest, "Device ID is required", "")
		return
	}

	deviceID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid device ID", err.Error())
		return
	}

	// Query status history
	rows, err := dc.DB.Query(`
		SELECT 
			h.historyno,
			h.deviceno,
			h.statusno,
			s.name as status_name,
			h.changedby,
			u.email as changed_by_email,
			h.changedat
		FROM devicestatushistory h
		JOIN status s ON h.statusno = s.statusno
		JOIN appuser u ON h.changedby = u.userid
		WHERE h.deviceno = $1
		ORDER BY h.changedat DESC
	`, deviceID)

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch status history", err.Error())
		return
	}
	defer rows.Close()

	type HistoryEntry struct {
		HistoryNo      int       `json:"historyNo"`
		DeviceNo       int       `json:"deviceNo"`
		StatusNo       int       `json:"statusNo"`
		StatusName     string    `json:"statusName"`
		ChangedBy      int       `json:"changedBy"`
		ChangedByEmail string    `json:"changedByEmail"`
		ChangedAt      time.Time `json:"changedAt"`
	}

	var history []HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		err := rows.Scan(
			&entry.HistoryNo,
			&entry.DeviceNo,
			&entry.StatusNo,
			&entry.StatusName,
			&entry.ChangedBy,
			&entry.ChangedByEmail,
			&entry.ChangedAt,
		)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Failed to parse status history", err.Error())
			return
		}
		history = append(history, entry)
	}

	if err = rows.Err(); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error reading status history", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusOK, "Status history retrieved successfully", history)
}

// GetAllDevicesAdmin handles GET /api/devices - returns ALL devices (all statuses, all owners) for authenticated users
func (dc *DeviceController) GetAllDevicesAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	deviceType := r.URL.Query().Get("type")

	baseQuery := `
		SELECT 
			d.DeviceNo,
			d.DeviceName,
			d.Description,
			d.RentPrice,
			COALESCE(dt.DeviceTypeName, 'อื่นๆ') as DeviceTypeName,
			d.Rating,
			COALESCE(s.Name, d.Status::TEXT) as StatusName,
			COALESCE(d.ImageUrl, '') as ImageUrl,
			d.CreatedAt,
			COALESCE(au.Email, '') as OwnerEmail,
			COALESCE(NULLIF(TRIM(COALESCE(o.FName,'') || ' ' || COALESCE(o.LName,'')), ''), au.Email, '') as OwnerName,
			COALESCE(d.is_admin_device, false) as IsAdminDevice
		FROM Device d
		LEFT JOIN DeviceType dt ON d.DeviceTypeNo = dt.DeviceTypeNo
		LEFT JOIN AppUser au ON d.UserId = au.UserId
		LEFT JOIN Status s ON d.Status = s.StatusNo
		LEFT JOIN Owner o ON o.UserId = d.UserId
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 1

	if deviceType != "" && deviceType != "all" {
		baseQuery += " AND dt.DeviceTypeName ILIKE $" + strconv.Itoa(argCount)
		args = append(args, "%"+deviceType+"%")
		argCount++
	}

	baseQuery += " ORDER BY d.Status ASC, d.CreatedAt DESC"

	rows, err := dc.DB.Query(baseQuery, args...)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch devices", err.Error())
		return
	}
	defer rows.Close()

	var devices []map[string]interface{}
	for rows.Next() {
		var deviceNo int
		var deviceName, description, imageUrl, ownerEmail, ownerName, deviceTypeName, statusName string
		var rentPrice, rating float64
		var createdAt sql.NullTime
		var isAdminDevice bool

		err := rows.Scan(
			&deviceNo, &deviceName, &description, &rentPrice,
			&deviceTypeName, &rating, &statusName,
			&imageUrl, &createdAt, &ownerEmail, &ownerName, &isAdminDevice,
		)
		if err != nil {
			fmt.Printf("Error scanning device row: %v\n", err)
			continue
		}

		device := map[string]interface{}{
			"deviceNo":      deviceNo,
			"name":          deviceName,
			"description":   description,
			"price":         rentPrice,
			"type":          deviceTypeName,
			"rating":        rating,
			"status":        statusName,
			"imageUrl":      imageUrl,
			"ownerEmail":    ownerEmail,
			"ownerName":     ownerName,
			"isAdminDevice": isAdminDevice,
		}
		if createdAt.Valid {
			device["createdAt"] = createdAt.Time
		}
		devices = append(devices, device)
	}

	if err = rows.Err(); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error reading devices", err.Error())
		return
	}

	if devices == nil {
		devices = []map[string]interface{}{}
	}

	respondWithSuccess(w, http.StatusOK, "All devices retrieved successfully", devices)
}
