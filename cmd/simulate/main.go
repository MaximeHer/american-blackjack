// Commande simulate : joue un grand nombre de coups de blackjack américain et
// rapporte l'avantage de la maison, le débit du moteur et son comportement
// mémoire.
//
// Le débit est la métrique centrale du travail d'optimisation. L'avantage de la
// maison, lui, ne doit jamais varier à seed constant au-delà du bruit
// statistique : il sert d'oracle de non-régression.
//
// Toutes les métriques sont relevées AUTOUR de la boucle de simulation, jamais
// à l'intérieur. Le coût de la mesure est donc constant et indépendant du
// nombre de coups : la boucle mesurée est exactement celle qui tournerait en
// production.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/MaximeHer/american-blackjack/internal/blackjack"
	"github.com/MaximeHer/american-blackjack/internal/metrics"
)

func main() {
	var (
		rounds = flag.Int("rounds", 1_000_000, "nombre de coups à simuler")
		seed   = flag.Int64("seed", 42, "graine du générateur pseudo-aléatoire")
		bet    = flag.Float64("bet", 1, "mise principale par coup")

		decks       = flag.Int("decks", 4, "nombre de jeux dans le sabot")
		penetration = flag.Float64("penetration", 0.75, "fraction du sabot distribuée avant rebattage")
		h17         = flag.Bool("h17", false, "le croupier tire sur 17 souple")
		surrender   = flag.Bool("surrender", true, "abandon tardif autorisé")
		das         = flag.Bool("das", true, "double autorisé après split")

		perfectPairs = flag.Float64("perfect-pairs", 0, "mise sur Perfect Pairs")
		plus3        = flag.Float64("21plus3", 0, "mise sur 21+3")
		luckyLadies  = flag.Float64("lucky-ladies", 0, "mise sur Lucky Ladies")
		buster       = flag.Float64("buster", 0, "mise sur Buster Blackjack")

		quiet  = flag.Bool("quiet", false, "n'afficher que les coups par seconde")
		asJSON = flag.Bool("json", false, "émettre toutes les métriques en JSON")
		warmup = flag.Int("warmup", 0, "coups joués et jetés avant la mesure")
	)
	flag.Parse()

	if *rounds <= 0 {
		fmt.Fprintln(os.Stderr, "le nombre de coups doit être positif")
		os.Exit(2)
	}

	rules := blackjack.DefaultRules()
	rules.NumDecks = *decks
	rules.Penetration = *penetration
	rules.DealerHitsSoft17 = *h17
	rules.LateSurrender = *surrender
	rules.DoubleAfterSplit = *das

	side := blackjack.SideBets{
		PerfectPairs:   *perfectPairs,
		TwentyOnePlus3: *plus3,
		LuckyLadies:    *luckyLadies,
		Buster:         *buster,
	}

	// Chauffe optionnelle : remplit les caches du processeur et stabilise le
	// tas avant la mesure. Son résultat est jeté.
	if *warmup > 0 {
		_ = blackjack.Simulate(*warmup, *seed, rules, *bet, side)
	}

	// La mesure encadre strictement la boucle, et rien d'autre.
	blackjack.ResetOps()
	run := metrics.Begin()
	st := blackjack.Simulate(*rounds, *seed, rules, *bet, side)
	m := run.End(int64(st.Rounds))
	ops := blackjack.Ops()

	switch {
	case *quiet:
		fmt.Printf("%.0f\n", m.OpsPerS)
	case *asJSON:
		emitJSON(st, m, ops, rules, side, *seed)
	default:
		emitText(st, m, ops, rules, side, *seed)
	}
}

