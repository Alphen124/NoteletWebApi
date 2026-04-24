package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"noteletwebservice-development/middlewares"
)

// AdminController handles admin-only operations
type AdminController struct {
	DB *sql.DB
}

// NewAdminController creates a new admin controller
func NewAdminController(db *sql.DB) *AdminController {
	return &AdminController{DB: db}
}

// userListItem represents a user in the list view
type userListItem struct {
	UserId         int    `json:"user_id"`
	Email          string `json:"email"`
	FName          string `json:"fname"`
	LName          string `json:"lname"`
	Tel            string `json:"tel"`
	IsActive       bool   `json:"is_active"`
	IsAdmin        bool   `json:"is_admin"`
	IsCentralStaff bool   `json:"is_central_staff"`
	CreatedAt      string `json:"created_at"`
}

// deviceAdminItem represents a device for admin view
type deviceAdminItem struct {
	DeviceNo   int     `json:"device_no"`
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Price      float64 `json:"price"`
	Status     string  `json:"status"`
	OwnerName  string  `json:"owner_name"`
	OwnerEmail string  `json:"owner_email"`
	IsAdminDev bool    `json:"is_admin_device"`
	ImageURL   string  `json:"image_url"`
	CreatedAt  string  `json:"created_at"`
}

// GetAllUsers GET /api/admin/users — list every user with roles
func (ac *AdminController) GetAllUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}
	rows, err := ac.DB.Query(`
		SELECT a.userid, a.email,
		       COALESCE(o.fname,'') AS fname,
		       COALESCE(o.lname,'') AS lname,
		       COALESCE(o.tel,'')   AS tel,
		       a.isactive,
		       COALESCE(a.is_admin, false),
		       COALESCE(a.is_central_staff, false),
		       TO_CHAR(a.createdat, 'YYYY-MM-DD HH24:MI') AS createdat
		FROM appuser a
		LEFT JOIN owner o ON o.userid = a.userid
		ORDER BY a.userid DESC
	`)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	defer rows.Close()

	var users []userListItem
	for rows.Next() {
		var u userListItem
		if err := rows.Scan(&u.UserId, &u.Email, &u.FName, &u.LName, &u.Tel,
			&u.IsActive, &u.IsAdmin, &u.IsCentralStaff, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}
	if users == nil {
		users = []userListItem{}
	}
	respondWithSuccess(w, http.StatusOK, "Users retrieved", users)
}

// SetStaffRole PATCH /api/admin/users/{userId}/set-staff — promote or demote staff
// Body: { "is_staff": true|false }
func (ac *AdminController) SetStaffRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	callerCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok || !callerCtx.IsAdmin {
		respondWithError(w, http.StatusForbidden, "Admin access required", "")
		return
	}

	// Extract userId from path: /api/admin/users/{userId}/set-staff
	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// parts: ["api","admin","users","{userId}","set-staff"]
	if len(parts) < 4 {
		respondWithError(w, http.StatusBadRequest, "Invalid path", "")
		return
	}
	userId, err := strconv.Atoi(parts[3])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID", "")
		return
	}

	var body struct {
		IsStaff bool `json:"is_staff"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Prevent admin from removing their own staff role accidentally
	_, err = ac.DB.Exec(`UPDATE appuser SET is_central_staff = $1 WHERE userid = $2`, body.IsStaff, userId)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	msg := "User promoted to staff"
	if !body.IsStaff {
		msg = "User demoted from staff"
	}
	respondWithSuccess(w, http.StatusOK, msg, nil)
}

// SetActiveStatus PATCH /api/admin/users/{userId}/set-active — activate or deactivate account
// Body: { "is_active": true|false }
func (ac *AdminController) SetActiveStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	callerCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok || !callerCtx.IsAdmin {
		respondWithError(w, http.StatusForbidden, "Admin access required", "")
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		respondWithError(w, http.StatusBadRequest, "Invalid path", "")
		return
	}
	userId, err := strconv.Atoi(parts[3])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID", "")
		return
	}

	// Prevent admin from deactivating themselves
	if userId == callerCtx.UserId {
		respondWithError(w, http.StatusBadRequest, "Cannot change your own active status", "")
		return
	}

	var body struct {
		IsActive bool `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	_, err = ac.DB.Exec(`UPDATE appuser SET isactive = $1 WHERE userid = $2`, body.IsActive, userId)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}

	msg := "Account activated"
	if !body.IsActive {
		msg = "Account deactivated"
	}
	respondWithSuccess(w, http.StatusOK, msg, nil)
}

