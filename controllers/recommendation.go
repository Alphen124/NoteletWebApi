package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"noteletwebservice-development/middlewares"
)

// RecommendationController handles hybrid recommendation logic.
type RecommendationController struct {
	DB *sql.DB
}

func NewRecommendationController(db *sql.DB) *RecommendationController {
	return &RecommendationController{DB: db}
}

// event weights (same as Python reference)
const (
	wView  = 1.0
	wClick = 2.0
	wRent  = 5.0
	decay  = 0.97 // per-day decay factor
)

func evtWeight(t string) float64 {
	switch t {
	case "click":
		return wClick
	case "rent":
		return wRent
	default:
		return wView
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// POST /api/devices/events  — log a user interaction
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RecommendationController) LogEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}

	var req struct {
		DeviceNo  int    `json:"deviceNo"`
		EventType string `json:"eventType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}
	if req.DeviceNo == 0 {
		respondWithError(w, http.StatusBadRequest, "deviceNo is required", "")
		return
	}
	// Sanitise event type
	switch req.EventType {
	case "view", "click", "rent":
	default:
		req.EventType = "view"
	}

	_, err := rc.DB.Exec(
		`INSERT INTO DeviceEvent (UserId, DeviceNo, EventType) VALUES ($1, $2, $3)`,
		userCtx.UserId, req.DeviceNo, req.EventType,
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to log event", err.Error())
		return
	}
	respondWithSuccess(w, http.StatusOK, "Event logged", nil)
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /api/recommendations  — return top-10 recommended devices for the caller
// ─────────────────────────────────────────────────────────────────────────────

type recEventRow struct {
	UserID    int
	DeviceNo  int
	EventType string
	CreatedAt time.Time
	TypeNo    int
	CPU       string
	RAM       string
}

type recDeviceResult struct {
	DeviceNo    int     `json:"deviceNo"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	Type        string  `json:"type"`
	Rating      float64 `json:"rating"`
	Status      string  `json:"status"`
	ImageUrl    string  `json:"imageUrl"`
	CPU         string  `json:"cpu"`
	RAM         string  `json:"ram"`
}