func emitText(st blackjack.Stats, m metrics.Report, ops blackjack.OpCounts,
	rules blackjack.Rules, side blackjack.SideBets, seed int64) {

	fmt.Println("=== Banc d'essai ===")
	fmt.Printf("Runtime              : %s sur %s/%s\n", m.GoVersion, m.GOOS, m.GOARCH)
	fmt.Printf("Coeurs logiques      : %d (GOMAXPROCS = %d)\n", m.NumCPU, m.GOMAXPROCS)
	fmt.Printf("Binaire              : %s\n", buildKind(ops))

	fmt.Println()
	fmt.Println("=== Configuration ===")
	fmt.Printf("Jeux                 : %d (pénétration %.0f %%)\n", rules.NumDecks, rules.Penetration*100)
	fmt.Printf("Croupier 17 souple   : %s\n", h17Label(rules.DealerHitsSoft17))
	fmt.Printf("Blackjack payé       : %.2f:1\n", rules.BlackjackPayout)
	fmt.Printf("Abandon tardif       : %v | Double après split : %v\n", rules.LateSurrender, rules.DoubleAfterSplit)
	fmt.Printf("Graine               : %d\n", seed)

	fmt.Println()
	fmt.Println("=== Résultats du jeu principal ===")
	fmt.Printf("Coups joués          : %s\n", num(int64(st.Rounds)))
	fmt.Printf("Mains jouées         : %s (%.3f par coup)\n", num(int64(st.Hands)), perRound(st.Hands, st.Rounds))
	fmt.Printf("Mise initiale totale : %.2f\n", st.MainWagered)
	fmt.Printf("Sommes engagées      : %.2f\n", st.Action)
	fmt.Printf("Résultat net         : %+.2f\n", st.MainNet)
	fmt.Printf("Avantage maison      : %+.4f %% de la mise initiale\n", st.HouseEdge()*100)
	fmt.Printf("Element of risk      : %+.4f %% des sommes engagées\n", st.ElementOfRisk()*100)

	fmt.Println()
	fmt.Println("=== Dispersion statistique ===")
	fmt.Printf("Écart-type par coup  : %.4f unité de mise\n", st.StdDev())
	fmt.Printf("Variance par coup    : %.4f\n", st.Variance())
	fmt.Printf("Erreur-type          : %.4f point sur l'avantage mesuré\n", st.StdError()*100)
	fmt.Printf("Intervalle à 95 %%    : [%+.4f ; %+.4f] %%\n",
		(st.HouseEdge()-1.96*st.StdError())*100,
		(st.HouseEdge()+1.96*st.StdError())*100)

	fmt.Println()
	fmt.Println("=== Débit ===")
	fmt.Printf("Durée (horloge)      : %s\n", dur(m.Wall))
	fmt.Printf("Temps CPU occupé     : %s\n", dur(m.CPUBusy))
	fmt.Printf("Parallélisme effectif: %.2f coeur sur %d\n", m.Parallel, m.NumCPU)
	fmt.Printf("Coups par seconde    : %s\n", num(int64(m.OpsPerS)))
	fmt.Printf("Temps par coup       : %s ns (horloge) | %s ns (CPU)\n",
		num(int64(m.NsPerOp)), num(int64(m.CPUNsOp)))
	fmt.Printf("Mains par seconde    : %s\n", num(int64(rate(st.Hands, m.WallSec))))
	fmt.Printf("Cartes par seconde   : %s\n", num(int64(rate(st.CardsDealt, m.WallSec))))
	fmt.Printf("Rebattages           : %s (%s par seconde)\n",
		num(int64(st.Shuffles)), num(int64(rate(st.Shuffles, m.WallSec))))

	fmt.Println()
	fmt.Println("=== Mémoire et ramasse-miettes ===")
	fmt.Printf("Alloué               : %s (%.0f o par coup)\n", bytes(m.BytesAlloc), m.BytesPerOp)
	fmt.Printf("Objets alloués       : %s (%.1f par coup)\n", num(int64(m.ObjectsAlloc)), m.AllocsPerOp)
	fmt.Printf("Débit d'allocation   : %.0f Mo/s\n", m.AllocRateMBs)
	fmt.Printf("Cycles de GC         : %d (%.0f par million de coups)\n", m.GCCycles, m.GCPerOp)
	fmt.Printf("Pause GC cumulée     : %s (%.2f %% du temps horloge)\n", dur(m.GCPauseTotal), m.GCWallShare)
	fmt.Printf("Pause GC p50 / p99   : %s / %s (max %s)\n", dur(m.GCPauseP50), dur(m.GCPauseP99), dur(m.GCPauseMax))
	fmt.Printf("Part CPU du GC       : %.2f %%\n", m.GCCPUShare)
	fmt.Printf("Objets sur le tas    : %s → %s\n", num(int64(m.HeapObjectsIn)), num(int64(m.HeapObjectsEnd)))

	fmt.Println()
	fmt.Println("=== Ordonnanceur ===")
	fmt.Printf("Goroutines créées    : %d\n", m.GoroutinesCreated)
	fmt.Printf("Latence p50 / p99    : %s / %s\n", dur(m.SchedLatP50), dur(m.SchedLatP99))

	fmt.Println()
	fmt.Println("=== Fréquences observées ===")
	fmt.Printf("Blackjacks joueur    : %.3f %%\n", pct(st.PlayerBJ, st.Rounds))
	fmt.Printf("Blackjacks croupier  : %.3f %%\n", pct(st.DealerBJ, st.Rounds))
	fmt.Printf("Croupier a joué      : %.2f %% des coups\n", pct(st.DealerPlayed, st.Rounds))
	fmt.Printf("Croupier sauté       : %.3f %% des mains qu'il a jouées\n", pct(st.DealerBust, st.DealerPlayed))

	if side.Any() {
		fmt.Println()
		fmt.Println("=== Paris annexes ===")
		fmt.Printf("Misé                 : %.2f\n", st.SideWagered)
		fmt.Printf("Résultat net         : %+.2f\n", st.SideNet)
		fmt.Printf("Avantage maison      : %+.4f %%\n", st.SideEdge()*100)
	}

	fmt.Println()
	fmt.Println("=== Opérations élémentaires ===")
	if !ops.Enabled {
		fmt.Println("Binaire non instrumenté : aucun compteur n'est compilé dans ce binaire.")
		fmt.Println("Pour les obtenir :  go run -tags instrument ./cmd/simulate -rounds 100000")
		fmt.Println("Ce binaire-là est plus lent et ne doit jamais servir à mesurer un débit.")
		return
	}
	r := float64(st.Rounds)
	fmt.Printf("Décisions            : %s (%.2f par coup)\n", num(int64(ops.Decisions)), float64(ops.Decisions)/r)
	fmt.Printf("Appels à Total()     : %s (%.2f par coup, %.2f par décision)\n",
		num(int64(ops.HandTotals)), float64(ops.HandTotals)/r, ratio(ops.HandTotals, ops.Decisions))
	fmt.Printf("Évaluations de carte : %s (%.2f par coup)\n", num(int64(ops.CardValues)), float64(ops.CardValues)/r)
	fmt.Printf("Consultations de map : %s (%.2f par coup)\n", num(int64(ops.MapLookups)), float64(ops.MapLookups)/r)
	fmt.Printf("Déplacements de mélange : %s (%.0f par rebattage)\n",
		num(int64(ops.ShuffleMoves)), ratio(ops.ShuffleMoves, uint64(st.Shuffles)))
	fmt.Println()
	fmt.Println("Le nombre de déplacements par rebattage mesure directement le coût")
	fmt.Println("quadratique du mélange naïf : Fisher-Yates en ferait 208.")
}

