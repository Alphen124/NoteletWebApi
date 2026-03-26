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
)

// RentalController handles rental request operations.
type RentalController struct {
	DB *sql.DB
}

func NewRentalController(db *sql.DB) *RentalController {
	return &RentalController{DB: db}
}

// ─────────────────────────────────────────────────────────────────────────────
// CreateRentalRequest  POST /api/rental-requests
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) CreateRentalRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	renterUserID := userCtx.UserId

	// Admin accounts cannot borrow/rent devices
	if userCtx.IsAdmin {
		respondWithError(w, http.StatusForbidden, "Admin accounts cannot rent or borrow devices", "")
		return
	}

	var req struct {
		DeviceNo   int     `json:"deviceNo"`
		StartDate  string  `json:"startDate"`
		EndDate    string  `json:"endDate"`
		TotalPrice float64 `json:"totalPrice"`
		Note       string  `json:"note"`
		PickupTime string  `json:"pickupTime"`
		ReturnTime string  `json:"returnTime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if req.DeviceNo == 0 || req.StartDate == "" || req.EndDate == "" {
		respondWithError(w, http.StatusBadRequest, "deviceNo, startDate and endDate are required", "")
		return
	}
	if req.StartDate > req.EndDate {
		respondWithError(w, http.StatusBadRequest, "startDate must be before or equal to endDate", "")
		return
	}

	// Verify device is Available (Status=1) and renter is not the owner.
	var deviceStatus int
	var ownerUserID int
	err := rc.DB.QueryRow(
		"SELECT Status, UserId FROM Device WHERE DeviceNo = $1", req.DeviceNo,
	).Scan(&deviceStatus, &ownerUserID)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Device not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to verify device", err.Error())
		return
	}
	if deviceStatus != 1 {
		respondWithError(w, http.StatusBadRequest, "Device is not available for rent", "")
		return
	}
	if ownerUserID == renterUserID {
		respondWithError(w, http.StatusBadRequest, "You cannot rent your own device", "")
		return
	}

	// Block duplicate pending request from the same renter for the same device.
	var pendingCount int
	rc.DB.QueryRow(
		"SELECT COUNT(*) FROM RentalRequest WHERE DeviceNo=$1 AND RenterUserId=$2 AND Status='Request Pending'",
		req.DeviceNo, renterUserID,
	).Scan(&pendingCount)
	if pendingCount > 0 {
		respondWithError(w, http.StatusBadRequest, "You already have a pending request for this device", "")
		return
	}

	// Lazy-create Renter profile so joins work correctly later.
	rc.DB.Exec("INSERT INTO Renter (UserId) VALUES ($1) ON CONFLICT (UserId) DO NOTHING", renterUserID)

	var requestNo int
	err = rc.DB.QueryRow(`
		INSERT INTO RentalRequest
			(DeviceNo, RenterUserId, StartDate, EndDate, TotalPrice, Note, PickupTime, ReturnTime, Status, CreatedAt, UpdatedAt)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6,''), NULLIF($7,''), NULLIF($8,''), 'Request Pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING RequestNo
	`, req.DeviceNo, renterUserID, req.StartDate, req.EndDate, req.TotalPrice, req.Note, req.PickupTime, req.ReturnTime).Scan(&requestNo)
	if err != nil {
		fmt.Printf("Error creating rental request: %v\n", err)
		respondWithError(w, http.StatusInternalServerError, "Failed to create rental request", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusCreated, "Rental request submitted successfully", map[string]interface{}{
		"requestNo": requestNo,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// GetRentalRequest  GET /api/rental-requests/{id}
// Returns full detail for a single request (owner or renter may view).
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) GetRentalRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}

	requestID, err := extractRentalRequestID(r.URL.Path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request ID", err.Error())
		return
	}

	type RequestDetail struct {
		RequestNo  int     `json:"requestNo"`
		DeviceNo   int     `json:"deviceNo"`
		StartDate  string  `json:"startDate"`
		EndDate    string  `json:"endDate"`
		PickupTime string  `json:"pickupTime"`
		ReturnTime string  `json:"returnTime"`
		TotalPrice float64 `json:"totalPrice"`
		Note       string  `json:"note"`
		Status     string  `json:"status"`
	}

	var d RequestDetail
	var note, pickup, returnt sql.NullString
	err = rc.DB.QueryRow(`
		SELECT rr.RequestNo, rr.DeviceNo,
		       rr.StartDate::TEXT, rr.EndDate::TEXT,
		       rr.PickupTime, rr.ReturnTime,
		       rr.TotalPrice, rr.Note, rr.Status
		FROM RentalRequest rr
		JOIN Device dv ON dv.DeviceNo = rr.DeviceNo
		WHERE rr.RequestNo = $1
		  AND (rr.RenterUserId = $2 OR dv.UserId = $2)
	`, requestID, userCtx.UserId).Scan(
		&d.RequestNo, &d.DeviceNo,
		&d.StartDate, &d.EndDate,
		&pickup, &returnt,
		&d.TotalPrice, &note, &d.Status,
	)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Request not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch request", err.Error())
		return
	}
	if note.Valid {
		d.Note = note.String
	}
	if pickup.Valid {
		d.PickupTime = pickup.String
	}
	if returnt.Valid {
		d.ReturnTime = returnt.String
	}

	respondWithSuccess(w, http.StatusOK, "Request retrieved", d)
}

// ─────────────────────────────────────────────────────────────────────────────
// UpdateRequestDates  PATCH /api/rental-requests/{id}/update-dates
// Renter updates dates while request is still pending.
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) UpdateRequestDates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}

	requestID, err := extractRentalRequestID(r.URL.Path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request ID", err.Error())
		return
	}

	var body struct {
		StartDate  string  `json:"startDate"`
		EndDate    string  `json:"endDate"`
		PickupTime string  `json:"pickupTime"`
		ReturnTime string  `json:"returnTime"`
		TotalPrice float64 `json:"totalPrice"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if body.StartDate == "" || body.EndDate == "" {
		respondWithError(w, http.StatusBadRequest, "startDate and endDate are required", "")
		return
	}
	if body.StartDate > body.EndDate {
		respondWithError(w, http.StatusBadRequest, "startDate must be before or equal to endDate", "")
		return
	}

	// Only renter of this request may update it, and only while pending.
	var renterUserId int
	var reqStatus string
	err = rc.DB.QueryRow(
		"SELECT RenterUserId, Status FROM RentalRequest WHERE RequestNo = $1",
		requestID,
	).Scan(&renterUserId, &reqStatus)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Request not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch request", err.Error())
		return
	}
	if renterUserId != userCtx.UserId {
		respondWithError(w, http.StatusForbidden, "Only the renter may update the dates", "")
		return
	}
	if reqStatus != "Request Pending" {
		respondWithError(w, http.StatusBadRequest, "Dates can only be updated while request is pending", "")
		return
	}

	_, err = rc.DB.Exec(`
		UPDATE RentalRequest
		SET StartDate  = $1,
		    EndDate    = $2,
		    PickupTime = NULLIF($3,''),
		    ReturnTime = NULLIF($4,''),
		    TotalPrice = $5,
		    UpdatedAt  = CURRENT_TIMESTAMP
		WHERE RequestNo = $6
	`, body.StartDate, body.EndDate, body.PickupTime, body.ReturnTime, body.TotalPrice, requestID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update dates", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusOK, "Dates updated", map[string]interface{}{
		"requestNo":  requestID,
		"startDate":  body.StartDate,
		"endDate":    body.EndDate,
		"pickupTime": body.PickupTime,
		"returnTime": body.ReturnTime,
		"totalPrice": body.TotalPrice,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// GetIncomingRequests  GET /api/rental-requests/incoming
// Returns requests for devices owned by the logged-in user.
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) GetIncomingRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	ownerUserID := userCtx.UserId

	// BUG FIX: JOIN Renter (not Owner) to get the renter's name.
	rows, err := rc.DB.Query(`
		SELECT
			rr.RequestNo,
			rr.DeviceNo,
			rr.RenterUserId,
			COALESCE(d.DeviceName, '')   AS DeviceName,
			COALESCE(d.ImageUrl, '')     AS ImageUrl,
			COALESCE(au.Email, '')       AS RenterEmail,
			COALESCE(
				NULLIF(TRIM(COALESCE(rt.FName,'') || ' ' || COALESCE(rt.LName,'')), ''),
				au.Email,
				''
			)                            AS RenterName,
			rr.StartDate::TEXT,
			rr.EndDate::TEXT,
			rr.TotalPrice,
			rr.Note,
			rr.Status,
			rr.CreatedAt
		FROM  RentalRequest rr
		JOIN  Device  d  ON d.DeviceNo   = rr.DeviceNo
		JOIN  AppUser au ON au.UserId    = rr.RenterUserId
		LEFT JOIN Renter rt ON rt.UserId = rr.RenterUserId
		WHERE d.UserId = $1
		ORDER BY
			CASE rr.Status WHEN 'Request Pending' THEN 0 WHEN 'Booking Confirmed' THEN 1 WHEN 'Rental Active' THEN 2 ELSE 3 END,
			rr.CreatedAt DESC
	`, ownerUserID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch incoming requests", err.Error())
		return
	}
	defer rows.Close()

	type RequestEntry struct {
		RequestNo    int       `json:"requestNo"`
		DeviceNo     int       `json:"deviceNo"`
		RenterUserId int       `json:"renterUserId"`
		DeviceName   string    `json:"deviceName"`
		ImageUrl     string    `json:"imageUrl"`
		RenterEmail  string    `json:"renterEmail"`
		RenterName   string    `json:"renterName"`
		StartDate    string    `json:"startDate"`
		EndDate      string    `json:"endDate"`
		TotalPrice   float64   `json:"totalPrice"`
		Note         string    `json:"note"`
		Status       string    `json:"status"`
		CreatedAt    time.Time `json:"createdAt"`
	}

	var requests []RequestEntry
	for rows.Next() {
		var e RequestEntry
		var note sql.NullString
		if err := rows.Scan(
			&e.RequestNo, &e.DeviceNo, &e.RenterUserId,
			&e.DeviceName, &e.ImageUrl,
			&e.RenterEmail, &e.RenterName,
			&e.StartDate, &e.EndDate,
			&e.TotalPrice, &note, &e.Status, &e.CreatedAt,
		); err != nil {
			fmt.Printf("Error scanning incoming request: %v\n", err)
			continue
		}
		if note.Valid {
			e.Note = note.String
		}
		requests = append(requests, e)
	}
	if requests == nil {
		requests = []RequestEntry{}
	}
	respondWithSuccess(w, http.StatusOK, "Incoming requests retrieved", requests)
}