func (rc *RecommendationController) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}

	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	userID := userCtx.UserId
	const topN = 10

	// ── 1. Load recent events (last 90 days) ────────────────────────────────
	rows, err := rc.DB.Query(`
		SELECT e.UserId, e.DeviceNo, e.EventType, e.CreatedAt,
		       COALESCE(d.DeviceTypeNo, 0),
		       COALESCE(d.CPU, ''),
		       COALESCE(d.RAM, '')
		FROM DeviceEvent e
		JOIN Device d ON d.DeviceNo = e.DeviceNo
		WHERE e.CreatedAt >= NOW() - INTERVAL '90 days'
	`)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to load events", err.Error())
		return
	}
	defer rows.Close()

	var events []recEventRow
	for rows.Next() {
		var ev recEventRow
		if err2 := rows.Scan(&ev.UserID, &ev.DeviceNo, &ev.EventType, &ev.CreatedAt,
			&ev.TypeNo, &ev.CPU, &ev.RAM); err2 != nil {
			continue
		}
		events = append(events, ev)
	}

	now := time.Now()

	// ── 2. Build user-item matrix & popularity ───────────────────────────────
	// matrix[userId][deviceNo] = aggregated decayed score
	matrix := make(map[int]map[int]float64)
	popularity := make(map[int]float64)
	itemMeta := make(map[int]recEventRow) // last-seen metadata per device

	for _, ev := range events {
		days := now.Sub(ev.CreatedAt).Hours() / 24
		score := evtWeight(ev.EventType) * math.Pow(decay, days)

		if matrix[ev.UserID] == nil {
			matrix[ev.UserID] = make(map[int]float64)
		}
		matrix[ev.UserID][ev.DeviceNo] += score
		popularity[ev.DeviceNo] += score
		itemMeta[ev.DeviceNo] = ev
	}

	// Normalize per-user
	for uid := range matrix {
		var mx float64
		for _, s := range matrix[uid] {
			if s > mx {
				mx = s
			}
		}
		if mx > 0 {
			for did := range matrix[uid] {
				matrix[uid][did] /= mx
			}
		}
	}

	// Normalize popularity
	var maxPop float64
	for _, p := range popularity {
		if p > maxPop {
			maxPop = p
		}
	}
	if maxPop > 0 {
		for did := range popularity {
			popularity[did] /= maxPop
		}
	}

	// ── 3. Build item vectors (device → user scores) for cosine sim ──────────
	itemVec := make(map[int]map[int]float64)
	for uid, uMap := range matrix {
		for did, s := range uMap {
			if itemVec[did] == nil {
				itemVec[did] = make(map[int]float64)
			}
			itemVec[did][uid] = s
		}
	}

	itemNorm := make(map[int]float64)
	for did, vec := range itemVec {
		var n float64
		for _, v := range vec {
			n += v * v
		}
		itemNorm[did] = math.Sqrt(n)
	}

	collabSim := func(i, j int) float64 {
		vi, oki := itemVec[i]
		vj, okj := itemVec[j]
		if !oki || !okj {
			return 0
		}
		var dot float64
		for uid, si := range vi {
			if sj, ok := vj[uid]; ok {
				dot += si * sj
			}
		}
		return dot / (itemNorm[i]*itemNorm[j] + 1e-9)
	}

	contentSim := func(i, j int) float64 {
		mi, oki := itemMeta[i]
		mj, okj := itemMeta[j]
		if !oki || !okj {
			return 0
		}
		var sim float64
		if mi.TypeNo > 0 && mi.TypeNo == mj.TypeNo {
			sim += 0.5
		}
		if mi.CPU != "" && strings.EqualFold(mi.CPU, mj.CPU) {
			sim += 0.3
		}
		if mi.RAM != "" && strings.EqualFold(mi.RAM, mj.RAM) {
			sim += 0.2
		}
		return sim
	}

	// ── 4. User profile: favourite device type ───────────────────────────────
	typeScore := make(map[int]float64)
	if uMap, ok := matrix[userID]; ok {
		for did, s := range uMap {
			if meta, ok := itemMeta[did]; ok && meta.TypeNo > 0 {
				typeScore[meta.TypeNo] += s
			}
		}
	}
	favType := 0
	var favTypeScore float64
	for t, s := range typeScore {
		if s > favTypeScore {
			favTypeScore = s
			favType = t
		}
	}

	// ── 5. Collect all device IDs we know about ──────────────────────────────
	allDevices := make([]int, 0, len(itemVec))
	for d := range itemVec {
		allDevices = append(allDevices, d)
	}

	// ── 6. Score candidates ───────────────────────────────────────────────────
	userVector := matrix[userID]
	candidateScores := make(map[int]float64)

	if len(userVector) > 0 {
		for interacted, uScore := range userVector {
			if uScore < 0.05 {
				continue
			}
			for _, candidate := range allDevices {
				if userVector[candidate] > 0 {
					continue // already interacted
				}
				cs := collabSim(interacted, candidate)
				cnt := contentSim(interacted, candidate)
				pop := popularity[candidate]
				score := 0.4*cs + 0.3*cnt + 0.2*pop

				if favType > 0 {
					if meta, ok := itemMeta[candidate]; ok && meta.TypeNo == favType {
						score *= 1.3
					}
				}
				candidateScores[candidate] += score
			}
		}
	}

	// ── 7. Sort candidates ────────────────────────────────────────────────────
	type scored struct {
		DeviceNo int
		Score    float64
	}
	ranked := make([]scored, 0, len(candidateScores))
	for d, s := range candidateScores {
		ranked = append(ranked, scored{d, s})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	result := make([]int, 0, topN)
	for _, r := range ranked {
		if len(result) >= topN {
			break
		}
		result = append(result, r.DeviceNo)
	}

	// ── 8. Fallback: fill with popular items ─────────────────────────────────
	if len(result) < topN {
		type popItem struct {
			DeviceNo int
			Pop      float64
		}
		popList := make([]popItem, 0, len(popularity))
		for d, p := range popularity {
			popList = append(popList, popItem{d, p})
		}
		sort.Slice(popList, func(i, j int) bool {
			return popList[i].Pop > popList[j].Pop
		})
		resultSet := make(map[int]bool)
		for _, d := range result {
			resultSet[d] = true
		}
		for _, pi := range popList {
			if len(result) >= topN {
				break
			}
			if !resultSet[pi.DeviceNo] {
				result = append(result, pi.DeviceNo)
				resultSet[pi.DeviceNo] = true
			}
		}
	}

	if len(result) == 0 {
		respondWithSuccess(w, http.StatusOK, "No recommendations", []interface{}{})
		return
	}

	// ── 9. Load full device details for the recommended IDs ──────────────────
	placeholders := make([]string, len(result))
	args := make([]interface{}, len(result))
	for i, id := range result {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	dRows, err := rc.DB.Query(fmt.Sprintf(`
		SELECT d.DeviceNo, d.DeviceName, COALESCE(d.Description,''),
		       d.RentPrice,
		       COALESCE(dt.DeviceTypeName,'') AS TypeName,
		       COALESCE(d.Rating,0),
		       COALESCE(s.Name, 'Available') AS StatusName,
		       COALESCE(d.ImageUrl,''),
		       COALESCE(d.CPU,''), COALESCE(d.RAM,'')
		FROM Device d
		LEFT JOIN DeviceType dt ON dt.DeviceTypeNo = d.DeviceTypeNo
		LEFT JOIN Status s      ON s.StatusNo = d.Status
		WHERE d.DeviceNo IN (%s)
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to load device details", err.Error())
		return
	}
	defer dRows.Close()

	deviceMap := make(map[int]recDeviceResult)
	for dRows.Next() {
		var dr recDeviceResult
		if err2 := dRows.Scan(
			&dr.DeviceNo, &dr.Name, &dr.Description,
			&dr.Price, &dr.Type, &dr.Rating, &dr.Status,
			&dr.ImageUrl, &dr.CPU, &dr.RAM,
		); err2 != nil {
			continue
		}
		deviceMap[dr.DeviceNo] = dr
	}

	// Return in ranked order
	finalList := make([]recDeviceResult, 0, len(result))
	for _, id := range result {
		if d, ok := deviceMap[id]; ok {
			finalList = append(finalList, d)
		}
	}

	respondWithSuccess(w, http.StatusOK, "Recommendations loaded", finalList)
}

// ─────────────────────────────────────────────────────────────────────────────
// GET /api/recommendations/metrics?k=10&test_days=14
// Offline ranking evaluation using a temporal train/test split.
// Training: events older than test_days days.
// Test:     events within the last test_days days.
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RecommendationController) GetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}
	_, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}

	// ── Parse params ─────────────────────────────────────────────────────────
	k := 10
	if kStr := r.URL.Query().Get("k"); kStr != "" {
		if kv, err := strconv.Atoi(kStr); err == nil && kv >= 1 && kv <= 50 {
			k = kv
		}
	}
	testDays := 14
	if tStr := r.URL.Query().Get("test_days"); tStr != "" {
		if tv, err := strconv.Atoi(tStr); err == nil && tv >= 1 && tv <= 30 {
			testDays = tv
		}
	}

	now := time.Now()
	splitDate := now.AddDate(0, 0, -testDays)

	// ── Load events ───────────────────────────────────────────────────────────
	rows, err := rc.DB.Query(`
		SELECT e.UserId, e.DeviceNo, e.EventType, e.CreatedAt,
		       COALESCE(d.DeviceTypeNo, 0),
		       COALESCE(d.CPU, ''),
		       COALESCE(d.RAM, '')
		FROM DeviceEvent e
		JOIN Device d ON d.DeviceNo = e.DeviceNo
		WHERE e.CreatedAt >= NOW() - INTERVAL '90 days'
	`)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to load events", err.Error())
		return
	}
	defer rows.Close()

	var trainEvents, testEvents []recEventRow
	for rows.Next() {
		var ev recEventRow
		if err2 := rows.Scan(&ev.UserID, &ev.DeviceNo, &ev.EventType, &ev.CreatedAt,
			&ev.TypeNo, &ev.CPU, &ev.RAM); err2 != nil {
			continue
		}
		if ev.CreatedAt.Before(splitDate) {
			trainEvents = append(trainEvents, ev)
		} else {
			testEvents = append(testEvents, ev)
		}
	}

	// ── Build training structures (mirror GetRecommendations) ─────────────────
	trainMatrix := make(map[int]map[int]float64)
	trainPop := make(map[int]float64)
	trainMeta := make(map[int]recEventRow)

	for _, ev := range trainEvents {
		days := now.Sub(ev.CreatedAt).Hours() / 24
		score := evtWeight(ev.EventType) * math.Pow(decay, days)
		if trainMatrix[ev.UserID] == nil {
			trainMatrix[ev.UserID] = make(map[int]float64)
		}
		trainMatrix[ev.UserID][ev.DeviceNo] += score
		trainPop[ev.DeviceNo] += score
		trainMeta[ev.DeviceNo] = ev
	}
	for uid := range trainMatrix {
		var mx float64
		for _, s := range trainMatrix[uid] {
			if s > mx {
				mx = s
			}
		}
		if mx > 0 {
			for did := range trainMatrix[uid] {
				trainMatrix[uid][did] /= mx
			}
		}
	}
	var maxPop float64
	for _, p := range trainPop {
		if p > maxPop {
			maxPop = p
		}
	}
	if maxPop > 0 {
		for did := range trainPop {
			trainPop[did] /= maxPop
		}
	}

	trainItemVec := make(map[int]map[int]float64)
	for uid, uMap := range trainMatrix {
		for did, s := range uMap {
			if trainItemVec[did] == nil {
				trainItemVec[did] = make(map[int]float64)
			}
			trainItemVec[did][uid] = s
		}
	}
	trainNorm := make(map[int]float64)
	for did, vec := range trainItemVec {
		var n float64
		for _, v := range vec {
			n += v * v
		}
		trainNorm[did] = math.Sqrt(n)
	}

	mCollabSim := func(i, j int) float64 {
		vi, oki := trainItemVec[i]
		vj, okj := trainItemVec[j]
		if !oki || !okj {
			return 0
		}
		var dot float64
		for uid, si := range vi {
			if sj, ok2 := vj[uid]; ok2 {
				dot += si * sj
			}
		}
		return dot / (trainNorm[i]*trainNorm[j] + 1e-9)
	}
	mContentSim := func(i, j int) float64 {
		mi, oki := trainMeta[i]
		mj, okj := trainMeta[j]
		if !oki || !okj {
			return 0
		}
		var sim float64
		if mi.TypeNo > 0 && mi.TypeNo == mj.TypeNo {
			sim += 0.5
		}
		if mi.CPU != "" && strings.EqualFold(mi.CPU, mj.CPU) {
			sim += 0.3
		}
		if mi.RAM != "" && strings.EqualFold(mi.RAM, mj.RAM) {
			sim += 0.2
		}
		return sim
	}

	allTrainDevices := make([]int, 0, len(trainItemVec))
	for d := range trainItemVec {
		allTrainDevices = append(allTrainDevices, d)
	}

	// ── Build test user→items map ─────────────────────────────────────────────
	testUserItems := make(map[int]map[int]bool)
	for _, ev := range testEvents {
		if testUserItems[ev.UserID] == nil {
			testUserItems[ev.UserID] = make(map[int]bool)
		}
		testUserItems[ev.UserID][ev.DeviceNo] = true
	}

	// ── Score ALL training-catalog devices for a user ─────────────────────────
	type rankedItem struct {
		DeviceNo int
		Score    float64
	}
	scoreUser := func(userID int) []rankedItem {
		scores := make(map[int]float64)
		// Fav type from training
		typeScoreMap := make(map[int]float64)
		if uMap, ok2 := trainMatrix[userID]; ok2 {
			for did, s := range uMap {
				if meta, ok3 := trainMeta[did]; ok3 && meta.TypeNo > 0 {
					typeScoreMap[meta.TypeNo] += s
				}
			}
		}
		favType := 0
		var favTypeScore float64
		for t, s := range typeScoreMap {
			if s > favTypeScore {
				favTypeScore = s
				favType = t
			}
		}
		userVector := trainMatrix[userID]
		if len(userVector) > 0 {
			for interacted, uScore := range userVector {
				if uScore < 0.05 {
					continue
				}
				for _, candidate := range allTrainDevices {
					cs := mCollabSim(interacted, candidate)
					cnt := mContentSim(interacted, candidate)
					pop := trainPop[candidate]
					s := 0.4*cs + 0.3*cnt + 0.2*pop
					if favType > 0 {
						if meta, ok2 := trainMeta[candidate]; ok2 && meta.TypeNo == favType {
							s *= 1.3
						}
					}
					scores[candidate] += s
				}
			}
		} else {
			// Cold-start: popularity only
			for _, candidate := range allTrainDevices {
				scores[candidate] = trainPop[candidate]
			}
		}
		ranked := make([]rankedItem, 0, len(scores))
		for d, s := range scores {
			ranked = append(ranked, rankedItem{d, s})
		}
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
		return ranked
	}

	// ── Evaluate ──────────────────────────────────────────────────────────────
	var totalPrecK, totalRecK, totalMRR, totalMAP, totalNDCG float64
	evaluatedUsers := 0

	for userID, testItems := range testUserItems {
		// At least one test item must be in the training catalog to rank it
		overlap := 0
		for tid := range testItems {
			if _, exists := trainMeta[tid]; exists {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}

		ranked := scoreUser(userID)
		topK := ranked
		if len(topK) > k {
			topK = ranked[:k]
		}

		hits := 0
		var sumP, dcg, rr float64
		rrFound := false
		for pos, item := range topK {
			if testItems[item.DeviceNo] {
				hits++
				sumP += float64(hits) / float64(pos+1)
				dcg += 1.0 / math.Log2(float64(pos+2))
				if !rrFound {
					rr = 1.0 / float64(pos+1)
					rrFound = true
				}
			}
		}

		precK := float64(hits) / float64(k)
		recK := float64(hits) / float64(len(testItems))
		var ap float64
		if len(testItems) > 0 {
			ap = sumP / float64(len(testItems))
		}

		idealHits := len(testItems)
		if idealHits > k {
			idealHits = k
		}
		var idcg float64
		for i := 0; i < idealHits; i++ {
			idcg += 1.0 / math.Log2(float64(i+2))
		}
		var ndcg float64
		if idcg > 0 {
			ndcg = dcg / idcg
		}

		totalPrecK += precK
		totalRecK += recK
		totalMRR += rr
		totalMAP += ap
		totalNDCG += ndcg
		evaluatedUsers++
	}

	if evaluatedUsers == 0 {
		respondWithSuccess(w, http.StatusOK, "Not enough data to evaluate", map[string]interface{}{
			"k":               k,
			"test_days":       testDays,
			"evaluated_users": 0,
			"train_events":    len(trainEvents),
			"test_events":     len(testEvents),
		})
		return
	}

	n := float64(evaluatedUsers)
	round4 := func(v float64) float64 { return math.Round(v*10000) / 10000 }

	respondWithSuccess(w, http.StatusOK, "Metrics computed", map[string]interface{}{
		"k":                k,
		"test_days":        testDays,
		"precision_at_k":   round4(totalPrecK / n),
		"recall_at_k":      round4(totalRecK / n),
		"mrr":              round4(totalMRR / n),
		"map":              round4(totalMAP / n),
		"ndcg_at_k":        round4(totalNDCG / n),
		"evaluated_users":  evaluatedUsers,
		"total_test_users": len(testUserItems),
		"train_events":     len(trainEvents),
		"test_events":      len(testEvents),
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// POST /api/recommendations/seed-test-data?clear=true
// Admin-only: insert synthetic DeviceEvent rows so metrics can be evaluated
// without waiting for real user interactions.
//
// Strategy (Temporal Split compatible):
//   - Users are split into preference clusters by DeviceTypeNo
//   - Training events: days -90 to -15  (75% of interactions per user)
//   - Test events:     days -14 to -1   (25% of interactions per user)
//   - 80% of test items come from the same type cluster as training
//     → the model SHOULD be able to predict them → realistic positive signal
//
// ?clear=true  — delete all existing DeviceEvent rows first (use with care)
// ─────────────────────────────────────────────────────────────────────────────

func (rc *RecommendationController) SeedTestData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Method not allowed", "")
		return
	}
	userCtx, ok := r.Context().Value(middlewares.UserContextKey).(middlewares.UserContext)
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", "")
		return
	}
	// Admin-only guard
	var isAdmin bool
	rc.DB.QueryRow(`SELECT COALESCE(is_admin, false) FROM appuser WHERE userid = $1`, userCtx.UserId).Scan(&isAdmin)
	if !isAdmin {
		respondWithError(w, http.StatusForbidden, "Admin access required", "")
		return
	}

	// Optional: clear all existing events first
	if r.URL.Query().Get("clear") == "true" {
		rc.DB.Exec(`DELETE FROM DeviceEvent`)
	}

	// ── Load real device IDs grouped by type ─────────────────────────────────
	devRows, err := rc.DB.Query(`SELECT DeviceNo, COALESCE(DeviceTypeNo, 0) FROM Device ORDER BY DeviceNo`)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to query devices", err.Error())
		return
	}
	defer devRows.Close()

	type devInfo struct {
		No     int
		TypeNo int
	}
	var allDevs []devInfo
	typeGroups := make(map[int][]int) // typeNo → []deviceNo
	for devRows.Next() {
		var d devInfo
		if devRows.Scan(&d.No, &d.TypeNo) == nil {
			allDevs = append(allDevs, d)
			typeGroups[d.TypeNo] = append(typeGroups[d.TypeNo], d.No)
		}
	}
	if len(allDevs) == 0 {
		respondWithError(w, http.StatusBadRequest, "No devices found in database", "")
		return
	}

	// Flatten type keys
	typeKeys := make([]int, 0, len(typeGroups))
	for k := range typeGroups {
		typeKeys = append(typeKeys, k)
	}
	sort.Ints(typeKeys)

	// ── Load real user IDs ────────────────────────────────────────────────────
	userRows, err := rc.DB.Query(`SELECT userid FROM appuser ORDER BY userid LIMIT 20`)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to query users", err.Error())
		return
	}
	defer userRows.Close()

	var userIDs []int
	for userRows.Next() {
		var uid int
		if userRows.Scan(&uid) == nil {
			userIDs = append(userIDs, uid)
		}
	}
	if len(userIDs) == 0 {
		respondWithError(w, http.StatusBadRequest, "No users found in database", "")
		return
	}

	// ── Generate synthetic events ─────────────────────────────────────────────
	rng := rand.New(rand.NewSource(42))
	now := time.Now()

	// Pick event type with realistic distribution: 60% view, 30% click, 10% rent
	pickEvent := func() string {
		v := rng.Float64()
		if v < 0.60 {
			return "view"
		} else if v < 0.90 {
			return "click"
		}
		return "rent"
	}

	// Each user gets a preferred type cluster
	type seedEvent struct {
		UserID    int
		DeviceNo  int
		EventType string
		CreatedAt time.Time
	}
	var events []seedEvent

	for idx, uid := range userIDs {
		// Assign preferred type
		prefTypeIdx := idx % len(typeKeys)
		prefType := typeKeys[prefTypeIdx]
		prefDevices := typeGroups[prefType]

		// Collect non-preferred devices
		var otherDevices []int
		for _, d := range allDevs {
			if d.TypeNo != prefType {
				otherDevices = append(otherDevices, d.No)
			}
		}

		// ── Training events: days -90 to -15 ──────────────────────────────────
		// Each user interacts with 60% of preferred devices + a few others in training
		rng.Shuffle(len(prefDevices), func(i, j int) { prefDevices[i], prefDevices[j] = prefDevices[j], prefDevices[i] })
		trainCount := int(float64(len(prefDevices)) * 0.60)
		if trainCount < 2 {
			trainCount = len(prefDevices)
		}
		trainDevices := prefDevices[:trainCount]

		// Add a couple of random non-preferred for diversity
		if len(otherDevices) > 0 {
			extras := 2
			if extras > len(otherDevices) {
				extras = len(otherDevices)
			}
			rng.Shuffle(len(otherDevices), func(i, j int) { otherDevices[i], otherDevices[j] = otherDevices[j], otherDevices[i] })
			trainDevices = append(trainDevices, otherDevices[:extras]...)
		}

		for _, did := range trainDevices {
			// Random day between -90 and -15 (training period)
			daysAgo := 15 + rng.Intn(75)
			t := now.AddDate(0, 0, -daysAgo).Add(-time.Duration(rng.Intn(86400)) * time.Second)
			events = append(events, seedEvent{uid, did, pickEvent(), t})
			// Some devices get multiple events (simulate interest)
			if rng.Float64() < 0.30 {
				t2 := t.Add(time.Duration(rng.Intn(3600)+1) * time.Second)
				events = append(events, seedEvent{uid, did, pickEvent(), t2})
			}
		}

		// ── Test events: days -14 to -1 ───────────────────────────────────────
		// 80% from preferred type (items NOT seen in training), 20% random
		trainingSet := make(map[int]bool)
		for _, did := range trainDevices {
			trainingSet[did] = true
		}
		var unseenPref []int
		for _, did := range prefDevices {
			if !trainingSet[did] {
				unseenPref = append(unseenPref, did)
			}
		}

		testDevices := []int{}
		// Up to 3 unseen preferred
		if len(unseenPref) > 0 {
			rng.Shuffle(len(unseenPref), func(i, j int) { unseenPref[i], unseenPref[j] = unseenPref[j], unseenPref[i] })
			n := 3
			if n > len(unseenPref) {
				n = len(unseenPref)
			}
			testDevices = append(testDevices, unseenPref[:n]...)
		}
		// 1 random unseen from other types
		var unseenOther []int
		for _, did := range otherDevices {
			if !trainingSet[did] {
				unseenOther = append(unseenOther, did)
			}
		}
		if len(unseenOther) > 0 {
			rng.Shuffle(len(unseenOther), func(i, j int) { unseenOther[i], unseenOther[j] = unseenOther[j], unseenOther[i] })
			testDevices = append(testDevices, unseenOther[0])
		}

		for _, did := range testDevices {
			daysAgo := 1 + rng.Intn(13) // -14 to -1 days
			t := now.AddDate(0, 0, -daysAgo).Add(-time.Duration(rng.Intn(86400)) * time.Second)
			events = append(events, seedEvent{uid, did, pickEvent(), t})
		}
	}

	// ── Batch insert ─────────────────────────────────────────────────────────
	tx, err := rc.DB.Begin()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "DB transaction failed", err.Error())
		return
	}
	stmt, err := tx.Prepare(`INSERT INTO DeviceEvent (UserId, DeviceNo, EventType, CreatedAt) VALUES ($1, $2, $3, $4)`)
	if err != nil {
		tx.Rollback()
		respondWithError(w, http.StatusInternalServerError, "Prepare failed", err.Error())
		return
	}
	defer stmt.Close()

	inserted := 0
	for _, ev := range events {
		if _, err2 := stmt.Exec(ev.UserID, ev.DeviceNo, ev.EventType, ev.CreatedAt); err2 == nil {
			inserted++
		}
	}
	if err = tx.Commit(); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Commit failed", err.Error())
		return
	}

	respondWithSuccess(w, http.StatusOK, "Seed data inserted", map[string]interface{}{
		"inserted_events": inserted,
		"users":           len(userIDs),
		"devices":         len(allDevs),
		"type_groups":     len(typeGroups),
	})
}