// payload est la forme JSON, destinée au script d'automatisation des mesures.
type payload struct {
	Config struct {
		Rounds       int     `json:"rounds"`
		Seed         int64   `json:"seed"`
		Decks        int     `json:"decks"`
		Penetration  float64 `json:"penetration"`
		H17          bool    `json:"dealerHitsSoft17"`
		SideBets     bool    `json:"sideBets"`
		Instrumented bool    `json:"instrumented"`
	} `json:"config"`
	Game struct {
		Rounds        int     `json:"rounds"`
		Hands         int     `json:"hands"`
		HouseEdge     float64 `json:"houseEdgePercent"`
		ElementOfRisk float64 `json:"elementOfRiskPercent"`
		StdDev        float64 `json:"stdDevPerRound"`
		Variance      float64 `json:"variancePerRound"`
		StdError      float64 `json:"stdErrorPercent"`
		CardsDealt    int     `json:"cardsDealt"`
		Shuffles      int     `json:"shuffles"`
		SideEdge      float64 `json:"sideEdgePercent"`
	} `json:"game"`
	Throughput struct {
		RoundsPerSecond float64 `json:"roundsPerSecond"`
		HandsPerSecond  float64 `json:"handsPerSecond"`
		CardsPerSecond  float64 `json:"cardsPerSecond"`
	} `json:"throughput"`
	Runtime metrics.Report     `json:"runtime"`
	Ops     blackjack.OpCounts `json:"ops"`
}

