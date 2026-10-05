// Commande simulate : joue un grand nombre de coups de blackjack américain et
// rapporte l'avantage de la maison ainsi que le débit du moteur.
//
// Le débit, exprimé en coups par seconde, est la métrique centrale de tout le
// travail d'optimisation. L'avantage de la maison, lui, ne doit jamais varier
// à seed constant : il sert d'oracle de non-régression.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/MaximeHer/american-blackjack/internal/blackjack"
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

		quiet = flag.Bool("quiet", false, "n'afficher que les coups par seconde")
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

	start := time.Now()
	st := blackjack.Simulate(*rounds, *seed, rules, *bet, side)
	elapsed := time.Since(start)

	perSec := float64(st.Rounds) / elapsed.Seconds()

	if *quiet {
		fmt.Printf("%.0f\n", perSec)
		return
	}

	fmt.Println("=== Configuration ===")
	fmt.Printf("Jeux                 : %d (pénétration %.0f %%)\n", rules.NumDecks, rules.Penetration*100)
	fmt.Printf("Croupier 17 souple   : %s\n", map[bool]string{true: "tire (H17)", false: "reste (S17)"}[rules.DealerHitsSoft17])
	fmt.Printf("Blackjack payé       : %.2f:1\n", rules.BlackjackPayout)
	fmt.Printf("Abandon tardif       : %v | Double après split : %v\n", rules.LateSurrender, rules.DoubleAfterSplit)
	fmt.Printf("Graine               : %d\n", *seed)
	fmt.Printf("Go                   : %s sur %s/%s, %d coeurs logiques\n",
		runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())

	fmt.Println()
	fmt.Println("=== Résultats du jeu principal ===")
	fmt.Printf("Coups joués          : %d\n", st.Rounds)
	fmt.Printf("Mains jouées         : %d (%.3f par coup)\n", st.Hands, float64(st.Hands)/float64(st.Rounds))
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
	fmt.Println("=== Fréquences observées ===")
	fmt.Printf("Blackjacks joueur    : %.3f %%\n", pct(st.PlayerBJ, st.Rounds))
	fmt.Printf("Blackjacks croupier  : %.3f %%\n", pct(st.DealerBJ, st.Rounds))
	fmt.Printf("Croupier a joué      : %.2f %% des coups\n", pct(st.DealerPlayed, st.Rounds))
	fmt.Printf("Croupier sauté       : %.3f %% des mains qu'il a jouées\n", pct(st.DealerBust, st.DealerPlayed))
	fmt.Printf("Rebattages           : %d | cartes distribuées : %d\n", st.Shuffles, st.CardsDealt)

	if side.Any() {
		fmt.Println()
		fmt.Println("=== Paris annexes ===")
		fmt.Printf("Misé                 : %.2f\n", st.SideWagered)
		fmt.Printf("Résultat net         : %+.2f\n", st.SideNet)
		fmt.Printf("Avantage maison      : %+.4f %%\n", st.SideEdge()*100)
	}

	fmt.Println()
	fmt.Println("=== Débit ===")
	fmt.Printf("Durée                : %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Coups par seconde    : %.0f\n", perSec)
	fmt.Printf("Temps par coup       : %.0f ns\n", float64(elapsed.Nanoseconds())/float64(st.Rounds))
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