// ─────────────────────────────────────────────────────────────────────────────
// GetOutgoingRequests  GET /api/rental-requests/outgoing
// Returns requests submitted by the logged-in user.
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) GetOutgoingRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	renterUserID := userCtx.UserId

	rows, err := rc.DB.Query(`
		SELECT
			rr.RequestNo,
			rr.DeviceNo,
			COALESCE(d.DeviceName, '')  AS DeviceName,
			COALESCE(d.ImageUrl, '')    AS ImageUrl,
			COALESCE(au.Email, '')      AS OwnerEmail,
			COALESCE(
				NULLIF(TRIM(COALESCE(o.FName,'') || ' ' || COALESCE(o.LName,'')), ''),
				au.Email,
				''
			)                           AS OwnerName,
			rr.StartDate::TEXT,
			rr.EndDate::TEXT,
			rr.TotalPrice,
			rr.Note,
			rr.Status,
			rr.CreatedAt
		FROM  RentalRequest rr
		JOIN  Device  d  ON d.DeviceNo  = rr.DeviceNo
		JOIN  AppUser au ON au.UserId   = d.UserId
		LEFT JOIN Owner o ON o.UserId   = d.UserId
		WHERE rr.RenterUserId = $1
		ORDER BY rr.CreatedAt DESC
	`, renterUserID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch outgoing requests", err.Error())
		return
	}
	defer rows.Close()

	type OutEntry struct {
		RequestNo  int       `json:"requestNo"`
		DeviceNo   int       `json:"deviceNo"`
		DeviceName string    `json:"deviceName"`
		ImageUrl   string    `json:"imageUrl"`
		OwnerEmail string    `json:"ownerEmail"`
		OwnerName  string    `json:"ownerName"`
		StartDate  string    `json:"startDate"`
		EndDate    string    `json:"endDate"`
		TotalPrice float64   `json:"totalPrice"`
		Note       string    `json:"note"`
		Status     string    `json:"status"`
		CreatedAt  time.Time `json:"createdAt"`
	}

	var requests []OutEntry
	for rows.Next() {
		var e OutEntry
		var note sql.NullString
		if err := rows.Scan(
			&e.RequestNo, &e.DeviceNo,
			&e.DeviceName, &e.ImageUrl,
			&e.OwnerEmail, &e.OwnerName,
			&e.StartDate, &e.EndDate,
			&e.TotalPrice, &note, &e.Status, &e.CreatedAt,
		); err != nil {
			fmt.Printf("Error scanning outgoing request: %v\n", err)
			continue
		}
		if note.Valid {
			e.Note = note.String
		}
		requests = append(requests, e)
	}
	if requests == nil {
		requests = []OutEntry{}
	}
	respondWithSuccess(w, http.StatusOK, "Outgoing requests retrieved", requests)
}