// GetAllDevicesAdmin GET /api/admin/devices — list all devices with owner info
func (ac *AdminController) GetAllDevicesAdmin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	rows, err := ac.DB.Query(`
		SELECT d.DeviceNo,
		       d.DeviceName,
		       COALESCE(dt.DeviceTypeName,'') AS typename,
		       COALESCE(d.RentPrice, 0)       AS price,
		       COALESCE(s.Name, d.Status::TEXT, 'Unknown') AS status,
		       COALESCE(NULLIF(TRIM(COALESCE(o.FName,'') || ' ' || COALESCE(o.LName,'')), ''), a.Email, '') AS ownername,
		       COALESCE(a.Email,'')            AS owneremail,
		       COALESCE(d.is_admin_device, false),
		       COALESCE(d.ImageUrl,'')         AS imageurl,
		       TO_CHAR(d.CreatedAt,'YYYY-MM-DD HH24:MI') AS createdat
		FROM Device d
		LEFT JOIN DeviceType dt ON dt.DeviceTypeNo = d.DeviceTypeNo
		LEFT JOIN AppUser a     ON a.UserId = d.UserId
		LEFT JOIN Owner o       ON o.UserId = d.UserId
		LEFT JOIN Status s      ON s.StatusNo = d.Status
		ORDER BY d.DeviceNo DESC
	`)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	defer rows.Close()

	var devices []deviceAdminItem
	for rows.Next() {
		var d deviceAdminItem
		if err := rows.Scan(&d.DeviceNo, &d.Name, &d.Type, &d.Price, &d.Status,
			&d.OwnerName, &d.OwnerEmail, &d.IsAdminDev, &d.ImageURL, &d.CreatedAt); err != nil {
			continue
		}
		devices = append(devices, d)
	}
	if devices == nil {
		devices = []deviceAdminItem{}
	}
	respondWithSuccess(w, http.StatusOK, "Devices retrieved", devices)
}

// AdminDeleteDevice DELETE /api/admin/devices/{deviceId} — delete any device
func (ac *AdminController) AdminDeleteDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// parts: ["api","admin","devices","{deviceId}"]
	if len(parts) < 4 {
		respondWithError(w, http.StatusBadRequest, "Invalid path", "")
		return
	}
	deviceId, err := strconv.Atoi(parts[3])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid device ID", "")
		return
	}

	result, err := ac.DB.Exec(`DELETE FROM device WHERE deviceno = $1`, deviceId)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		respondWithError(w, http.StatusNotFound, "Device not found", "")
		return
	}
	respondWithSuccess(w, http.StatusOK, "Device deleted", nil)
}

// AdminUpdateDevice PUT /api/admin/devices/{deviceId} — update any device fields
func (ac *AdminController) AdminUpdateDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		respondWithError(w, http.StatusBadRequest, "Invalid path", "")
		return
	}
	deviceId, err := strconv.Atoi(parts[3])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid device ID", "")
		return
	}

	var body struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       float64 `json:"price"`
		Status      string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	result, err := ac.DB.Exec(`
		UPDATE device
		SET devicename = COALESCE(NULLIF($1,''), devicename),
		    description = COALESCE(NULLIF($2,''), description),
		    rentalprice  = CASE WHEN $3 > 0 THEN $3 ELSE rentalprice END,
		    currentstatus = COALESCE(NULLIF($4,''), currentstatus)
		WHERE deviceno = $5
	`, body.Name, body.Description, body.Price, body.Status, deviceId)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		respondWithError(w, http.StatusNotFound, "Device not found", "")
		return
	}
	respondWithSuccess(w, http.StatusOK, "Device updated", nil)
}

// GetAdminStats GET /api/admin/stats — dashboard summary numbers
func (ac *AdminController) GetAdminStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	var totalUsers, totalDevices, totalStaff, totalAdmins, activeDevices, totalRentals int
	ac.DB.QueryRow(`SELECT COUNT(*) FROM appuser`).Scan(&totalUsers)
	ac.DB.QueryRow(`SELECT COUNT(*) FROM device`).Scan(&totalDevices)
	ac.DB.QueryRow(`SELECT COUNT(*) FROM appuser WHERE COALESCE(is_central_staff,false)=true`).Scan(&totalStaff)
	ac.DB.QueryRow(`SELECT COUNT(*) FROM appuser WHERE COALESCE(is_admin,false)=true`).Scan(&totalAdmins)
	ac.DB.QueryRow(`SELECT COUNT(*) FROM device WHERE currentstatus NOT IN ('deleted','unavailable')`).Scan(&activeDevices)
	ac.DB.QueryRow(`SELECT COUNT(*) FROM rentalrequest`).Scan(&totalRentals)

	respondWithSuccess(w, http.StatusOK, "Stats retrieved", map[string]int{
		"total_users":    totalUsers,
		"total_devices":  totalDevices,
		"total_staff":    totalStaff,
		"total_admins":   totalAdmins,
		"active_devices": activeDevices,
		"total_rentals":  totalRentals,
	})
}

// SetAdminRole PATCH /api/admin/users/{userId}/set-admin — promote or demote admin
// Body: { "is_admin": true|false }
func (ac *AdminController) SetAdminRole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	callerCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok || !callerCtx.IsAdmin {
		respondWithError(w, http.StatusForbidden, "Admin access required", "")
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		respondWithError(w, http.StatusBadRequest, "Invalid path", "")
		return
	}
	userId, err := strconv.Atoi(parts[3])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID", "")
		return
	}
	if userId == callerCtx.UserId {
		respondWithError(w, http.StatusBadRequest, "Cannot change your own admin role", "")
		return
	}

	var body struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	_, err = ac.DB.Exec(`UPDATE appuser SET is_admin = $1 WHERE userid = $2`, body.IsAdmin, userId)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	msg := "User promoted to admin"
	if !body.IsAdmin {
		msg = "User demoted from admin"
	}
	respondWithSuccess(w, http.StatusOK, msg, nil)
}

