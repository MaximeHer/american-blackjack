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
	"net/http"
	"sync"
	"time"

	"github.com/MaximeHer/american-blackjack/internal/blackjack"
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
	addr := flag.String("addr", ":8080", "adresse d'écoute")
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

	fmt.Printf("Table ouverte sur http://localhost%s\n", *addr)
	fmt.Printf("Règles : %d jeux, croupier %s, blackjack payé %.2f:1\n",
		rules.NumDecks,
		map[bool]string{true: "tire sur 17 souple", false: "reste sur 17 souple"}[rules.DealerHitsSoft17],
		rules.BlackjackPayout)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
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

	start := time.Now()
	st := blackjack.Simulate(rounds, seed, rules, 1, side)
	elapsed := time.Since(start)

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
		"roundsPerSec":  float64(st.Rounds) / elapsed.Seconds(),
		"nsPerRound":    float64(elapsed.Nanoseconds()) / float64(st.Rounds),
	})
}

func handleCurve(w http.ResponseWriter, r *http.Request) {
	rounds := intParam(r, "rounds", 2_000_000, 1_000, 50_000_000)
	points := intParam(r, "points", 40, 5, 200)
	seed := int64(intParam(r, "seed", 42, 0, 1<<30))

	writeJSON(w, blackjack.SimulateCurve(rounds, points, seed, blackjack.DefaultRules(), 1, blackjack.SideBets{}))
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