// ─────────────────────────────────────────────────────────────────────────────
// ConfirmRequest  PATCH /api/rental-requests/{id}/confirm
//
// Full transactional flow:
//  1. Ensure Renter profile exists for the renter user
//  2. Create Schedule  (StartDate, EndDate)
//  3. Create RentBill  (RenterNo, RentDate=today)
//  4. Create Reservation (DeviceNo ↔ ScheduleNo)
//  5. insert_rentlist(0, DeviceNo, ScheduleNo, RentingNo) → RentListNo, RentListSeq
//  6. Device.Status → 2 (Delivered)
//  7. DeviceStatusHistory entry (StatusNo=2)
//  8. Update RentalRequest with all linked IDs + status='confirmed'
//  9. Auto-reject all other pending requests for the same device
//
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) ConfirmRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	ownerUserID := userCtx.UserId

	requestID, err := extractRentalRequestID(r.URL.Path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request ID", err.Error())
		return
	}

	// Fetch request + device owner + dates in one query.
	var deviceNo, renterUserID, deviceOwnerID int
	var reqStatus, startDate, endDate string
	err = rc.DB.QueryRow(`
		SELECT rr.DeviceNo, rr.RenterUserId, d.UserId,
		       rr.Status, rr.StartDate::TEXT, rr.EndDate::TEXT
		FROM  RentalRequest rr
		JOIN  Device d ON d.DeviceNo = rr.DeviceNo
		WHERE rr.RequestNo = $1
	`, requestID).Scan(&deviceNo, &renterUserID, &deviceOwnerID, &reqStatus, &startDate, &endDate)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Rental request not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch request", err.Error())
		return
	}
	if deviceOwnerID != ownerUserID {
		respondWithError(w, http.StatusForbidden, "Not authorized to confirm this request", "")
		return
	}
	if reqStatus != "Request Pending" {
		respondWithError(w, http.StatusBadRequest, "Request is not in pending status", "")
		return
	}

	// ── Begin transaction ──────────────────────────────────────────────────
	tx, err := rc.DB.Begin()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to start transaction", err.Error())
		return
	}
	defer tx.Rollback() // no-op after Commit

	// Step 1 – Ensure Renter profile exists (lazy upsert).
	tx.Exec("INSERT INTO Renter (UserId) VALUES ($1) ON CONFLICT (UserId) DO NOTHING", renterUserID)
	var renterNo int
	if err = tx.QueryRow(
		"SELECT RenterNo FROM Renter WHERE UserId = $1", renterUserID,
	).Scan(&renterNo); err != nil {
		fmt.Printf("Warning: could not get renterNo for UserId %d: %v\n", renterUserID, err)
	}

	// Step 2 – Create Schedule entry.
	var scheduleNo int
	if err = tx.QueryRow(
		"INSERT INTO Schedule (StartDate, EndDate) VALUES ($1, $2) RETURNING ScheduleNo",
		startDate, endDate,
	).Scan(&scheduleNo); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create schedule", err.Error())
		return
	}

	// Step 3 – Create RentBill entry (RenterNo=0 maps to NULL safely).
	var rentingNo int
	if err = tx.QueryRow(
		"INSERT INTO RentBill (RenterNo, RentDate) VALUES (NULLIF($1, 0), CURRENT_DATE) RETURNING RentingNo",
		renterNo,
	).Scan(&rentingNo); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create rent bill", err.Error())
		return
	}

	// Step 4 – Create Reservation link (device ↔ schedule).
	if _, err = tx.Exec(
		"INSERT INTO Reservation (DeviceNo, ScheduleNo) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		deviceNo, scheduleNo,
	); err != nil {
		fmt.Printf("Warning: create reservation: %v\n", err)
	}

	// Step 5 – Insert RentList row via the stored procedure.
	//          insert_rentlist(0, …) allocates a new RentListNo via seq_rentlist_no.
	var rentListNo, rentListSeq int
	if err = tx.QueryRow(
		"SELECT out_rentlistno, out_seq FROM insert_rentlist(0, $1, $2, $3)",
		deviceNo, scheduleNo, rentingNo,
	).Scan(&rentListNo, &rentListSeq); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create rent list entry", err.Error())
		return
	}

	// Step 6 – Device.Status → Delivered (2).
	if _, err = tx.Exec(
		"UPDATE Device SET Status = 2, UpdatedAt = CURRENT_TIMESTAMP WHERE DeviceNo = $1",
		deviceNo,
	); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update device status", err.Error())
		return
	}

	// Step 7 – DeviceStatusHistory: record Delivered event.
	tx.Exec(`
		INSERT INTO DeviceStatusHistory (DeviceNo, StatusNo, ChangedBy, ChangedAt)
		VALUES ($1, 2, $2, CURRENT_TIMESTAMP)
	`, deviceNo, ownerUserID)

	// Step 8 – Link all created IDs back into RentalRequest.
	if _, err = tx.Exec(`
		UPDATE RentalRequest
		SET  Status      = 'Booking Confirmed',
		     ScheduleNo  = $1,
		     RentingNo   = $2,
		     RentListNo  = $3,
		     RentListSeq = $4,
		     UpdatedAt   = CURRENT_TIMESTAMP
		WHERE RequestNo = $5
	`, scheduleNo, rentingNo, rentListNo, rentListSeq, requestID); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update rental request", err.Error())
		return
	}

	// Step 9 – Auto-reject competing pending requests for the same device.
	tx.Exec(`
		UPDATE RentalRequest
		SET Status = 'rejected', UpdatedAt = CURRENT_TIMESTAMP
		WHERE DeviceNo = $1 AND Status = 'Request Pending' AND RequestNo != $2
	`, deviceNo, requestID)

	if err = tx.Commit(); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to commit transaction", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusOK, "Rental request confirmed", map[string]interface{}{
		"requestNo":   requestID,
		"deviceNo":    deviceNo,
		"scheduleNo":  scheduleNo,
		"rentingNo":   rentingNo,
		"rentListNo":  rentListNo,
		"rentListSeq": rentListSeq,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// RejectRequest  PATCH /api/rental-requests/{id}/reject
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) RejectRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	ownerUserID := userCtx.UserId

	requestID, err := extractRentalRequestID(r.URL.Path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request ID", err.Error())
		return
	}

	var deviceOwnerID int
	var reqStatus string
	err = rc.DB.QueryRow(`
		SELECT d.UserId, rr.Status
		FROM  RentalRequest rr
		JOIN  Device d ON d.DeviceNo = rr.DeviceNo
		WHERE rr.RequestNo = $1
	`, requestID).Scan(&deviceOwnerID, &reqStatus)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Rental request not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch request", err.Error())
		return
	}
	if deviceOwnerID != ownerUserID {
		respondWithError(w, http.StatusForbidden, "Not authorized to reject this request", "")
		return
	}
	if reqStatus != "Request Pending" {
		respondWithError(w, http.StatusBadRequest, "Only pending requests can be rejected", "")
		return
	}

	if _, err = rc.DB.Exec(
		"UPDATE RentalRequest SET Status='rejected', UpdatedAt=CURRENT_TIMESTAMP WHERE RequestNo=$1",
		requestID,
	); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to reject request", err.Error())
		return
	}
	respondWithSuccess(w, http.StatusOK, "Rental request rejected", map[string]interface{}{
		"requestNo": requestID,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// CancelRequest  PATCH /api/rental-requests/{id}/cancel
// Renter cancels their own pending request.
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) CancelRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	renterUserID := userCtx.UserId

	requestID, err := extractRentalRequestID(r.URL.Path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request ID", err.Error())
		return
	}

	var reqRenterID int
	var reqStatus string
	err = rc.DB.QueryRow(
		"SELECT RenterUserId, Status FROM RentalRequest WHERE RequestNo = $1",
		requestID,
	).Scan(&reqRenterID, &reqStatus)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Rental request not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch request", err.Error())
		return
	}
	if reqRenterID != renterUserID {
		respondWithError(w, http.StatusForbidden, "Not authorized to cancel this request", "")
		return
	}
	if reqStatus != "Request Pending" {
		respondWithError(w, http.StatusBadRequest, "Only pending requests can be cancelled", "")
		return
	}

	if _, err = rc.DB.Exec(
		"UPDATE RentalRequest SET Status='cancelled', UpdatedAt=CURRENT_TIMESTAMP WHERE RequestNo=$1",
		requestID,
	); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to cancel request", err.Error())
		return
	}
	respondWithSuccess(w, http.StatusOK, "Rental request cancelled", map[string]interface{}{
		"requestNo": requestID,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// MarkActive  PATCH /api/rental-requests/{id}/active
// Owner marks the device as actively being rented (picked up by renter).
// Transition: Booking Confirmed → Rental Active
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) MarkActive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	ownerUserID := userCtx.UserId

	requestID, err := extractRentalRequestID(r.URL.Path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request ID", err.Error())
		return
	}

	var deviceNo, deviceOwnerID int
	var reqStatus string
	err = rc.DB.QueryRow(`
		SELECT rr.DeviceNo, d.UserId, rr.Status
		FROM  RentalRequest rr
		JOIN  Device d ON d.DeviceNo = rr.DeviceNo
		WHERE rr.RequestNo = $1
	`, requestID).Scan(&deviceNo, &deviceOwnerID, &reqStatus)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Rental request not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch request", err.Error())
		return
	}
	if deviceOwnerID != ownerUserID {
		respondWithError(w, http.StatusForbidden, "Not authorized", "")
		return
	}
	if reqStatus != "Booking Confirmed" {
		respondWithError(w, http.StatusBadRequest, "Only Booking Confirmed requests can be marked as Rental Active", "")
		return
	}

	tx, err := rc.DB.Begin()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to start transaction", err.Error())
		return
	}
	defer tx.Rollback()

	// Update RentalRequest status
	if _, err = tx.Exec(
		"UPDATE RentalRequest SET Status='Rental Active', UpdatedAt=CURRENT_TIMESTAMP WHERE RequestNo=$1",
		requestID,
	); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update rental request", err.Error())
		return
	}

	// Update Device status to Rented (6)
	if _, err = tx.Exec(
		"UPDATE Device SET Status=6, UpdatedAt=CURRENT_TIMESTAMP WHERE DeviceNo=$1",
		deviceNo,
	); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update device status", err.Error())
		return
	}

	// Record DeviceStatusHistory
	tx.Exec(`
		INSERT INTO DeviceStatusHistory (DeviceNo, StatusNo, ChangedBy, ChangedAt)
		VALUES ($1, 6, $2, CURRENT_TIMESTAMP)
	`, deviceNo, ownerUserID)

	if err = tx.Commit(); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to commit transaction", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusOK, "Rental marked as active", map[string]interface{}{
		"requestNo": requestID,
		"deviceNo":  deviceNo,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// MarkReturned  PATCH /api/rental-requests/{id}/returned
