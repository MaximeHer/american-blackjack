// Commande server : expose la table de blackjack et les données d'analyse à
// l'interface web.
//
// Ce serveur n'est PAS l'objet des mesures de performance. Il réutilise le
// moteur sans le modifier, et la boucle de simulation mesurée reste intacte.
// L'interface est un observateur extérieur : elle lit un état, elle ne
// s'intercale jamais dans le chemin chaud.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/MaximeHer/american-blackjack/internal/blackjack"
	"github.com/MaximeHer/american-blackjack/internal/metrics"
)

//go:embed all:web
var webFS embed.FS

// session garde l'unique table du serveur. L'application est locale et
// mono-joueur ; un mutex suffit à sérialiser les actions.
type session struct {
	mu    sync.Mutex
	table *blackjack.Table
	rules blackjack.Rules
	start float64
}

func main() {
	// Port 8090 et non 8080 : ce dernier est très fréquemment occupé, en
	// particulier par le listener HTTP d'Oracle XE (TNSLSNR) qui répond un
	// 401 et fait croire à une demande d'authentification de notre serveur.
	addr := flag.String("addr", ":8090", "adresse d'écoute")
	bankroll := flag.Float64("bankroll", 1000, "solde initial")
	decks := flag.Int("decks", 4, "nombre de jeux")
	h17 := flag.Bool("h17", false, "le croupier tire sur 17 souple")
	seed := flag.Int64("seed", 0, "graine du sabot ; 0 pour une partie non reproductible")
	flag.Parse()

	rules := blackjack.DefaultRules()
	rules.NumDecks = *decks
	rules.DealerHitsSoft17 = *h17

	s := &session{rules: rules, start: *bankroll}
	s.reset(*seed)

	content, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(content)))
	mux.HandleFunc("/api/state", s.handleState)
	mux.HandleFunc("/api/deal", s.handleDeal)
	mux.HandleFunc("/api/action", s.handleAction)
	mux.HandleFunc("/api/reset", s.handleReset)
	mux.HandleFunc("/api/strategy", handleStrategy)
	mux.HandleFunc("/api/simulate", handleSimulate)
	mux.HandleFunc("/api/curve", handleCurve)
	mux.HandleFunc("/api/rules-comparison", handleRulesComparison)
	mux.HandleFunc("/api/sample-round", handleSampleRound)
	mux.HandleFunc("/api/simulate/stream", handleSimulateStream)

	// Le port est réservé AVANT d'annoncer l'URL : sinon, en cas de conflit,
	// le serveur affiche une adresse joignable alors qu'il n'a rien pris, et
	// le navigateur atterrit sur le service qui occupe déjà le port.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Impossible d'ouvrir %s : %v\n\n", *addr, err)
		fmt.Fprintf(os.Stderr, "Le port est probablement déjà pris par un autre service.\n")
		fmt.Fprintf(os.Stderr, "Relance en choisissant un autre port, par exemple :\n")
		fmt.Fprintf(os.Stderr, "    go run ./cmd/server -addr :8091\n")
		os.Exit(1)
	}

	fmt.Printf("Table ouverte sur http://localhost%s\n", *addr)
	fmt.Printf("Règles : %d jeux, croupier %s, blackjack payé %.2f:1\n",
		rules.NumDecks,
		map[bool]string{true: "tire sur 17 souple", false: "reste sur 17 souple"}[rules.DealerHitsSoft17],
		rules.BlackjackPayout)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(srv.Serve(ln))
}

func (s *session) reset(seed int64) {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	s.table = blackjack.NewTable(s.rules, s.start, seed)
}

func (s *session) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, s.table.View())
}

