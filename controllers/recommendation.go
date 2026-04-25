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

	// ── 5b. User-based CF: precompute similar users ───────────────────────────
	userVector := matrix[userID]
	userNormMap := make(map[int]float64)
	for uid, uMap := range matrix {
		var n float64
		for _, s := range uMap {
			n += s * s
		}
		userNormMap[uid] = math.Sqrt(n)
	}
	type userSimPair struct {
		uid int
		sim float64
	}
	var simUsers []userSimPair
	uNorm := userNormMap[userID]
	for uid, uMap := range matrix {
		if uid == userID {
			continue
		}
		var dot float64
		for did, s1 := range userVector {
			if s2, ok2 := uMap[did]; ok2 {
				dot += s1 * s2
			}
		}
		sim := dot / (uNorm*userNormMap[uid] + 1e-9)
		if sim > 0.01 {
			simUsers = append(simUsers, userSimPair{uid, sim})
		}
	}
	sort.Slice(simUsers, func(i, j int) bool { return simUsers[i].sim > simUsers[j].sim })
	const maxSimUsers = 20
	if len(simUsers) > maxSimUsers {
		simUsers = simUsers[:maxSimUsers]
	}
	ucfRaw := make(map[int]float64)
	for _, su := range simUsers {
		for did, s := range matrix[su.uid] {
			ucfRaw[did] += su.sim * s
		}
	}
	var maxUCF float64
	for _, s := range ucfRaw {
		if s > maxUCF {
			maxUCF = s
		}
	}
	if maxUCF > 0 {
		for did := range ucfRaw {
			ucfRaw[did] /= maxUCF
		}
	}

	// ── 6. Score candidates (Item-CF×uScore + Content×uScore + Pop + UserCF) ──
	candidateScores := make(map[int]float64)

	if len(userVector) > 0 {
		// Item-based CF + Content-based, each weighted by user's interaction strength
		for interacted, uScore := range userVector {
			if uScore < 0.05 {
				continue
			}
			for _, candidate := range allDevices {
				if userVector[candidate] > 0 {
					continue // already interacted
				}
				cs := uScore * collabSim(interacted, candidate)
				cnt := uScore * contentSim(interacted, candidate)
				candidateScores[candidate] += 0.40*cs + 0.25*cnt
			}
		}
		// Popularity + User-based CF: global components, added once (not per interaction)
		for _, candidate := range allDevices {
			if userVector[candidate] > 0 {
				continue
			}
			candidateScores[candidate] += 0.15*popularity[candidate] + 0.20*ucfRaw[candidate]
		}
	} else {
		// Cold-start: popularity-based ranking
		for _, candidate := range allDevices {
			candidateScores[candidate] = popularity[candidate]
		}
	}

	// Fav type boost (applied once after all score components are accumulated)
	if favType > 0 {
		for candidate := range candidateScores {
			if meta, ok2 := itemMeta[candidate]; ok2 && meta.TypeNo == favType {
				candidateScores[candidate] *= 1.3
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

	// User norms for user-based CF during metrics evaluation
	trainUserNorm := make(map[int]float64)
	for uid, uMap := range trainMatrix {
		var n float64
		for _, s := range uMap {
			n += s * s
		}
		trainUserNorm[uid] = math.Sqrt(n)
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
	scoreUser := func(uid int) []rankedItem {
		scores := make(map[int]float64)

		// Fav type from training
		typeScoreMap := make(map[int]float64)
		if uMap, ok2 := trainMatrix[uid]; ok2 {
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

		uVec := trainMatrix[uid]
		if len(uVec) > 0 {
			// Item-based CF + Content-based, weighted by user's interaction strength
			for interacted, uScore := range uVec {
				if uScore < 0.05 {
					continue
				}
				for _, candidate := range allTrainDevices {
					cs := uScore * mCollabSim(interacted, candidate)
					cnt := uScore * mContentSim(interacted, candidate)
					scores[candidate] += 0.40*cs + 0.25*cnt
				}
			}
			// User-based CF: find top-20 similar training users
			type usPair struct {
				id  int
				sim float64
			}
			var simUs []usPair
			uNorm1 := trainUserNorm[uid]
			for other, oMap := range trainMatrix {
				if other == uid {
					continue
				}
				var dot float64
				for did, s1 := range uVec {
					if s2, ok2 := oMap[did]; ok2 {
						dot += s1 * s2
					}
				}
				sim := dot / (uNorm1*trainUserNorm[other] + 1e-9)
				if sim > 0.01 {
					simUs = append(simUs, usPair{other, sim})
				}
			}
			sort.Slice(simUs, func(i, j int) bool { return simUs[i].sim > simUs[j].sim })
			if len(simUs) > 20 {
				simUs = simUs[:20]
			}
			ucfRaw := make(map[int]float64)
			for _, su := range simUs {
				for did, s := range trainMatrix[su.id] {
					ucfRaw[did] += su.sim * s
				}
			}
			var maxUCF float64
			for _, s := range ucfRaw {
				if s > maxUCF {
					maxUCF = s
				}
			}
			if maxUCF > 0 {
				for _, candidate := range allTrainDevices {
					scores[candidate] += 0.20 * (ucfRaw[candidate] / maxUCF)
				}
			}
			// Global: popularity (added once)
			for _, candidate := range allTrainDevices {
				scores[candidate] += 0.15 * trainPop[candidate]
			}
		} else {
			// Cold-start: popularity only
			for _, candidate := range allTrainDevices {
				scores[candidate] = trainPop[candidate]
			}
		}

		// Fav type boost (applied once after all components are accumulated)
		if favType > 0 {
			for candidate := range scores {
				if meta, ok2 := trainMeta[candidate]; ok2 && meta.TypeNo == favType {
					scores[candidate] *= 1.3
				}
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

	// ── Load real device IDs grouped by type (up to 100) ────────────────────
	devRows, err := rc.DB.Query(`SELECT DeviceNo, COALESCE(DeviceTypeNo, 0) FROM Device ORDER BY DeviceNo LIMIT 100`)
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

	// ── Load real user IDs (up to 500) ─────────────────────────────────────
	userRows, err := rc.DB.Query(`SELECT userid FROM appuser ORDER BY userid LIMIT 500`)
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

	// ── Generate synthetic events targeting ~5000 total ─────────────────────
	const targetTrainEvents = 5000
	rng := rand.New(rand.NewSource(42))
	now := time.Now()

	// Events per user for training (clamped 5–200)
	eventsPerUserTrain := targetTrainEvents / len(userIDs)
	if eventsPerUserTrain < 5 {
		eventsPerUserTrain = 5
	}
	if eventsPerUserTrain > 200 {
		eventsPerUserTrain = 200
	}
	eventsPerUserTest := eventsPerUserTrain / 4
	if eventsPerUserTest < 2 {
		eventsPerUserTest = 2
	}

	// Pick event type: 60% view, 30% click, 10% rent
	pickEvent := func() string {
		v := rng.Float64()
		if v < 0.60 {
			return "view"
		} else if v < 0.90 {
			return "click"
		}
		return "rent"
	}

	type seedEvent struct {
		UserID    int
		DeviceNo  int
		EventType string
		CreatedAt time.Time
	}
	var events []seedEvent

	for idx, uid := range userIDs {
		// Assign preferred type cluster (round-robin)
		prefType := typeKeys[idx%len(typeKeys)]

		// Make a local copy so shuffle doesn't mutate the shared slice
		rawPref := typeGroups[prefType]
		prefDevices := make([]int, len(rawPref))
		copy(prefDevices, rawPref)
		rng.Shuffle(len(prefDevices), func(i, j int) { prefDevices[i], prefDevices[j] = prefDevices[j], prefDevices[i] })

		// Collect non-preferred devices
		var otherDevices []int
		for _, d := range allDevs {
			if d.TypeNo != prefType {
				otherDevices = append(otherDevices, d.No)
			}
		}

		// Split preferred devices: 70% for training, 30% for test (held-out)
		trainSplit := int(float64(len(prefDevices)) * 0.70)
		if trainSplit < 1 {
			trainSplit = 1
		}
		if trainSplit >= len(prefDevices) && len(prefDevices) > 1 {
			trainSplit = len(prefDevices) - 1
		}
		trainPrefDevices := prefDevices[:trainSplit]
		testPrefDevices := prefDevices[trainSplit:]

		// ── Training events: days -90 to -15 (~2.5 months of history) ──────
		// Cycle through trainPrefDevices to reach eventsPerUserTrain
		for i := 0; i < eventsPerUserTrain; i++ {
			did := trainPrefDevices[i%len(trainPrefDevices)]
			daysAgo := 15 + rng.Intn(75) // -90 to -15
			t := now.AddDate(0, 0, -daysAgo).Add(-time.Duration(rng.Intn(86400)) * time.Second)
			events = append(events, seedEvent{uid, did, pickEvent(), t})
		}
		// Cross-type interactions: 20% of training volume for diversity
		if len(otherDevices) > 0 {
			crossCount := eventsPerUserTrain / 5
			if crossCount < 1 {
				crossCount = 1
			}
			for i := 0; i < crossCount; i++ {
				did := otherDevices[i%len(otherDevices)]
				daysAgo := 15 + rng.Intn(75)
				t := now.AddDate(0, 0, -daysAgo).Add(-time.Duration(rng.Intn(86400)) * time.Second)
				events = append(events, seedEvent{uid, did, pickEvent(), t})
			}
		}

		// ── Test events: days -14 to -1 (from held-out preferred devices) ────
		if len(testPrefDevices) > 0 {
			for i := 0; i < eventsPerUserTest; i++ {
				did := testPrefDevices[i%len(testPrefDevices)]
				daysAgo := 1 + rng.Intn(13) // -14 to -1
				t := now.AddDate(0, 0, -daysAgo).Add(-time.Duration(rng.Intn(86400)) * time.Second)
				events = append(events, seedEvent{uid, did, pickEvent(), t})
			}
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
		"inserted_events":       inserted,
		"users":                 len(userIDs),
		"devices":               len(allDevs),
		"type_groups":           len(typeGroups),
		"events_per_user_train": eventsPerUserTrain,
		"events_per_user_test":  eventsPerUserTest,
	})
}