//
// Transactional flow:
//  1. RentalRequest.Status → 'returned'
//  2. Device.Status → 1 (Available)
//  3. DeviceStatusHistory entry (StatusNo=3  Returned)
//  4. Legacy StatusHistory entry (only when linked to a RentList row)
//
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) MarkReturned(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	ownerUserID := userCtx.UserId

	requestID, err := extractRentalRequestID(r.URL.Path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request ID", err.Error())
		return
	}

	var deviceNo, deviceOwnerID int
	var reqStatus string
	var rentListNo, rentListSeq sql.NullInt64
	err = rc.DB.QueryRow(`
		SELECT rr.DeviceNo, d.UserId, rr.Status, rr.RentListNo, rr.RentListSeq
		FROM  RentalRequest rr
		JOIN  Device d ON d.DeviceNo = rr.DeviceNo
		WHERE rr.RequestNo = $1
	`, requestID).Scan(&deviceNo, &deviceOwnerID, &reqStatus, &rentListNo, &rentListSeq)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Rental request not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch request", err.Error())
		return
	}
	if deviceOwnerID != ownerUserID {
		respondWithError(w, http.StatusForbidden, "Not authorized", "")
		return
	}
	if reqStatus != "Booking Confirmed" && reqStatus != "Rental Active" {
		respondWithError(w, http.StatusBadRequest, "Only confirmed/active requests can be marked as returned", "")
		return
	}

	// ── Begin transaction ──────────────────────────────────────────────────
	tx, err := rc.DB.Begin()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to start transaction", err.Error())
		return
	}
	defer tx.Rollback()

	// 1 – Mark request returned.
	if _, err = tx.Exec(
		"UPDATE RentalRequest SET Status='Rental Completed', UpdatedAt=CURRENT_TIMESTAMP WHERE RequestNo=$1",
		requestID,
	); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update rental request", err.Error())
		return
	}

	// 2 – Device back to Available (1).
	if _, err = tx.Exec(
		"UPDATE Device SET Status=1, UpdatedAt=CURRENT_TIMESTAMP WHERE DeviceNo=$1",
		deviceNo,
	); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update device status", err.Error())
		return
	}

	// 3 – DeviceStatusHistory: record the Returned event (StatusNo=3).
	tx.Exec(`
		INSERT INTO DeviceStatusHistory (DeviceNo, StatusNo, ChangedBy, ChangedAt)
		VALUES ($1, 3, $2, CURRENT_TIMESTAMP)
	`, deviceNo, ownerUserID)

	// 4 – Legacy StatusHistory: only when the confirm step linked a RentList row.
	if rentListNo.Valid && rentListSeq.Valid {
		if _, err = tx.Exec(
			"INSERT INTO StatusHistory (StatusNo, HistoryDate, RentListNo, RentListSeq) VALUES (3, CURRENT_DATE, $1, $2)",
			rentListNo.Int64, rentListSeq.Int64,
		); err != nil {
			fmt.Printf("Warning: insert StatusHistory: %v\n", err)
		}
	}

	if err = tx.Commit(); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to commit transaction", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusOK, "Device marked as returned", map[string]interface{}{
		"requestNo": requestID,
		"deviceNo":  deviceNo,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// ─────────────────────────────────────────────────────────────────────────────