func emitJSON(st blackjack.Stats, m metrics.Report, ops blackjack.OpCounts,
	rules blackjack.Rules, side blackjack.SideBets, seed int64) {

	var p payload
	p.Config.Rounds = st.Rounds
	p.Config.Seed = seed
	p.Config.Decks = rules.NumDecks
	p.Config.Penetration = rules.Penetration
	p.Config.H17 = rules.DealerHitsSoft17
	p.Config.SideBets = side.Any()
	p.Config.Instrumented = blackjack.Instrumented

	p.Game.Rounds = st.Rounds
	p.Game.Hands = st.Hands
	p.Game.HouseEdge = st.HouseEdge() * 100
	p.Game.ElementOfRisk = st.ElementOfRisk() * 100
	p.Game.StdDev = st.StdDev()
	p.Game.Variance = st.Variance()
	p.Game.StdError = st.StdError() * 100
	p.Game.CardsDealt = st.CardsDealt
	p.Game.Shuffles = st.Shuffles
	p.Game.SideEdge = st.SideEdge() * 100

	p.Throughput.RoundsPerSecond = m.OpsPerS
	p.Throughput.HandsPerSecond = rate(st.Hands, m.WallSec)
	p.Throughput.CardsPerSecond = rate(st.CardsDealt, m.WallSec)

	p.Runtime = m
	p.Ops = ops

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(p); err != nil {
		fmt.Fprintf(os.Stderr, "encodage JSON : %v\n", err)
		os.Exit(1)
	}
}

// --- mise en forme ---

func h17Label(h17 bool) string {
	if h17 {
		return "tire (H17)"
	}
	return "reste (S17)"
}

func buildKind(ops blackjack.OpCounts) string {
	if ops.Enabled {
		return "INSTRUMENTÉ (-tags instrument) — ne pas s'en servir pour mesurer un débit"
	}
	return "par défaut, non instrumenté"
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

func perRound(n, rounds int) float64 {
	if rounds == 0 {
		return 0
	}
	return float64(n) / float64(rounds)
}

func rate(n int, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(n) / seconds
}

func ratio(a, b uint64) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// num met en forme un entier avec des espaces fines comme séparateurs de
// milliers, pour que les grands nombres restent lisibles.
func num(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%d", v)
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ' ')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

func bytes(b uint64) string {
	const unit = 1024
	switch {
	case b < unit:
		return fmt.Sprintf("%d o", b)
	case b < unit*unit:
		return fmt.Sprintf("%.1f Kio", float64(b)/unit)
	case b < unit*unit*unit:
		return fmt.Sprintf("%.1f Mio", float64(b)/(unit*unit))
	default:
		return fmt.Sprintf("%.2f Gio", float64(b)/(unit*unit*unit))
	}
}

func dur(d time.Duration) string {
	switch {
	case d == 0:
		return "0"
	case d < time.Microsecond:
		return fmt.Sprintf("%d ns", d.Nanoseconds())
	case d < time.Millisecond:
		return fmt.Sprintf("%.1f µs", float64(d.Nanoseconds())/1e3)
	case d < time.Second:
		return fmt.Sprintf("%.2f ms", float64(d.Nanoseconds())/1e6)
	default:
		return d.Round(time.Millisecond).String()
	}
}
