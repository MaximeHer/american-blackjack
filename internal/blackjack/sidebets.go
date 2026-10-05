package blackjack

import (
	"fmt"
	"sort"
)

// SideBets porte les montants misés sur chaque pari annexe. Un montant nul
// signifie que le pari n'est pas joué, et son évaluation est alors sautée.
type SideBets struct {
	PerfectPairs   float64
	TwentyOnePlus3 float64
	LuckyLadies    float64
	Buster         float64
}

// Total renvoie la somme engagée sur les paris annexes pour un coup.
func (sb SideBets) Total() float64 {
	return sb.PerfectPairs + sb.TwentyOnePlus3 + sb.LuckyLadies + sb.Buster
}

// Any indique si au moins un pari annexe est joué.
func (sb SideBets) Any() bool { return sb.Total() > 0 }

// Tables de gains, exprimées en multiplicateur du montant misé (gain net,
// hors retour de la mise).
//
// VERSION DE RÉFÉRENCE : des maps indexées par chaîne, consultées sur le hot
// path. Chaque consultation hache une chaîne, et plusieurs libellés sont même
// construits par fmt.Sprintf. Les paliers d'optimisation remplaceront ces maps
// par des tableaux plats indexés par une énumération entière.
var perfectPairsPaytable = map[string]float64{
	"parfaite": 25, // même rang et même enseigne
	"coloree":  12, // même rang, même couleur chromatique, enseignes différentes
	"mixte":    6,  // même rang, couleurs chromatiques différentes
}

var twentyOnePlus3Paytable = map[string]float64{
	"brelan_couleur": 100, // trois cartes identiques de même enseigne
	"quinte_flush":   40,
	"brelan":         30,
	"quinte":         10,
	"couleur":        5,
}

var luckyLadiesPaytable = map[string]float64{
	"paire_dame_coeur_bj": 1000, // deux Dames de coeur et blackjack du croupier
	"paire_dame_coeur":    200,
	"vingt_identique":     25, // 20 formé de deux cartes rigoureusement identiques
	"vingt_couleur":       10, // 20 de même enseigne
	"vingt":               4,
}

// busterPaytable paie selon le nombre de cartes de la main sautée du croupier.
var busterPaytable = map[int]float64{
	3: 2,
	4: 2,
	5: 4,
	6: 18,
	7: 50,
	8: 250, // 8 cartes ou plus
}

// EvalPerfectPairs évalue le pari Perfect Pairs sur les deux premières cartes
// du joueur. Renvoie le multiplicateur de gain et le libellé de la combinaison.
func EvalPerfectPairs(a, b Card) (float64, string) {
	if a.Rank != b.Rank {
		return 0, "perdu"
	}
	switch {
	case a.Suit == b.Suit:
		return perfectPairsPaytable["parfaite"], "parfaite"
	case a.Color() == b.Color():
		return perfectPairsPaytable["coloree"], "coloree"
	default:
		return perfectPairsPaytable["mixte"], "mixte"
	}
}

// EvalTwentyOnePlus3 évalue le pari 21+3 : les deux cartes du joueur et la
// carte visible du croupier forment une main de poker à trois cartes.
func EvalTwentyOnePlus3(a, b, up Card) (float64, string) {
	flush := a.Suit == b.Suit && b.Suit == up.Suit
	trips := a.Rank == b.Rank && b.Rank == up.Rank
	straight := isStraight(a, b, up)

	switch {
	case trips && flush:
		return twentyOnePlus3Paytable["brelan_couleur"], "brelan_couleur"
	case straight && flush:
		return twentyOnePlus3Paytable["quinte_flush"], "quinte_flush"
	case trips:
		return twentyOnePlus3Paytable["brelan"], "brelan"
	case straight:
		return twentyOnePlus3Paytable["quinte"], "quinte"
	case flush:
		return twentyOnePlus3Paytable["couleur"], "couleur"
	}
	return 0, "perdu"
}

// isStraight teste trois cartes pour une suite. L'As est évalué deux fois,
// comme 14 pour reconnaître Dame-Roi-As et comme 1 pour reconnaître As-2-3.
func isStraight(cards ...Card) bool {
	high := make([]int, 0, len(cards))
	low := make([]int, 0, len(cards))
	for _, c := range cards {
		r := c.StraightRank()
		high = append(high, r)
		if r == 14 {
			low = append(low, 1)
		} else {
			low = append(low, r)
		}
	}
	return consecutive(high) || consecutive(low)
}

// consecutive indique si les valeurs forment une séquence strictement
// croissante de pas 1 après tri.
func consecutive(v []int) bool {
	s := make([]int, len(v))
	copy(s, v)
	sort.Ints(s)
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1]+1 {
			return false
		}
	}
	return true
}

// EvalLuckyLadies évalue le pari Lucky Ladies, qui paie si les deux premières
// cartes du joueur totalisent 20. Le gain maximal combine une paire de Dames
// de coeur et un blackjack du croupier.
func EvalLuckyLadies(a, b Card, dealerBJ bool) (float64, string) {
	h := &Hand{Cards: []Card{a, b}}
	total, _ := h.Total()
	if total != 20 {
		return 0, "perdu"
	}
	queenOfHearts := a.Rank == "Q" && a.Suit == "Coeur" &&
		b.Rank == "Q" && b.Suit == "Coeur"
	switch {
	case queenOfHearts && dealerBJ:
		return luckyLadiesPaytable["paire_dame_coeur_bj"], "paire_dame_coeur_bj"
	case queenOfHearts:
		return luckyLadiesPaytable["paire_dame_coeur"], "paire_dame_coeur"
	case a.Rank == b.Rank && a.Suit == b.Suit:
		return luckyLadiesPaytable["vingt_identique"], "vingt_identique"
	case a.Suit == b.Suit:
		return luckyLadiesPaytable["vingt_couleur"], "vingt_couleur"
	default:
		return luckyLadiesPaytable["vingt"], "vingt"
	}
}

// EvalBuster évalue le pari Buster Blackjack, qui paie quand le croupier
// saute, d'autant plus que sa main compte de cartes.
func EvalBuster(dealerCardCount int, dealerBusted bool) (float64, string) {
	if !dealerBusted {
		return 0, "perdu"
	}
	n := dealerCardCount
	if n > 8 {
		n = 8
	}
	m, ok := busterPaytable[n]
	if !ok {
		return 0, "perdu"
	}
	return m, fmt.Sprintf("bust_%d_cartes", n)
}

// netSide convertit un multiplicateur de gain en résultat net : si le
// multiplicateur est nul, la mise est perdue.
func netSide(stake, multiplier float64) float64 {
	if multiplier <= 0 {
		return -stake
	}
	return stake * multiplier
}