// ConfirmRentalFromChat  POST /api/chat/confirm-rental
//
// เจ้าของยืนยันการเช่าโดยตรงจากหน้า Chat หลังจากตกลงรายละเอียดกับผู้เช่าแล้ว
// ดำเนินการบันทึกข้อมูลทั้งหมดในตาราง:
//  1. Schedule    — ช่วงวันที่เช่า
//  2. RentBill    — ข้อมูลการเรียกเก็บเงิน
//  3. Reservation — เชื่อมอุปกรณ์กับ schedule
//  4. RentList    — รายการเช่าหลัก
//  5. Device      — อัปเดตสถานะเป็น Delivered (2)
//  6. DeviceStatusHistory — บันทึกการเปลี่ยนสถานะอุปกรณ์
//  7. StatusHistory       — บันทึกประวัติสถานะรายการเช่า
//  8. RentalRequest       — สร้างรายการด้วยสถานะ Booking Confirmed พร้อม IDs ทั้งหมด
//
// ตรวจสอบ schedule conflict ก่อนบันทึก และ auto-reject คำขอ pending อื่นๆ
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RentalController) ConfirmRentalFromChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	ownerUserID := userCtx.UserId

	var req struct {
		DeviceNo     int     `json:"deviceNo"`
		RenterUserID int     `json:"renterUserId"`
		StartDate    string  `json:"startDate"`
		EndDate      string  `json:"endDate"`
		TotalPrice   float64 `json:"totalPrice"`
		Note         string  `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if req.DeviceNo == 0 || req.RenterUserID == 0 || req.StartDate == "" || req.EndDate == "" {
		respondWithError(w, http.StatusBadRequest, "deviceNo, renterUserId, startDate and endDate are required", "")
		return
	}
	if req.StartDate > req.EndDate {
		respondWithError(w, http.StatusBadRequest, "startDate must be before or equal to endDate", "")
		return
	}

	// ── ตรวจสอบความเป็นเจ้าของและสถานะอุปกรณ์ ──────────────────────────────
	var deviceStatus, deviceOwnerID int
	err := rc.DB.QueryRow(
		"SELECT Status, UserId FROM Device WHERE DeviceNo = $1", req.DeviceNo,
	).Scan(&deviceStatus, &deviceOwnerID)
	if err == sql.ErrNoRows {
		respondWithError(w, http.StatusNotFound, "Device not found", "")
		return
	} else if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to verify device", err.Error())
		return
	}
	if deviceOwnerID != ownerUserID {
		respondWithError(w, http.StatusForbidden, "You don't own this device", "")
		return
	}
	if deviceStatus != 1 {
		respondWithError(w, http.StatusBadRequest, "Device is not available (status must be Available)", "")
		return
	}
	if req.RenterUserID == ownerUserID {
		respondWithError(w, http.StatusBadRequest, "Owner cannot rent their own device", "")
		return
	}

	// ── ตรวจสอบ Schedule conflict ────────────────────────────────────────────
	var conflictCount int
	err = rc.DB.QueryRow(`
		SELECT COUNT(*)
		FROM   Schedule sc
		JOIN   Reservation rv ON rv.ScheduleNo = sc.ScheduleNo
		LEFT JOIN RentalRequest rr ON rr.ScheduleNo = sc.ScheduleNo
		WHERE  rv.DeviceNo = $1
		  AND  NOT (sc.EndDate < $2::DATE OR sc.StartDate > $3::DATE)
		  AND  (rr.RequestNo IS NULL
		        OR rr.Status IN ('Request Pending', 'Booking Confirmed', 'Rental Active'))
	`, req.DeviceNo, req.StartDate, req.EndDate).Scan(&conflictCount)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to check schedule conflict", err.Error())
		return
	}
	if conflictCount > 0 {
		respondWithError(w, http.StatusConflict, "Device is already booked during this period", "")
		return
	}

	// ── Begin transaction ────────────────────────────────────────────────────
	tx, err := rc.DB.Begin()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to start transaction", err.Error())
		return
	}
	defer tx.Rollback()

	// ขั้นที่ 1 — Ensure Renter profile exists
	tx.Exec("INSERT INTO Renter (UserId) VALUES ($1) ON CONFLICT (UserId) DO NOTHING", req.RenterUserID)
	var renterNo int
	if err = tx.QueryRow(
		"SELECT RenterNo FROM Renter WHERE UserId = $1", req.RenterUserID,
	).Scan(&renterNo); err != nil {
		fmt.Printf("Warning: could not get renterNo for UserId %d: %v\n", req.RenterUserID, err)
	}

	// ขั้นที่ 2 — สร้าง Schedule
	var scheduleNo int
	if err = tx.QueryRow(
		"INSERT INTO Schedule (StartDate, EndDate) VALUES ($1, $2) RETURNING ScheduleNo",
		req.StartDate, req.EndDate,
	).Scan(&scheduleNo); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create schedule", err.Error())
		return
	}

	// ขั้นที่ 3 — สร้าง RentBill
	var rentingNo int
	if err = tx.QueryRow(
		"INSERT INTO RentBill (RenterNo, RentDate) VALUES (NULLIF($1, 0), CURRENT_DATE) RETURNING RentingNo",
		renterNo,
	).Scan(&rentingNo); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create rent bill", err.Error())
		return
	}

	// ขั้นที่ 4 — สร้าง Reservation (device ↔ schedule)
	if _, err = tx.Exec(
		"INSERT INTO Reservation (DeviceNo, ScheduleNo) VALUES ($1, $2) ON CONFLICT DO NOTHING",
		req.DeviceNo, scheduleNo,
	); err != nil {
		fmt.Printf("Warning: create reservation: %v\n", err)
	}

	// ขั้นที่ 5 — Insert RentList (ใช้ RentListNo=0 ตาม convention, ดึง seq อัตโนมัติ)
	var rentListNo, rentListSeq int
	if err = tx.QueryRow(`
		INSERT INTO RentList (RentListNo, RentListSeq, DeviceNo, ScheduleNo, RentingNo)
		VALUES (
			0,
			(SELECT COALESCE(MAX(RentListSeq), 0) + 1 FROM RentList WHERE RentListNo = 0),
			$1, $2, $3
		)
		RETURNING RentListNo, RentListSeq
	`, req.DeviceNo, scheduleNo, rentingNo).Scan(&rentListNo, &rentListSeq); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create rent list entry", err.Error())
		return
	}

	// ขั้นที่ 6 — อัปเดตสถานะอุปกรณ์: Reserved (5) ถ้าเริ่มในอนาคต, Rented (6) ถ้าเริ่มวันนี้หรือก่อนหน้า
	newDeviceStatus := 5 // Reserved by default
	if req.StartDate <= time.Now().Format("2006-01-02") {
		newDeviceStatus = 6 // Rented — rental starts today or earlier
	}
	if _, err = tx.Exec(
		"UPDATE Device SET Status = $1 WHERE DeviceNo = $2",
		newDeviceStatus, req.DeviceNo,
	); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update device status", err.Error())
		return
	}

	// ขั้นที่ 7 — บันทึก DeviceStatusHistory (StatusNo = Reserved หรือ Rented)
	tx.Exec(`
		INSERT INTO DeviceStatusHistory (DeviceNo, StatusNo, ChangedBy, ChangedAt)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
	`, req.DeviceNo, newDeviceStatus, ownerUserID)

	// ขั้นที่ 8 — บันทึก StatusHistory สำหรับรายการเช่า (StatusNo=2 = Booking Confirmed)
	tx.Exec(`
		INSERT INTO StatusHistory (StatusNo, HistoryDate, RentListNo, RentListSeq)
		VALUES (2, CURRENT_DATE, $1, $2)
	`, rentListNo, rentListSeq)

	// ขั้นที่ 9 — สร้าง RentalRequest พร้อมสถานะ Booking Confirmed และ IDs ทั้งหมด
	var requestNo int
	if err = tx.QueryRow(`
		INSERT INTO RentalRequest
			(DeviceNo, RenterUserId, StartDate, EndDate, TotalPrice, Note,
			 ScheduleNo, RentingNo, RentListNo, RentListSeq,
			 Status, CreatedAt, UpdatedAt)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''),
		        $7, $8, $9, $10,
		        'Booking Confirmed', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING RequestNo
	`, req.DeviceNo, req.RenterUserID, req.StartDate, req.EndDate, req.TotalPrice, req.Note,
		scheduleNo, rentingNo, rentListNo, rentListSeq,
	).Scan(&requestNo); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create rental request", err.Error())
		return
	}

	// ขั้นที่ 10 — Auto-reject คำขอ pending อื่นๆ สำหรับอุปกรณ์เดียวกัน
	tx.Exec(`
		UPDATE RentalRequest
		SET Status = 'rejected', UpdatedAt = CURRENT_TIMESTAMP
		WHERE DeviceNo = $1 AND Status = 'Request Pending' AND RequestNo != $2
	`, req.DeviceNo, requestNo)

	if err = tx.Commit(); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to commit transaction", err.Error())
		return
	}

	fmt.Printf("ConfirmRentalFromChat: DeviceNo=%d, Renter=%d, Owner=%d, Schedule=%d, RentBill=%d, RentList=(%d,%d), Request=%d\n",
		req.DeviceNo, req.RenterUserID, ownerUserID, scheduleNo, rentingNo, rentListNo, rentListSeq, requestNo)

	respondWithSuccess(w, http.StatusCreated, "Rental confirmed successfully", map[string]interface{}{
		"requestNo":   requestNo,
		"deviceNo":    req.DeviceNo,
		"scheduleNo":  scheduleNo,
		"rentingNo":   rentingNo,
		"rentListNo":  rentListNo,
		"rentListSeq": rentListSeq,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// extractRentalRequestID parses the numeric ID from paths like
//
//	/api/rental-requests/42/confirm  →  42
func extractRentalRequestID(path string) (int, error) {
	parts := strings.Split(strings.TrimPrefix(path, "/api/rental-requests/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return 0, fmt.Errorf("missing request ID")
	}
	return strconv.Atoi(parts[0])
}
