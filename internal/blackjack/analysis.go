package blackjack

import (
	"math"
	"math/rand"
)

// Ce fichier produit les données des figures du rapport. Il est HORS du chemin
// mesuré : SimulateCurve duplique volontairement la boucle de Simulate plutôt
// que d'instrumenter celle-ci, car ajouter des points de contrôle dans la
// boucle de référence en falsifierait la mesure.

// CurvePoint est un relevé de convergence.
type CurvePoint struct {
	Rounds int     `json:"rounds"`
	Edge   float64 `json:"edge"`
	StdErr float64 `json:"stderr"`
}

// SimulateCurve joue n coups en relevant l'avantage de la maison cumulé en
// points jalons répartis logarithmiquement.
//
// La figure obtenue illustre la décroissance de l'erreur-type en 1/racine(n) :
// l'avantage mesuré oscille très largement sur les premiers milliers de coups
// avant de se resserrer autour de sa valeur vraie. C'est la justification
// visuelle du besoin de débit.
func SimulateCurve(n, points int, seed int64, r Rules, bet float64, sb SideBets) []CurvePoint {
	if points < 2 {
		points = 2
	}
	rng := rand.New(rand.NewSource(seed))
	shoe := NewShoe(r.NumDecks, r.Penetration, rng)

	marks := logSpaced(n, points)
	next := 0

	var st Stats
	out := make([]CurvePoint, 0, points)

	for i := 1; i <= n; i++ {
		if shoe.CutReached() {
			shoe.Shuffle()
		}
		st.Add(PlayRound(shoe, r, bet, sb, nil))

		if next < len(marks) && i == marks[next] {
			out = append(out, CurvePoint{
				Rounds: i,
				Edge:   st.HouseEdge() * 100,
				StdErr: st.StdError() * 100,
			})
			next++
		}
	}
	return out
}

// logSpaced répartit `points` jalons entre 100 et n sur une échelle
// logarithmique, en évitant les doublons.
func logSpaced(n, points int) []int {
	if n < 100 {
		return []int{n}
	}
	lo, hi := math.Log10(100), math.Log10(float64(n))
	out := make([]int, 0, points)
	last := 0
	for i := 0; i < points; i++ {
		f := lo + (hi-lo)*float64(i)/float64(points-1)
		v := int(math.Round(math.Pow(10, f)))
		if v > last && v <= n {
			out = append(out, v)
			last = v
		}
	}
	if last != n {
		out = append(out, n)
	}
	return out
}

// StrategyExport est la stratégie de base sous forme tabulaire, pour
// l'affichage d'une grille colorée.
type StrategyExport struct {
	Dealer []string          `json:"dealer"`
	Hard   map[string]string `json:"hard"`
	Soft   map[string]string `json:"soft"`
	Pairs  map[string]string `json:"pairs"`
}

// ExportStrategy renvoie les tables de stratégie de base.
func ExportStrategy() StrategyExport {
	hard := make(map[string]string, len(hardRows))
	for total, row := range hardRows {
		hard[itoa(total)] = row
	}
	soft := make(map[string]string, len(softRows))
	for total, row := range softRows {
		soft[itoa(total)] = row
	}
	pairs := make(map[string]string, len(pairRows))
	for rank, row := range pairRows {
		pairs[rank] = row
	}
	return StrategyExport{
		Dealer: dealerColumns,
		Hard:   hard,
		Soft:   soft,
		Pairs:  pairs,
	}
}

// itoa évite d'importer strconv pour ce seul usage hors chemin mesuré.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// RuleComparison est l'avantage de la maison mesuré pour une variante de
// règles donnée.
type RuleComparison struct {
	Label string  `json:"label"`
	Edge  float64 `json:"edge"`
	Error float64 `json:"error"`
}

// CompareRules mesure l'avantage de la maison sur plusieurs variantes de
// règles, pour la figure comparative du rapport.
func CompareRules(n int, seed int64) []RuleComparison {
	type variant struct {
		label string
		mod   func(*Rules)
	}
	variants := []variant{
		{"Référence (4 jeux, S17)", func(r *Rules) {}},
		{"Croupier tire sur 17 souple", func(r *Rules) { r.DealerHitsSoft17 = true }},
		{"8 jeux", func(r *Rules) { r.NumDecks = 8 }},
		{"Jeu unique", func(r *Rules) { r.NumDecks = 1 }},
		{"Blackjack payé 6:5", func(r *Rules) { r.BlackjackPayout = 1.2 }},
		{"Sans abandon", func(r *Rules) { r.LateSurrender = false }},
		{"Sans double après split", func(r *Rules) { r.DoubleAfterSplit = false }},
		{"Double limité à 9-11", func(r *Rules) { r.DoubleAnyTwo = false }},
	}

	out := make([]RuleComparison, 0, len(variants))
	for _, v := range variants {
		r := DefaultRules()
		v.mod(&r)
		st := Simulate(n, seed, r, 1, SideBets{})
		out = append(out, RuleComparison{
			Label: v.label,
			Edge:  st.HouseEdge() * 100,
			Error: st.StdError() * 100,
		})
	}
	return out
}

// SampleRound joue un coup isolé et renvoie son résultat, journal narratif
// compris. C'est le consommateur légitime du journal construit par PlayRound,
// affiché par le panneau de narration de l'interface.
//
// Le défaut de la version de référence n'est pas de produire ce journal, c'est
// de le produire aussi dans la boucle de simulation, qui ne le lit jamais.
func SampleRound(seed int64, r Rules, bet float64, sb SideBets) (RoundResult, []string) {
	rng := rand.New(rand.NewSource(seed))
	shoe := NewShoe(r.NumDecks, r.Penetration, rng)
	tr := NewTrace()
	res := PlayRound(shoe, r, bet, sb, tr)
	return res, tr.Lines
}