func (s *session) handleDeal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Bet      float64            `json:"bet"`
		SideBets blackjack.SideBets `json:"sideBets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "requête illisible")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.table.Deal(req.Bet, req.SideBets); err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, s.table.View())
}

func (s *session) handleAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "requête illisible")
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.table.Act(req.Action); err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, s.table.View())
}

func (s *session) handleReset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reset(0)
	writeJSON(w, s.table.View())
}

func handleStrategy(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, blackjack.ExportStrategy())
}

// handleSimulate lance une simulation et renvoie ses statistiques ainsi que le
// débit observé. C'est la mesure de débit telle que l'interface la montre ;
// les mesures qui comptent pour l'audit passent par le harnais dédié, pas par
// une requête HTTP.
func handleSimulate(w http.ResponseWriter, r *http.Request) {
	rounds := intParam(r, "rounds", 1_000_000, 1_000, 50_000_000)
	seed := int64(intParam(r, "seed", 42, 0, 1<<30))

	rules := blackjack.DefaultRules()
	rules.NumDecks = intParam(r, "decks", 4, 1, 8)
	rules.DealerHitsSoft17 = r.URL.Query().Get("h17") == "true"

	var side blackjack.SideBets
	if r.URL.Query().Get("sidebets") == "true" {
		side = blackjack.SideBets{PerfectPairs: 1, TwentyOnePlus3: 1, LuckyLadies: 1, Buster: 1}
	}

	// La mesure encadre strictement la boucle. Les compteurs du runtime sont
	// lus avant et après, jamais pendant : le coût de l'instrumentation est
	// constant et n'entre pas dans le chemin mesuré.
	blackjack.ResetOps()
	run := metrics.Begin()
	st := blackjack.Simulate(rounds, seed, rules, 1, side)
	m := run.End(int64(st.Rounds))
	elapsed := m.Wall

	writeJSON(w, map[string]any{
		"rounds":        st.Rounds,
		"hands":         st.Hands,
		"houseEdge":     st.HouseEdge() * 100,
		"elementOfRisk": st.ElementOfRisk() * 100,
		"stdDev":        st.StdDev(),
		"variance":      st.Variance(),
		"stdError":      st.StdError() * 100,
		"playerBJ":      ratio(st.PlayerBJ, st.Rounds),
		"dealerBJ":      ratio(st.DealerBJ, st.Rounds),
		"dealerPlayed":  ratio(st.DealerPlayed, st.Rounds),
		"dealerBust":    ratio(st.DealerBust, st.DealerPlayed),
		"shuffles":      st.Shuffles,
		"cardsDealt":    st.CardsDealt,
		"sideEdge":      st.SideEdge() * 100,
		"sideWagered":   st.SideWagered,
		"seconds":       elapsed.Seconds(),
		"roundsPerSec":  m.OpsPerS,
		"nsPerRound":    m.NsPerOp,

		// Débits dérivés de compteurs déjà tenus par le moteur : aucun coût
		// supplémentaire dans la boucle.
		"handsPerSec": float64(st.Hands) / elapsed.Seconds(),
		"cardsPerSec": float64(st.CardsDealt) / elapsed.Seconds(),

		// Mémoire et ramasse-miettes, lus dans le runtime.
		"bytesPerRound":  m.BytesPerOp,
		"allocsPerRound": m.AllocsPerOp,
		"bytesAllocated": m.BytesAlloc,
		"allocRateMBs":   m.AllocRateMBs,
		"gcCycles":       m.GCCycles,
		"gcPauseTotalMs": m.GCPauseTotalMs,
		"gcPauseP99Us":   m.GCPauseP99Us,
		"gcWallShare":    m.GCWallShare,
		"gcCpuShare":     m.GCCPUShare,

		// Temps CPU et parallélisme effectif.
		"cpuNsPerRound": m.CPUNsOp,
		"parallelism":   m.Parallel,
		"numCPU":        m.NumCPU,
		"gomaxprocs":    m.GOMAXPROCS,

		// Compteurs d'opérations : vides si le binaire n'est pas instrumenté.
		"ops":          blackjack.Ops(),
		"instrumented": blackjack.Instrumented,
	})
}

func handleCurve(w http.ResponseWriter, r *http.Request) {
	rounds := intParam(r, "rounds", 2_000_000, 1_000, 50_000_000)
	points := intParam(r, "points", 40, 5, 200)
	seed := int64(intParam(r, "seed", 42, 0, 1<<30))

	writeJSON(w, blackjack.SimulateCurve(rounds, points, seed, blackjack.DefaultRules(), 1, blackjack.SideBets{}))
}

// streamEvent est un relevé émis après chaque lot.
type streamEvent struct {
	Kind       string  `json:"kind"` // "progress" ou "done"
	Done       int     `json:"done"`
	Total      int     `json:"total"`
	ElapsedSec float64 `json:"elapsedSeconds"`

	// Mesures du lot écoulé : ce sont elles qui bougent en direct.
	RoundsPerSec   float64 `json:"roundsPerSec"`
	NsPerRound     float64 `json:"nsPerRound"`
	AllocMBs       float64 `json:"allocMBPerSec"`
	BytesPerRound  float64 `json:"bytesPerRound"`
	AllocsPerRound float64 `json:"allocsPerRound"`
	GCCpuShare     float64 `json:"gcCpuShare"`
	Parallelism    float64 `json:"parallelism"`

	// Cumuls depuis le début de l'exécution.
	GCCyclesTotal uint64  `json:"gcCyclesTotal"`
	HouseEdge     float64 `json:"houseEdge"`
	StdError      float64 `json:"stdError"`
	StdDev        float64 `json:"stdDev"`
	Hands         int     `json:"hands"`
	CardsDealt    int     `json:"cardsDealt"`
	Shuffles      int     `json:"shuffles"`
	SideEdge      float64 `json:"sideEdge"`

	// Renseigné uniquement sur l'événement final.
	OverallRoundsPerSec float64             `json:"overallRoundsPerSec,omitempty"`
	Ops                 *blackjack.OpCounts `json:"ops,omitempty"`
}

// handleSimulateStream déroule une simulation par lots et émet un relevé de
// métriques après chacun, en Server-Sent Events.
//
// AVERTISSEMENT DE MÉTHODE : cette exécution n'est pas une mesure de référence.
// Elle passe par blackjack.Runner, qui duplique la boucle de Simulate pour
// pouvoir être interrompue, et relève les compteurs du runtime entre chaque
// lot. Le chiffre qui fait foi reste celui d'une exécution d'un seul bloc en
// ligne de commande. L'intérêt ici est d'observer l'établissement du régime,
// pas de chiffrer un gain.
func handleSimulateStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpError(w, http.StatusInternalServerError, "le flux n'est pas supporté par ce serveur")
		return
	}

	rounds := intParam(r, "rounds", 2_000_000, 10_000, 50_000_000)
	batches := intParam(r, "batches", 80, 10, 400)
	seed := int64(intParam(r, "seed", 42, 0, 1<<29))

	rules := blackjack.DefaultRules()
	rules.NumDecks = intParam(r, "decks", 4, 1, 8)
	rules.DealerHitsSoft17 = r.URL.Query().Get("h17") == "true"

	var side blackjack.SideBets
	if r.URL.Query().Get("sidebets") == "true" {
		side = blackjack.SideBets{PerfectPairs: 1, TwentyOnePlus3: 1, LuckyLadies: 1, Buster: 1}
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	// Désactive une éventuelle mise en tampon par un intermédiaire.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	blackjack.ResetOps()
	runner := blackjack.NewRunner(seed, rules, 1, side)

	batchSize := rounds / batches
	if batchSize < 1 {
		batchSize = 1
	}

	overall := metrics.Begin()
	start := time.Now()
	done := 0

	for done < rounds {
		// Un client qui ferme l'onglet ne doit pas laisser le serveur simuler
		// dans le vide.
		select {
		case <-r.Context().Done():
			return
		default:
		}

		n := batchSize
		if done+n > rounds {
			n = rounds - done
		}

		bm := metrics.Begin()
		runner.RunBatch(n)
		m := bm.End(int64(n))
		done += n

		st := runner.Stats()
		if !sendEvent(w, flusher, streamEvent{
			Kind:       "progress",
			Done:       done,
			Total:      rounds,
			ElapsedSec: time.Since(start).Seconds(),

			RoundsPerSec:   m.OpsPerS,
			NsPerRound:     m.NsPerOp,
			AllocMBs:       m.AllocRateMBs,
			BytesPerRound:  m.BytesPerOp,
			AllocsPerRound: m.AllocsPerOp,
			GCCpuShare:     m.GCCPUShare,
			Parallelism:    m.Parallel,

			GCCyclesTotal: cumulativeGC(overall),
			HouseEdge:     st.HouseEdge() * 100,
			StdError:      st.StdError() * 100,
			StdDev:        st.StdDev(),
			Hands:         st.Hands,
			CardsDealt:    st.CardsDealt,
			Shuffles:      st.Shuffles,
			SideEdge:      st.SideEdge() * 100,
		}) {
			return
		}
	}

	final := overall.End(int64(done))
	st := runner.Stats()
	ops := blackjack.Ops()
	sendEvent(w, flusher, streamEvent{
		Kind:       "done",
		Done:       done,
		Total:      rounds,
		ElapsedSec: final.WallSec,

		RoundsPerSec:   final.OpsPerS,
		NsPerRound:     final.NsPerOp,
		AllocMBs:       final.AllocRateMBs,
		BytesPerRound:  final.BytesPerOp,
		AllocsPerRound: final.AllocsPerOp,
		GCCpuShare:     final.GCCPUShare,
		Parallelism:    final.Parallel,

		GCCyclesTotal: final.GCCycles,
		HouseEdge:     st.HouseEdge() * 100,
		StdError:      st.StdError() * 100,
		StdDev:        st.StdDev(),
		Hands:         st.Hands,
		CardsDealt:    st.CardsDealt,
		Shuffles:      st.Shuffles,
		SideEdge:      st.SideEdge() * 100,

		OverallRoundsPerSec: final.OpsPerS,
		Ops:                 &ops,
	})
}

// sendEvent écrit un événement SSE et le pousse immédiatement. Renvoie false si
// le client s'est déconnecté.
func sendEvent(w http.ResponseWriter, f http.Flusher, ev streamEvent) bool {
	payload, err := json.Marshal(ev)
	if err != nil {
		log.Printf("encodage de l'événement : %v", err)
		return false
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return false
	}
	f.Flush()
	return true
}

// cumulativeGC relève le nombre de cycles de GC depuis le début de l'exécution
// sans clore la mesure globale.
func cumulativeGC(run *metrics.Run) uint64 {
	return run.GCCyclesSoFar()
}

// handleSampleRound joue un coup isole et renvoie son recit. C'est le
// consommateur du journal narratif construit par PlayRound.
func handleSampleRound(w http.ResponseWriter, r *http.Request) {
	seed := int64(intParam(r, "seed", int(time.Now().UnixNano()%(1<<29)), 0, 1<<29))

	side := blackjack.SideBets{}
	if r.URL.Query().Get("sidebets") == "true" {
		side = blackjack.SideBets{PerfectPairs: 5, TwentyOnePlus3: 5, LuckyLadies: 5, Buster: 5}
	}

	res := blackjack.SampleRound(seed, blackjack.DefaultRules(), 10, side)
	writeJSON(w, map[string]any{
		"seed":    seed,
		"log":     res.Log,
		"mainNet": res.MainNet,
		"sideNet": res.SideNet,
		"hands":   res.Hands,
	})
}

func handleRulesComparison(w http.ResponseWriter, r *http.Request) {
	rounds := intParam(r, "rounds", 500_000, 10_000, 10_000_000)
	seed := int64(intParam(r, "seed", 42, 0, 1<<30))

	writeJSON(w, blackjack.CompareRules(rounds, seed))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encodage JSON : %v", err)
	}
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// intParam lit un entier de la requête en le bornant, pour qu'une valeur
// aberrante ne puisse pas monopoliser le serveur.
func intParam(r *http.Request, name string, def, min, max int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def
	}
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
		if n > max {
			return max
		}
	}
	if n < min {
		return min
	}
	return n
}

func ratio(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