// UpdateUserProfile PUT /api/admin/users/{userId} — edit a user's profile info
// Body: { "fname": "", "lname": "", "tel": "" }
func (ac *AdminController) UpdateUserProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		respondWithError(w, http.StatusBadRequest, "Invalid path", "")
		return
	}
	userId, err := strconv.Atoi(parts[3])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID", "")
		return
	}

	var body struct {
		FName string `json:"fname"`
		LName string `json:"lname"`
		Tel   string `json:"tel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Update owner table (profile info)
	res, err := ac.DB.Exec(`
		UPDATE owner SET
			fname = COALESCE(NULLIF($1,''), fname),
			lname = COALESCE(NULLIF($2,''), lname),
			tel   = COALESCE(NULLIF($3,''), tel)
		WHERE userid = $4
	`, body.FName, body.LName, body.Tel, userId)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		respondWithError(w, http.StatusNotFound, "User not found or no owner profile", "")
		return
	}
	respondWithSuccess(w, http.StatusOK, "User profile updated", nil)
}

// rentalAdminItem represents a rental request for admin view
type rentalAdminItem struct {
	RequestNo   int     `json:"request_no"`
	DeviceNo    int     `json:"device_no"`
	DeviceName  string  `json:"device_name"`
	RenterEmail string  `json:"renter_email"`
	RenterName  string  `json:"renter_name"`
	OwnerEmail  string  `json:"owner_email"`
	StartDate   string  `json:"start_date"`
	EndDate     string  `json:"end_date"`
	TotalPrice  float64 `json:"total_price"`
	Status      string  `json:"status"`
	CreatedAt   string  `json:"created_at"`
}

// GetAllRentals GET /api/admin/rentals — list every rental request in the system
func (ac *AdminController) GetAllRentals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	rows, err := ac.DB.Query(`
		SELECT rr.requestno,
		       rr.deviceno,
		       COALESCE(d.devicename,'—')                       AS device_name,
		       COALESCE(au_renter.email,'—')                    AS renter_email,
		       COALESCE(rt.fname||' '||rt.lname,'—')            AS renter_name,
		       COALESCE(au_owner.email,'—')                     AS owner_email,
		       COALESCE(rr.startdate::TEXT,'—')                 AS start_date,
		       COALESCE(rr.enddate::TEXT,'—')                   AS end_date,
		       COALESCE(rr.totalprice, 0)                       AS total_price,
		       COALESCE(rr.status,'—')                          AS status,
		       TO_CHAR(rr.createdat,'YYYY-MM-DD HH24:MI')       AS created_at
		FROM rentalrequest rr
		LEFT JOIN device       d          ON d.deviceno      = rr.deviceno
		LEFT JOIN appuser      au_renter  ON au_renter.userid = rr.renteruserid
		LEFT JOIN renter       rt         ON rt.userid        = rr.renteruserid
		LEFT JOIN owner        ow         ON ow.ownerno       = d.ownerno
		LEFT JOIN appuser      au_owner   ON au_owner.userid  = ow.userid
		ORDER BY rr.requestno DESC
		LIMIT 500
	`)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	defer rows.Close()

	var rentals []rentalAdminItem
	for rows.Next() {
		var item rentalAdminItem
		if err := rows.Scan(
			&item.RequestNo, &item.DeviceNo, &item.DeviceName,
			&item.RenterEmail, &item.RenterName, &item.OwnerEmail,
			&item.StartDate, &item.EndDate, &item.TotalPrice,
			&item.Status, &item.CreatedAt,
		); err != nil {
			continue
		}
		rentals = append(rentals, item)
	}
	if rentals == nil {
		rentals = []rentalAdminItem{}
	}
	respondWithSuccess(w, http.StatusOK, "Rentals retrieved", rentals)
}

// UpdateRentalStatus PATCH /api/admin/rentals/{requestNo}/status — override rental status
// Body: { "status": "Request Pending" | "Confirmed" | "Active" | "Returned" | "Rejected" | "Cancelled" }
func (ac *AdminController) UpdateRentalStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	path := r.URL.Path
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// expected: ["api","admin","rentals","{requestNo}","status"]
	if len(parts) < 4 {
		respondWithError(w, http.StatusBadRequest, "Invalid path", "")
		return
	}
	requestNo, err := strconv.Atoi(parts[3])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request number", "")
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if body.Status == "" {
		respondWithError(w, http.StatusBadRequest, "Status is required", "")
		return
	}

	res, err := ac.DB.Exec(`UPDATE rentalrequest SET status = $1 WHERE requestno = $2`, body.Status, requestNo)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Database error", err.Error())
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		respondWithError(w, http.StatusNotFound, "Rental request not found", "")
		return
	}
	respondWithSuccess(w, http.StatusOK, "Rental status updated", nil)
}
